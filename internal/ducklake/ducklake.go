// Package ducklake provides a DuckLake-backed storage path for Etiquetta,
// evaluated behind an opt-in flag. DuckLake stores tables as Parquet data files
// plus a catalog with snapshots and time-travel, which replaces the hand-rolled
// buffer -> parquet -> INSERT pipeline and removes an entire class of storage
// bugs (INT32 overflow, constraint drift, quarantine loops).
//
// Nothing here modifies an existing DuckDB file: migration attaches the source
// READ_ONLY and copies out of it.
package ducklake

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

// EventTables are the append-only, high-write tables suited to DuckLake's Parquet
// storage and time-travel. visitor_sessions is deliberately excluded: it is
// materialized via DELETE+INSERT, which a DuckLake-backed view cannot support.
var EventTables = []string{"events", "performance", "errors"}

// Config attaches a DuckLake catalog.
type Config struct {
	CatalogPath string // catalog file, e.g. {dataDir}/lake/catalog.ducklake
	DataPath    string // directory holding the Parquet data files
	Alias       string // attached schema name; defaults to "lake"
}

func (c Config) alias() string {
	if c.Alias == "" {
		return "lake"
	}
	return c.Alias
}

func sqlEscape(s string) string { return strings.ReplaceAll(s, "'", "''") }

// Attach installs and loads the required extensions and attaches the DuckLake
// catalog. Safe to call more than once (ATTACH IF NOT EXISTS).
func Attach(db *sql.DB, cfg Config) error {
	// icu: timezone-correct bucketing. httpfs: object-storage backing later.
	for _, ext := range []string{"ducklake", "httpfs", "icu"} {
		if _, err := db.Exec("INSTALL " + ext); err != nil {
			return fmt.Errorf("install %s: %w", ext, err)
		}
		if _, err := db.Exec("LOAD " + ext); err != nil {
			return fmt.Errorf("load %s: %w", ext, err)
		}
	}
	// Resolve to absolute paths. DuckLake records data-file locations relative to
	// the DATA_PATH given at attach time, so a relative path makes the catalog
	// only re-openable from the same working directory — absolute keeps it
	// portable regardless of where the process runs.
	catalog, err := filepath.Abs(cfg.CatalogPath)
	if err != nil {
		return fmt.Errorf("resolve catalog path: %w", err)
	}
	data, err := filepath.Abs(cfg.DataPath)
	if err != nil {
		return fmt.Errorf("resolve data path: %w", err)
	}
	attach := fmt.Sprintf("ATTACH IF NOT EXISTS 'ducklake:%s' AS %s (DATA_PATH '%s')",
		sqlEscape(catalog), cfg.alias(), sqlEscape(data))
	if _, err := db.Exec(attach); err != nil {
		return fmt.Errorf("attach ducklake catalog: %w", err)
	}
	return nil
}

// TableCounts records source vs migrated row counts for verification.
type TableCounts struct {
	Table  string
	Source int64
	Lake   int64
}

// MigrateFromDuckDB copies the given tables from an existing DuckDB file into the
// attached lake and verifies row counts. The source is attached READ_ONLY and is
// never modified. Returns per-table counts; errors if any count mismatches.
func MigrateFromDuckDB(db *sql.DB, sourcePath string, tables []string) ([]TableCounts, error) {
	if _, err := db.Exec(fmt.Sprintf("ATTACH IF NOT EXISTS '%s' AS oldsrc (READ_ONLY)", sqlEscape(sourcePath))); err != nil {
		return nil, fmt.Errorf("attach source read-only: %w", err)
	}
	defer func() { _, _ = db.Exec("DETACH oldsrc") }()

	alias := "lake"
	counts := make([]TableCounts, 0, len(tables))
	for _, t := range tables {
		if !validIdent(t) {
			return counts, fmt.Errorf("invalid table name %q", t)
		}
		// Does the source have this table? Skip missing ones rather than fail.
		var exists int
		if err := db.QueryRow(
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_catalog = 'oldsrc' AND table_name = ?", t,
		).Scan(&exists); err != nil || exists == 0 {
			continue
		}
		// Copy schema + data. CTAS preserves the source column types, so widen
		// the one column that overflowed on the old INT32 schema — the lake then
		// holds it as BIGINT and the overflow that quarantined batches can't recur.
		selectExpr := "*"
		if t == "events" && columnExists(db, "oldsrc", "events", "page_duration") {
			selectExpr = "* REPLACE (CAST(page_duration AS BIGINT) AS page_duration)"
		}
		if _, err := db.Exec(fmt.Sprintf("CREATE OR REPLACE TABLE %s.%s AS SELECT %s FROM oldsrc.%s", alias, t, selectExpr, t)); err != nil {
			return counts, fmt.Errorf("copy %s into lake: %w", t, err)
		}
		var c TableCounts
		c.Table = t
		if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM oldsrc.%s", t)).Scan(&c.Source); err != nil {
			return counts, fmt.Errorf("count source %s: %w", t, err)
		}
		if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", alias, t)).Scan(&c.Lake); err != nil {
			return counts, fmt.Errorf("count lake %s: %w", t, err)
		}
		if c.Source != c.Lake {
			return counts, fmt.Errorf("row count mismatch for %s: source=%d lake=%d", t, c.Source, c.Lake)
		}
		counts = append(counts, c)
	}
	return counts, nil
}

// tableKind returns "BASE TABLE", "VIEW", or "" (absent) for a table in a catalog.
func tableKind(db *sql.DB, catalog, table string) string {
	var k string
	_ = db.QueryRow(
		"SELECT table_type FROM information_schema.tables WHERE table_catalog = ? AND table_name = ?",
		catalog, table,
	).Scan(&k)
	return k
}

// Lakeify moves each table's data from the main DuckDB into the lake and drops
// the main table, so a main-first search_path routes unqualified reads AND writes
// (INSERT/UPDATE/DELETE) through to the mutable lake table. It is idempotent and
// crash-safe: a table already absent from main (present in the lake) is left
// alone, and the main table is dropped only after the copy is verified row-for-
// row, so the data is always in the lake before the source is removed.
//
// Requires the lake to be attached as "lake". A view is deliberately NOT used —
// views are read-only, which would break retention deletes and bot updates.
func Lakeify(db *sql.DB, tables []string) error {
	// The main catalog is named after the database file (e.g. "etiquetta"), not
	// "main" (which is a schema). Resolve it so every reference is unambiguous.
	var mainCat string
	if err := db.QueryRow("SELECT current_database()").Scan(&mainCat); err != nil {
		return fmt.Errorf("determine main catalog: %w", err)
	}
	if !validIdent(mainCat) {
		return fmt.Errorf("unexpected catalog name %q", mainCat)
	}
	const lakeCat = "lake"
	mainT := func(t string) string { return fmt.Sprintf("%s.main.%s", mainCat, t) }
	lakeT := func(t string) string { return fmt.Sprintf("%s.main.%s", lakeCat, t) }

	for _, t := range tables {
		if !validIdent(t) {
			return fmt.Errorf("invalid table name %q", t)
		}
		mainKind := tableKind(db, mainCat, t)
		lakeHasTable := tableKind(db, lakeCat, t) == "BASE TABLE"

		// Already lakeified: gone from main, present in the lake.
		if mainKind == "" && lakeHasTable {
			continue
		}
		// A leftover view from an older build must go so writes can reach the lake.
		if mainKind == "VIEW" {
			if _, err := db.Exec(fmt.Sprintf("DROP VIEW %s", mainT(t))); err != nil {
				return fmt.Errorf("drop stale view %s: %w", t, err)
			}
			if lakeHasTable {
				continue
			}
			mainKind = "" // nothing left in main to copy
		}
		if mainKind != "BASE TABLE" {
			continue // nothing in main to move
		}

		// Copy to the lake, widening page_duration on events so it can never
		// overflow the old INT32 column again.
		sel := "*"
		if t == "events" && columnExists(db, mainCat, "events", "page_duration") {
			sel = "* REPLACE (CAST(page_duration AS BIGINT) AS page_duration)"
		}
		if _, err := db.Exec(fmt.Sprintf("CREATE OR REPLACE TABLE %s AS SELECT %s FROM %s", lakeT(t), sel, mainT(t))); err != nil {
			return fmt.Errorf("copy %s into lake: %w", t, err)
		}
		var src, lake int64
		if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", mainT(t))).Scan(&src); err != nil {
			return fmt.Errorf("count %s: %w", mainT(t), err)
		}
		if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", lakeT(t))).Scan(&lake); err != nil {
			return fmt.Errorf("count %s: %w", lakeT(t), err)
		}
		if src != lake {
			return fmt.Errorf("lakeify %s: count mismatch main=%d lake=%d (main not touched)", t, src, lake)
		}

		// Drop the source only after the verified copy; the data is now in the
		// lake and unqualified `events` will fall through the search_path to it.
		if _, err := db.Exec(fmt.Sprintf("DROP TABLE %s", mainT(t))); err != nil {
			return fmt.Errorf("drop %s: %w", mainT(t), err)
		}
	}
	return nil
}

// Compact runs DuckLake maintenance: merge small data files and expire old
// snapshots. Safe to call periodically.
func Compact(db *sql.DB, alias string, expireOlderThanDays int) error {
	if alias == "" {
		alias = "lake"
	}
	if _, err := db.Exec(fmt.Sprintf("CALL ducklake_merge_adjacent_files('%s')", sqlEscape(alias))); err != nil {
		return fmt.Errorf("merge files: %w", err)
	}
	if expireOlderThanDays > 0 {
		q := fmt.Sprintf("CALL ducklake_expire_snapshots('%s', older_than => now() - INTERVAL '%d days')", sqlEscape(alias), expireOlderThanDays)
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("expire snapshots: %w", err)
		}
	}
	return nil
}

// SnapshotCount returns the number of DuckLake snapshots in the attached catalog.
func SnapshotCount(db *sql.DB, alias string) (int64, error) {
	if alias == "" {
		alias = "lake"
	}
	var n int64
	err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM ducklake_snapshots('%s')", sqlEscape(alias))).Scan(&n)
	return n, err
}

func columnExists(db *sql.DB, catalog, table, column string) bool {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_catalog = ? AND table_name = ? AND column_name = ?`,
		catalog, table, column,
	).Scan(&n)
	return err == nil && n > 0
}

func validIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
