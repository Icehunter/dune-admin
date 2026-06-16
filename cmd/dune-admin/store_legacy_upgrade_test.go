package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestApplyUnifiedSchema_LegacyTablesNoServerID reproduces the 0.39.5 upgrade
// crash: pre-scoping tables exist WITHOUT a server_id column, so the per-table
// schema init must not create a server_id index on them (it would fail with
// "no such column: server_id" and drop the whole store to the legacy fallback).
// After the schema applies, migrateUnifiedRemodel must convert them to int-FK
// scoped tables stamped to the default server's id, with the index in place.
func TestApplyUnifiedSchema_LegacyTablesNoServerID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// 1. Seed a 0.39.5-shaped store: legacy tables with NO server_id column.
	seed, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatalf("open seed: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE play_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT, account_id INTEGER NOT NULL,
			started_at TEXT NOT NULL, ended_at TEXT, duration_secs INTEGER)`,
		`INSERT INTO play_sessions (account_id, started_at) VALUES (42, '2024-01-01T00:00:00Z')`,
		`CREATE TABLE welcome_grants (
			fls_id TEXT NOT NULL, package_version TEXT NOT NULL, account_id INTEGER NOT NULL,
			character_name TEXT NOT NULL DEFAULT '', status TEXT NOT NULL,
			granted_at TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 1,
			last_error TEXT NOT NULL DEFAULT '', detected_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (fls_id, package_version, account_id))`,
		`INSERT INTO welcome_grants (fls_id, package_version, account_id, status) VALUES ('FLS1','v1',42,'granted')`,
	} {
		if _, err := seed.Exec(stmt); err != nil {
			t.Fatalf("seed exec %q: %v", stmt, err)
		}
	}
	_ = seed.Close()

	// 2. Open through the real path — applyUnifiedSchema must NOT error on the
	//    legacy tables (this is the regression: previously it crashed here).
	db, err := openUnifiedStore(path)
	if err != nil {
		t.Fatalf("openUnifiedStore on legacy DB: %v", err)
	}
	defer func() { _ = db.Close() }()

	// 3. A server must exist before the remodel (the boot does this via the
	//    config.yaml import); simulate it.
	if _, err := db.Exec(
		`INSERT INTO servers (id, name, position, created_at, updated_at)
		 VALUES (1, 'Default', 0, '', '')`); err != nil {
		t.Fatalf("insert server: %v", err)
	}

	// 4. Run the remodel and assert the legacy rows are converted + stamped.
	migrateUnifiedRemodel(db, 1)

	if typ, _ := columnType(db, "play_sessions", "server_id"); typ != "INTEGER" {
		t.Fatalf("play_sessions.server_id type = %q, want INTEGER", typ)
	}
	var sid, acct int
	if err := db.QueryRow(`SELECT server_id, account_id FROM play_sessions`).Scan(&sid, &acct); err != nil {
		t.Fatalf("read converted play_sessions: %v", err)
	}
	if sid != 1 || acct != 42 {
		t.Errorf("converted row = server_id %d acct %d, want 1/42", sid, acct)
	}
	var gid int
	if err := db.QueryRow(`SELECT server_id FROM welcome_grants WHERE fls_id='FLS1'`).Scan(&gid); err != nil {
		t.Fatalf("read converted welcome_grants: %v", err)
	}
	if gid != 1 {
		t.Errorf("welcome_grants server_id = %d, want 1", gid)
	}

	// server_id index now exists.
	var idxName string
	if err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_ps_server'`).Scan(&idxName); err != nil {
		t.Errorf("idx_ps_server not created after remodel: %v", err)
	}

	// Cascade is live: deleting the server purges the converted rows.
	if _, err := db.Exec(`DELETE FROM servers WHERE id = 1`); err != nil {
		t.Fatalf("delete server: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM play_sessions`).Scan(&n); err != nil {
		t.Fatalf("count after cascade: %v", err)
	}
	if n != 0 {
		t.Errorf("play_sessions not cascaded on server delete: %d rows", n)
	}
}
