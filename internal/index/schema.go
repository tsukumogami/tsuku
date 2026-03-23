package index

import (
	"database/sql"
	"fmt"
)

// initSchema creates the binary index tables if they do not already exist.
// It is safe to call on a database that already has the schema (idempotent).
func initSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS binaries (
			command      TEXT NOT NULL,
			recipe       TEXT NOT NULL,
			binary_path  TEXT NOT NULL,
			source       TEXT NOT NULL,
			installed    INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (command, recipe)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_command ON binaries(command)`,
		`CREATE TABLE IF NOT EXISTS meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("index: schema init failed: %w", err)
		}
	}

	return nil
}
