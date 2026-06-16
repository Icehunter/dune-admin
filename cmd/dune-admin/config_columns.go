package main

import "database/sql"

// This file holds shared helpers for the typed-column config stores that
// replace the legacy JSON-blob columns (app_settings.config_json, etc.).

// boolPtrToNullInt encodes a tri-state *bool for a nullable INTEGER column:
// nil → NULL, false → 0, true → 1. Pointer bools distinguish "unset"
// (feature default) from an explicit false, so the NULL state must survive.
func boolPtrToNullInt(b *bool) sql.NullInt64 {
	if b == nil {
		return sql.NullInt64{}
	}
	if *b {
		return sql.NullInt64{Int64: 1, Valid: true}
	}
	return sql.NullInt64{Int64: 0, Valid: true}
}

// nullIntToBoolPtr is the inverse of boolPtrToNullInt.
func nullIntToBoolPtr(n sql.NullInt64) *bool {
	if !n.Valid {
		return nil
	}
	v := n.Int64 != 0
	return &v
}

// runColumnMigrationOnce runs fn inside a single transaction iff the meta
// marker is unset, then sets the marker as the last write. Idempotent: a
// present marker is a no-op, and a crash before commit leaves the marker
// unset so the migration re-runs cleanly on the next boot.
func runColumnMigrationOnce(db *sql.DB, marker string, fn func(*sql.Tx) error) error {
	existing, err := metaGet(db, marker)
	if err != nil {
		return err
	}
	if existing != "" {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO meta(key,value) VALUES(?,?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		marker, "done"); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
