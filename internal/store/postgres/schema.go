package postgres

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed schema.sql
var schemaSQL string

// EnsureSchema applies the MIDAS schema to the database.
// schema.sql is written with idempotent DDL (CREATE TABLE IF NOT EXISTS,
// CREATE INDEX IF NOT EXISTS, CREATE OR REPLACE VIEW) so this function is
// safe to call on every startup against an already-initialised database.
//
// This is intentionally a simple bootstrap mechanism, not a migration system.
// schema.sql is the single source of truth for the database structure.
func EnsureSchema(db *sql.DB) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}
