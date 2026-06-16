package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// columnType returns the declared type of col in table, or "" if absent.
func columnType(db *sql.DB, table, col string) (string, error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table)) // #nosec G201 -- table is an internal literal
	if err != nil {
		return "", fmt.Errorf("PRAGMA table_info(%s): %w", table, err)
	}
	return scanColumnType(rows, col)
}

// columnTypeTx is columnType against an open transaction so the presence check
// observes the same connection's pending schema state.
func columnTypeTx(tx *sql.Tx, table, col string) (string, error) {
	rows, err := tx.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table)) // #nosec G201 -- table is an internal literal
	if err != nil {
		return "", fmt.Errorf("PRAGMA table_info(%s): %w", table, err)
	}
	return scanColumnType(rows, col)
}

// scanColumnType walks a PRAGMA table_info result set looking for col, returning
// its declared type or "" if absent. Closes rows.
func scanColumnType(rows *sql.Rows, col string) (string, error) {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return "", err
		}
		if name == col {
			return colType, nil
		}
	}
	return "", rows.Err()
}

// rebuildLegacyServerIDToInt converts a 0.39.5-shaped table whose server_id is
// TEXT (or absent) to the int-FK schema, stamping defaultID on every existing
// row. It is used by the one-way 0.39.5 → unified migration; fresh installs
// never hit it because their tables are created with int server_id already.
//
// newDDL must CREATE tmpName with the final int-FK schema. cols is the ordered
// list of data columns (server_id excluded) shared between old and new tables.
// Runs FK-disabled on a single connection so the temporary orphan during the
// drop/rename never trips foreign-key enforcement.
func rebuildLegacyServerIDToInt(db *sql.DB, table, tmpName, newDDL string, cols []string, defaultID int) error {
	typ, err := columnType(db, table, "server_id")
	if err != nil {
		return err
	}
	// Already int (fresh install or prior migration) — nothing to do.
	if typ == "INTEGER" {
		return nil
	}
	colList := strings.Join(cols, ", ")
	// Source expression for the new integer server_id:
	//   - legacy table has NO server_id (0.39.5 pre-scoping, typ == ""): stamp the
	//     default id — the column doesn't exist to read.
	//   - legacy table has a TEXT server_id (0.40.0 multi-server): preserve a
	//     numeric scope ("1","2",…) and map a non-numeric/'default' scope to the
	//     default id. Without this, multi-server data collapses onto one server and
	//     composite-PK tables abort on the resulting duplicate key.
	scopeExpr := fmt.Sprintf("%d", defaultID)
	if typ != "" {
		scopeExpr = fmt.Sprintf("CASE WHEN server_id GLOB '[0-9]*' THEN CAST(server_id AS INTEGER) ELSE %d END", defaultID)
	}
	return withForeignKeysDisabled(context.Background(), db, func(conn *sql.Conn) error {
		ctx := context.Background()
		if _, err := conn.ExecContext(ctx, newDDL); err != nil {
			return fmt.Errorf("rebuild %s: create %s: %w", table, tmpName, err)
		}
		if _, err := conn.ExecContext(ctx, fmt.Sprintf(
			`INSERT INTO %s (server_id, %s) SELECT %s, %s FROM %s`,
			tmpName, colList, scopeExpr, colList, table,
		)); err != nil {
			return fmt.Errorf("rebuild %s: copy rows: %w", table, err)
		}
		if _, err := conn.ExecContext(ctx, `DROP TABLE `+table); err != nil { // #nosec G201 -- internal literal
			return fmt.Errorf("rebuild %s: drop original: %w", table, err)
		}
		if _, err := conn.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s RENAME TO %s`, tmpName, table)); err != nil {
			return fmt.Errorf("rebuild %s: rename: %w", table, err)
		}
		return nil
	})
}
