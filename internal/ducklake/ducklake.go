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

// EventTables are the high-write tables the ingestion path manages. These are
// where DuckLake's Parquet storage and time-travel matter most.
var EventTables = []string{"events", "performance", "errors", "visitor_sessions"}

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
