package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestRemodel_NoMalformedSchema rebuilds a realistic 0.39.5-shaped
// battlepass_accounts (account_id PK, NO server_id) and asserts the result is a
// well-formed schema (PRAGMA integrity_check = ok and a subsequent DDL parses).
// Guards against the DROP+RENAME rebuild corrupting sqlite_master.
func TestRemodel_NoMalformedSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bp.db")

	seed, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatalf("open seed: %v", err)
	}
	for _, s := range []string{
		`CREATE TABLE battlepass_accounts (account_id INTEGER PRIMARY KEY, baselined_at TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO battlepass_accounts (account_id, baselined_at) VALUES (7, '2024-01-01')`,
	} {
		if _, err := seed.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
	_ = seed.Close()

	db, err := openUnifiedStore(path)
	if err != nil {
		t.Fatalf("openUnifiedStore: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(
		`INSERT INTO servers (id, name, position, created_at, updated_at) VALUES (1,'D',0,'','')`); err != nil {
		t.Fatalf("insert server: %v", err)
	}
	migrateUnifiedRemodel(db, 1)

	var ic string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&ic); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if ic != "ok" {
		t.Fatalf("integrity_check = %q, want ok (schema corrupted by rebuild)", ic)
	}
	// A fresh DDL must still parse (this is what failed for the operator).
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS _probe (x INTEGER)`); err != nil {
		t.Fatalf("post-rebuild DDL failed (malformed schema?): %v", err)
	}
	// Row converted + stamped.
	var sid, acct int
	if err := db.QueryRow(`SELECT server_id, account_id FROM battlepass_accounts`).Scan(&sid, &acct); err != nil {
		t.Fatalf("read converted: %v", err)
	}
	if sid != 1 || acct != 7 {
		t.Errorf("converted row = %d/%d, want 1/7", sid, acct)
	}
}
