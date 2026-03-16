package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

// migrationTables defines the order for migrating tables from SQLite to DuckDB.
// Config tables first, then data tables.
var migrationTables = []string{
	"settings",
	"users",
	"sessions",
	"domains",
	"campaigns",
	"consent_configs",
	"tm_containers",
	"tm_tags",
	"tm_triggers",
	"tm_tag_triggers",
	"tm_variables",
	"tm_snapshots",
	"events",
	"performance",
	"errors",
	"consent_records",
	"visitor_sessions",
	"audit_log",
}

// NeedsSQLiteMigration checks if a SQLite database exists and DuckDB doesn't.
func NeedsSQLiteMigration(dataDir string) (sqlitePath string, needed bool) {
	sqlitePath = dataDir + "/etiquetta.db"
	duckdbPath := dataDir + "/etiquetta.duckdb"

	_, sqliteErr := os.Stat(sqlitePath)
	_, duckdbErr := os.Stat(duckdbPath)

	return sqlitePath, sqliteErr == nil && os.IsNotExist(duckdbErr)
}

// MigrateSQLite migrates data from a SQLite database to the current DuckDB database.
// Uses DuckDB's built-in sqlite_scanner extension.
func MigrateSQLite(db *sql.DB, sqlitePath string) error {
	log.Println("[migration] Starting SQLite -> DuckDB migration...")

	// Install and load the SQLite scanner extension
	if _, err := db.Exec("INSTALL sqlite; LOAD sqlite;"); err != nil {
		return fmt.Errorf("failed to load sqlite extension: %w", err)
	}

	totalRows := int64(0)

	for _, table := range migrationTables {
		// Check if the table exists in SQLite by attempting a scan
		var count int64
		err := db.QueryRow(
			fmt.Sprintf("SELECT COUNT(*) FROM sqlite_scan('%s', '%s')", sqlitePath, table),
		).Scan(&count)
		if err != nil {
			// Table doesn't exist in SQLite — skip
			log.Printf("[migration] Skipping %s (not found in SQLite)", table)
			continue
		}

		if count == 0 {
			log.Printf("[migration] Skipping %s (0 rows)", table)
			continue
		}

		// Copy data from SQLite to DuckDB (ON CONFLICT DO NOTHING makes this idempotent)
		_, err = db.Exec(
			fmt.Sprintf("INSERT INTO %s SELECT * FROM sqlite_scan('%s', '%s') ON CONFLICT DO NOTHING", table, sqlitePath, table),
		)
		if err != nil {
			log.Printf("[migration] Warning: failed to migrate %s: %v", table, err)
			continue
		}

		totalRows += count
		log.Printf("[migration] Migrated %s: %d rows", table, count)
	}

	// Rename the old SQLite file to .bak
	bakPath := sqlitePath + ".bak"
	if err := os.Rename(sqlitePath, bakPath); err != nil {
		log.Printf("[migration] Warning: could not rename %s to %s: %v", sqlitePath, bakPath, err)
	} else {
		log.Printf("[migration] Renamed %s -> %s", sqlitePath, bakPath)
	}

	// Also rename WAL and SHM files if they exist
	os.Rename(sqlitePath+"-wal", bakPath+"-wal")
	os.Rename(sqlitePath+"-shm", bakPath+"-shm")

	log.Printf("[migration] SQLite -> DuckDB migration complete: %d total rows migrated", totalRows)
	return nil
}
