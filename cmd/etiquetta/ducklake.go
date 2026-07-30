package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/caioricciuti/etiquetta/internal/ducklake"
)

var duckLakeMigrateCmd = &cobra.Command{
	Use:   "ducklake-migrate",
	Short: "Copy event data into a DuckLake catalog (non-destructive evaluation)",
	Long: `Copies events, performance, errors, and visitor_sessions from the current
DuckDB into a DuckLake catalog under {data-dir}/lake, verifying row counts.

The source database is opened READ-ONLY and never modified — this only builds a
DuckLake copy alongside it for evaluation. Stop the server first so the database
is not locked.`,
	RunE:         runDuckLakeMigrate,
	SilenceUsage: true,
}

func runDuckLakeMigrate(cmd *cobra.Command, args []string) error {
	srcPath := filepath.Join(dataDir, "etiquetta.duckdb")
	if _, err := os.Stat(srcPath); err != nil {
		return fmt.Errorf("no database found at %s", srcPath)
	}

	lakeDir := filepath.Join(dataDir, "lake")
	dataPath := filepath.Join(lakeDir, "data")
	if err := os.MkdirAll(dataPath, 0o755); err != nil {
		return fmt.Errorf("create lake directory: %w", err)
	}

	db, err := sql.Open("duckdb", "")
	if err != nil {
		return fmt.Errorf("open working database: %w", err)
	}
	defer db.Close()

	cfg := ducklake.Config{
		CatalogPath: filepath.Join(lakeDir, "catalog.ducklake"),
		DataPath:    dataPath,
	}

	fmt.Println("Attaching DuckLake catalog (installing extensions if needed)...")
	if err := ducklake.Attach(db, cfg); err != nil {
		return fmt.Errorf("attach DuckLake: %w", err)
	}

	fmt.Println("Copying event tables (source is read-only)...")
	counts, err := ducklake.MigrateFromDuckDB(db, srcPath, ducklake.EventTables)
	if err != nil {
		return fmt.Errorf("migration failed (source unchanged): %w", err)
	}

	for _, c := range counts {
		mark := "ok"
		if c.Source != c.Lake {
			mark = "MISMATCH"
		}
		fmt.Printf("  %-18s %8d rows -> lake %8d  [%s]\n", c.Table, c.Source, c.Lake, mark)
	}

	snaps, _ := ducklake.SnapshotCount(db, "lake")
	fmt.Println()
	fmt.Printf("Done. DuckLake catalog at %s (%d snapshots).\n", lakeDir, snaps)
	fmt.Println("Your original database was not modified.")
	return nil
}
