package main

import (
	"database/sql"
	"testing"
)

// legacy0395SchemaSQL recreates the 0.39.5 store shape: scoped tables with NO
// server_id (single-server) and JSON blobs on welcome_config / give_packs_config.
// The unified `servers` + `meta` tables are added so the remodel migration has a
// parent server to scope to.
const legacy0395SchemaSQL = `
CREATE TABLE servers (
	id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL DEFAULT '',
	position INTEGER NOT NULL DEFAULT 0, created_at TEXT, updated_at TEXT
);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE play_sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT, account_id INTEGER NOT NULL,
	started_at TEXT NOT NULL, ended_at TEXT, duration_secs INTEGER
);
CREATE TABLE stat_snapshots (
	id INTEGER PRIMARY KEY AUTOINCREMENT, account_id INTEGER NOT NULL,
	snapped_at TEXT NOT NULL, char_xp INTEGER, skill_points INTEGER, intel_points INTEGER,
	combat_xp INTEGER, crafting_xp INTEGER, gathering_xp INTEGER, exploration_xp INTEGER,
	sabotage_xp INTEGER, solaris_balance INTEGER
);
CREATE TABLE welcome_grants (
	fls_id TEXT NOT NULL, package_version TEXT NOT NULL, account_id INTEGER NOT NULL,
	character_name TEXT NOT NULL DEFAULT '', status TEXT NOT NULL,
	granted_at TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 1,
	last_error TEXT NOT NULL DEFAULT '', detected_at TEXT NOT NULL, updated_at TEXT NOT NULL,
	PRIMARY KEY (fls_id, package_version, account_id)
);
CREATE TABLE welcome_config (
	id INTEGER PRIMARY KEY CHECK (id = 1), enabled INTEGER NOT NULL DEFAULT 0,
	scan_secs INTEGER NOT NULL DEFAULT 30, active_version TEXT NOT NULL DEFAULT '',
	active_versions_json TEXT NOT NULL DEFAULT '', packages_json TEXT NOT NULL DEFAULT '[]',
	welcome_message_enabled INTEGER NOT NULL DEFAULT 0, welcome_message TEXT NOT NULL DEFAULT '',
	welcome_whisper_source_player TEXT NOT NULL DEFAULT '',
	motd_enabled INTEGER NOT NULL DEFAULT 0, motd_message TEXT NOT NULL DEFAULT '',
	motd_source_player TEXT NOT NULL DEFAULT '',
	region_join_enabled INTEGER NOT NULL DEFAULT 0, region_leave_enabled INTEGER NOT NULL DEFAULT 0,
	region_join_template TEXT NOT NULL DEFAULT '', region_leave_template TEXT NOT NULL DEFAULT '',
	region_chat_channel TEXT NOT NULL DEFAULT 'whisper', updated_at TEXT NOT NULL
);
CREATE TABLE give_packs_config (
	id INTEGER PRIMARY KEY CHECK (id = 1), base_packs_loaded INTEGER NOT NULL DEFAULT 0,
	packs_json TEXT NOT NULL DEFAULT '[]', updated_at TEXT NOT NULL
);
CREATE TABLE event_definitions (
	id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL DEFAULT 1,
	config_json TEXT NOT NULL DEFAULT '{}', reward_json TEXT NOT NULL DEFAULT '',
	announce_channel_id TEXT NOT NULL DEFAULT '', announce_template TEXT NOT NULL DEFAULT '',
	poll_seconds INTEGER NOT NULL DEFAULT 7, jitter_seconds INTEGER NOT NULL DEFAULT 3,
	created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE event_award_claims (
	event_id INTEGER NOT NULL, version INTEGER NOT NULL, account_id INTEGER NOT NULL,
	status TEXT NOT NULL, claimed_at TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 1,
	last_error TEXT NOT NULL DEFAULT '', next_attempt_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL,
	PRIMARY KEY (event_id, version, account_id)
);
CREATE TABLE battlepass_tiers (
	id INTEGER PRIMARY KEY AUTOINCREMENT, tier_key TEXT NOT NULL UNIQUE, category TEXT NOT NULL,
	label TEXT NOT NULL, signal TEXT NOT NULL, signal_key TEXT NOT NULL DEFAULT '',
	threshold INTEGER NOT NULL DEFAULT 0, intel INTEGER NOT NULL DEFAULT 0,
	enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE battlepass_claims (
	tier_key TEXT NOT NULL, account_id INTEGER NOT NULL, status TEXT NOT NULL,
	intel INTEGER NOT NULL DEFAULT 0, earned_at TEXT NOT NULL DEFAULT '',
	granted_at TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 0,
	last_error TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL,
	PRIMARY KEY (tier_key, account_id)
);
CREATE TABLE battlepass_accounts (account_id INTEGER PRIMARY KEY, baselined_at TEXT NOT NULL);
CREATE TABLE battlepass_grant_ledger (
	tier_key TEXT NOT NULL, account_id INTEGER NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
	attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '',
	next_attempt_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL,
	PRIMARY KEY (tier_key, account_id)
);
`

func seedLegacy0395Rows(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`INSERT INTO servers(name, position) VALUES ('Default', 0)`,
		`INSERT INTO play_sessions(account_id, started_at, ended_at, duration_secs) VALUES (42, '2024-01-01T00:00:00Z', '2024-01-01T01:00:00Z', 3600)`,
		`INSERT INTO stat_snapshots(account_id, snapped_at, char_xp) VALUES (42, '2024-01-01T00:00:00Z', 1000)`,
		`INSERT INTO welcome_grants(fls_id,package_version,account_id,status,detected_at,updated_at) VALUES ('fls1','v1',42,'granted','2024-01-01T00:00:00Z','2024-01-01T00:00:00Z')`,
		`INSERT INTO welcome_config(id,enabled,packages_json,active_versions_json,updated_at) VALUES (1,1,'[{"version":"v1","items":[{"template":"weapon","qty":2,"quality":3}]}]','["v1"]','2024-01-01T00:00:00Z')`,
		`INSERT INTO give_packs_config(id,packs_json,updated_at) VALUES (1,'[{"id":"starter","name":"Starter","items":[{"template":"water","qty":5}]}]','2024-01-01T00:00:00Z')`,
		`INSERT INTO event_definitions(name,type,enabled,created_at,updated_at) VALUES ('test','milestone',1,'2024-01-01T00:00:00Z','2024-01-01T00:00:00Z')`,
		`INSERT INTO event_award_claims(event_id,version,account_id,status,updated_at) VALUES (1,1,42,'granted','2024-01-01T00:00:00Z')`,
		`INSERT INTO battlepass_tiers(tier_key,category,label,signal,created_at,updated_at) VALUES ('level:5','level','Level 5','level','2024-01-01T00:00:00Z','2024-01-01T00:00:00Z')`,
		`INSERT INTO battlepass_claims(tier_key,account_id,status,updated_at) VALUES ('level:5',42,'earned','2024-01-01T00:00:00Z')`,
		`INSERT INTO battlepass_accounts(account_id,baselined_at) VALUES (42,'2024-01-01T00:00:00Z')`,
		`INSERT INTO battlepass_grant_ledger(tier_key,account_id,updated_at) VALUES ('level:5',42,'2024-01-01T00:00:00Z')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v\nSQL: %s", err, s)
		}
	}
}

func countRowsWhere(t *testing.T, db *sql.DB, table, where string, args ...any) int {
	t.Helper()
	var n int
	q := "SELECT COUNT(*) FROM " + table
	if where != "" {
		q += " WHERE " + where
	}
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("countRowsWhere(%s): %v", table, err)
	}
	return n
}

// TestMigrateUnifiedRemodel_From0395 verifies the one-way 0.39.5 → unified
// migration: legacy single-server scoped tables get an int server_id stamped to
// the default server, JSON blobs are decomposed into the surrogate-id child
// tables, and re-running is idempotent (no duplicate rows, no errors).
func TestMigrateUnifiedRemodel_From0395(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(legacy0395SchemaSQL); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	seedLegacy0395Rows(t, db)

	// Create the surrogate-id child tables (these don't exist in 0.39.5; in
	// production applyUnifiedSchema makes them before the remodel runs).
	if err := initWelcomeColumnsSchema(db); err != nil {
		t.Fatalf("welcome columns schema: %v", err)
	}
	if err := initGivePacksColumnsSchema(db); err != nil {
		t.Fatalf("give columns schema: %v", err)
	}

	defaultID := 1
	migrateUnifiedRemodel(db, defaultID)

	// Scoped tables now carry int server_id = defaultID.
	for _, tbl := range []string{
		"play_sessions", "stat_snapshots", "welcome_grants", "welcome_config",
		"give_packs_config", "event_award_claims", "battlepass_claims",
		"battlepass_accounts", "battlepass_grant_ledger",
	} {
		typ, err := columnType(db, tbl, "server_id")
		if err != nil {
			t.Fatalf("columnType(%s): %v", tbl, err)
		}
		if typ != "INTEGER" {
			t.Errorf("%s.server_id type = %q, want INTEGER", tbl, typ)
		}
		if n := countRowsWhere(t, db, tbl, "server_id = ?", defaultID); n == 0 {
			t.Errorf("%s: no rows scoped to default server after migration", tbl)
		}
	}
	// Catalog tables gained an int server_id stamped to default.
	for _, tbl := range []string{"event_definitions", "battlepass_tiers"} {
		if n := countRowsWhere(t, db, tbl, "server_id = ?", defaultID); n == 0 {
			t.Errorf("%s: no rows scoped to default server after migration", tbl)
		}
	}

	// Welcome blob decomposed into surrogate child tables.
	if n := countRowsWhere(t, db, "welcome_packages", "server_id = ?", defaultID); n != 1 {
		t.Errorf("welcome_packages = %d, want 1", n)
	}
	if n := countRowsWhere(t, db, "welcome_package_items", "", nil...); n != 1 {
		t.Errorf("welcome_package_items = %d, want 1", n)
	}
	if n := countRowsWhere(t, db, "welcome_active_versions", "server_id = ?", defaultID); n != 1 {
		t.Errorf("welcome_active_versions = %d, want 1", n)
	}
	// Give-packs blob decomposed.
	if n := countRowsWhere(t, db, "give_packs", "server_id = ?", defaultID); n != 1 {
		t.Errorf("give_packs = %d, want 1", n)
	}
	if n := countRowsWhere(t, db, "give_pack_items", "", nil...); n != 1 {
		t.Errorf("give_pack_items = %d, want 1", n)
	}

	// AFTER decomposition the welcome/give-packs vestigial blob columns are dropped.
	for _, tc := range []struct{ table, col string }{
		{"give_packs_config", "packs_json"},
		{"welcome_config", "packages_json"},
		{"welcome_config", "active_versions_json"},
	} {
		if typ, _ := columnType(db, tc.table, tc.col); typ != "" {
			t.Errorf("%s.%s should be dropped after migration, got %q", tc.table, tc.col, typ)
		}
	}

	// Event config/reward are opaque frontend-owned documents — they STAY as JSON
	// columns (decomposing them would drop fields the backend doesn't model, e.g.
	// reward.faction_scrip).
	for _, col := range []string{"config_json", "reward_json"} {
		if typ, _ := columnType(db, "event_definitions", col); typ == "" {
			t.Errorf("event_definitions.%s should be kept (opaque JSON), but was dropped", col)
		}
	}

	// Idempotency: re-run must not change row counts or error on dropped columns.
	before := countRowsWhere(t, db, "welcome_packages", "", nil...)
	migrateUnifiedRemodel(db, defaultID)
	if after := countRowsWhere(t, db, "welcome_packages", "", nil...); after != before {
		t.Errorf("welcome_packages changed on re-run: %d → %d", before, after)
	}
}

// TestApplyUnifiedSchema_FreshInstall verifies a fresh unified store creates all
// scoped tables with an INTEGER server_id and no legacy blob columns.
func TestApplyUnifiedSchema_FreshInstall(t *testing.T) {
	db, err := openUnifiedStore(":memory:")
	if err != nil {
		t.Fatalf("openUnifiedStore: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, tbl := range []string{
		"welcome_grants", "welcome_config", "give_packs_config",
		"event_award_claims", "battlepass_claims", "play_sessions", "stat_snapshots",
	} {
		typ, err := columnType(db, tbl, "server_id")
		if err != nil {
			t.Fatalf("columnType(%s): %v", tbl, err)
		}
		if typ != "INTEGER" {
			t.Errorf("fresh %s.server_id = %q, want INTEGER", tbl, typ)
		}
	}
	// Vestigial welcome/give-packs blob columns are gone on a fresh install.
	for _, tc := range []struct{ table, col string }{
		{"welcome_config", "packages_json"},
		{"welcome_config", "active_versions_json"},
		{"give_packs_config", "packs_json"},
	} {
		if typ, _ := columnType(db, tc.table, tc.col); typ != "" {
			t.Errorf("fresh %s should not have %s", tc.table, tc.col)
		}
	}
	// Event config/reward stay as opaque JSON columns (frontend-owned documents).
	for _, col := range []string{"config_json", "reward_json"} {
		if typ, _ := columnType(db, "event_definitions", col); typ == "" {
			t.Errorf("fresh event_definitions should keep %s (opaque JSON)", col)
		}
	}
}
