package main

import (
	"database/sql"
	"fmt"
)

// app_permissions_store.go holds the app-level (global, no server_id) DB schema
// + store funcs for the role→capability permissions matrix, moved off the legacy
// permissions.yaml file. Fully relational: one row per (role_id, capability).

// initAppPermissionsSchema creates the app_role_capabilities table. This is
// global dashboard config — not per-server — so it has no FK to servers.
// Idempotent.
func initAppPermissionsSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS app_role_capabilities (
			role_id    TEXT NOT NULL,
			capability TEXT NOT NULL,
			PRIMARY KEY (role_id, capability)
		)`)
	return err
}

// loadPermissionMatrix reads the full role→capability matrix from the DB.
// ok=false when the table is empty (no rows) so the caller can seed defaults.
func loadPermissionMatrix(db *sql.DB) (map[string][]string, bool, error) {
	rows, err := db.Query(`SELECT role_id, capability FROM app_role_capabilities ORDER BY role_id, capability`)
	if err != nil {
		return nil, false, fmt.Errorf("load permission matrix: %w", err)
	}
	defer func() { _ = rows.Close() }()
	matrix := map[string][]string{}
	for rows.Next() {
		var role, cap string
		if err := rows.Scan(&role, &cap); err != nil {
			return nil, false, fmt.Errorf("scan permission row: %w", err)
		}
		matrix[role] = append(matrix[role], cap)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return matrix, len(matrix) > 0, nil
}

// savePermissionMatrix replaces the entire matrix in a single transaction.
func savePermissionMatrix(db *sql.DB, matrix map[string][]string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM app_role_capabilities`); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("clear permission matrix: %w", err)
	}
	for role, caps := range matrix {
		for _, cap := range caps {
			if _, err := tx.Exec(
				`INSERT OR IGNORE INTO app_role_capabilities (role_id, capability) VALUES (?, ?)`,
				role, cap); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("insert permission %s/%s: %w", role, cap, err)
			}
		}
	}
	return tx.Commit()
}
