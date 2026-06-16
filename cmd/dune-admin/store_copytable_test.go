package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestCopyTable_ColumnDrift reproduces the 0.39.5 legacy-import warning: the
// standalone legacy file's table has more columns than the unified target
// (or vice-versa). copyTable must copy the intersecting columns instead of
// failing with "N columns but M values supplied".
func TestCopyTable_ColumnDrift(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.db")
	legacyPath := filepath.Join(dir, "legacy.db")

	// Target (unified) play_sessions: 5 columns, no server_id (0.39.5 shape).
	main, err := sql.Open("sqlite", "file:"+mainPath+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatalf("open main: %v", err)
	}
	t.Cleanup(func() { _ = main.Close() })
	if _, err := main.Exec(`CREATE TABLE play_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT, account_id INTEGER NOT NULL,
		started_at TEXT NOT NULL, ended_at TEXT, duration_secs INTEGER)`); err != nil {
		t.Fatalf("create target: %v", err)
	}

	// Legacy source play_sessions: 6 columns (extra server_id) + a row.
	legacy, err := sql.Open("sqlite", "file:"+legacyPath+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	if _, err := legacy.Exec(`CREATE TABLE play_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT, server_id TEXT, account_id INTEGER NOT NULL,
		started_at TEXT NOT NULL, ended_at TEXT, duration_secs INTEGER)`); err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	if _, err := legacy.Exec(
		`INSERT INTO play_sessions (server_id, account_id, started_at) VALUES ('default', 99, '2024-02-02')`); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	_ = legacy.Close()

	if _, err := main.Exec(`ATTACH DATABASE ? AS legacy_src`, "file:"+legacyPath+"?mode=ro"); err != nil {
		t.Fatalf("attach: %v", err)
	}

	// Previously failed with "5 columns but 6 values"; now copies the 5 shared.
	if err := copyTable(main, "legacy_src", "play_sessions"); err != nil {
		t.Fatalf("copyTable: %v", err)
	}
	var acct int
	if err := main.QueryRow(`SELECT account_id FROM play_sessions`).Scan(&acct); err != nil {
		t.Fatalf("read copied row: %v", err)
	}
	if acct != 99 {
		t.Errorf("copied account_id = %d, want 99", acct)
	}
}
