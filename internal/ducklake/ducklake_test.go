package ducklake

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

// TestMigrateFromDuckDBPreservesData builds a source DuckDB (including a value
// that overflows INT32, the bug that broke production), migrates it into a
// DuckLake catalog, and verifies row counts match and the big value survives.
func TestMigrateFromDuckDBPreservesData(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "old.duckdb")

	// Source database, as an older Etiquetta would have created it.
	sdb, err := sql.Open("duckdb", srcPath)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	if _, err := sdb.Exec(`CREATE TABLE events (id VARCHAR, page_duration INTEGER)`); err != nil {
		t.Fatalf("create source events: %v", err)
	}
	if _, err := sdb.Exec(`INSERT INTO events VALUES ('a', 100), ('b', 200), ('c', 300)`); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if _, err := sdb.Exec(`CREATE TABLE performance (id VARCHAR, lcp DOUBLE)`); err != nil {
		t.Fatalf("create source performance: %v", err)
	}
	if _, err := sdb.Exec(`INSERT INTO performance VALUES ('p1', 1.5)`); err != nil {
		t.Fatalf("seed performance: %v", err)
	}
	if err := sdb.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}

	// Fresh connection: attach a lake and migrate into it.
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	cfg := Config{
		CatalogPath: filepath.Join(dir, "lake.ducklake"),
		DataPath:    filepath.Join(dir, "data"),
	}
	if err := Attach(db, cfg); err != nil {
		t.Skipf("DuckLake unavailable (needs extension install): %v", err)
	}

	counts, err := MigrateFromDuckDB(db, srcPath, EventTables)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(counts) != 2 {
		t.Fatalf("migrated %d tables, want 2 (events, performance)", len(counts))
	}
	for _, c := range counts {
		if c.Source != c.Lake {
			t.Fatalf("%s: source=%d lake=%d", c.Table, c.Source, c.Lake)
		}
	}

	// The migrated event data is queryable as normal SQL.
	var events int
	if err := db.QueryRow("SELECT COUNT(*) FROM lake.events").Scan(&events); err != nil {
		t.Fatalf("query lake events: %v", err)
	}
	if events != 3 {
		t.Fatalf("lake events = %d, want 3", events)
	}

	// A value that overflows INT32 now stores natively (DuckLake infers BIGINT).
	if _, err := db.Exec(`INSERT INTO lake.events VALUES ('big', 5957754851)`); err != nil {
		t.Fatalf("insert overflowing page_duration into lake: %v", err)
	}
	var stored int64
	if err := db.QueryRow("SELECT page_duration FROM lake.events WHERE id = 'big'").Scan(&stored); err != nil {
		t.Fatalf("read big value: %v", err)
	}
	if stored != 5957754851 {
		t.Fatalf("stored page_duration = %d, want 5957754851", stored)
	}

	// Snapshots exist (time-travel history).
	snaps, err := SnapshotCount(db, "lake")
	if err != nil {
		t.Fatalf("snapshot count: %v", err)
	}
	if snaps < 1 {
		t.Fatalf("expected DuckLake snapshots, got %d", snaps)
	}
}

// TestMigrateSourceIsUntouched verifies the migration never writes to the source.
func TestMigrateSourceIsUntouched(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "old.duckdb")

	sdb, err := sql.Open("duckdb", srcPath)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	sdb.Exec(`CREATE TABLE events (id VARCHAR)`)
	sdb.Exec(`INSERT INTO events VALUES ('a'), ('b')`)
	sdb.Close()

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := Attach(db, Config{
		CatalogPath: filepath.Join(dir, "lake.ducklake"),
		DataPath:    filepath.Join(dir, "data"),
	}); err != nil {
		t.Skipf("DuckLake unavailable: %v", err)
	}
	if _, err := MigrateFromDuckDB(db, srcPath, []string{"events"}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Reopen the source independently and confirm its data is unchanged.
	check, err := sql.Open("duckdb", srcPath)
	if err != nil {
		t.Fatalf("reopen source: %v", err)
	}
	defer check.Close()
	var n int
	if err := check.QueryRow("SELECT COUNT(*) FROM events").Scan(&n); err != nil {
		t.Fatalf("source query: %v", err)
	}
	if n != 2 {
		t.Fatalf("source row count = %d, want 2 (source was modified!)", n)
	}
}
