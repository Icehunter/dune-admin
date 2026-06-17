package main

import (
	"database/sql"
	"testing"
)

// openRawMemDB opens a raw in-memory SQLite DB without any schema applied,
// used to simulate DBs from older versions.
func openRawMemDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatalf("open raw mem db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// ── Bug #2: welcome_config missing columns ────────────────────────────────────

// TestWelcomeConfigMissingColumns_AddsMissingCols verifies that
// initWelcomeConfigMissingColumns adds region_join_enabled and related columns
// to an old welcome_config that was created without them.
func TestWelcomeConfigMissingColumns_AddsMissingCols(t *testing.T) {
	t.Parallel()
	db := openRawMemDB(t)

	// Old schema: welcome_config without region columns.
	if _, err := db.Exec(`
		CREATE TABLE welcome_config (
			server_id                     INTEGER NOT NULL DEFAULT 1,
			enabled                       INTEGER NOT NULL DEFAULT 0,
			scan_secs                     INTEGER NOT NULL DEFAULT 30,
			active_version                TEXT    NOT NULL DEFAULT '',
			packages_json                 TEXT    NOT NULL DEFAULT '[]',
			welcome_message_enabled       INTEGER NOT NULL DEFAULT 0,
			welcome_message               TEXT    NOT NULL DEFAULT '',
			welcome_whisper_source_player TEXT    NOT NULL DEFAULT '',
			motd_enabled                  INTEGER NOT NULL DEFAULT 0,
			motd_message                  TEXT    NOT NULL DEFAULT '',
			motd_source_player            TEXT    NOT NULL DEFAULT '',
			updated_at                    TEXT    NOT NULL DEFAULT '',
			PRIMARY KEY (server_id)
		)
	`); err != nil {
		t.Fatalf("create old welcome_config: %v", err)
	}

	if err := initWelcomeConfigMissingColumns(db); err != nil {
		t.Fatalf("initWelcomeConfigMissingColumns: %v", err)
	}

	for _, col := range []string{
		"region_join_enabled", "region_leave_enabled",
		"region_join_template", "region_leave_template", "region_chat_channel",
	} {
		typ, err := columnType(db, "welcome_config", col)
		if err != nil {
			t.Fatalf("columnType(%s): %v", col, err)
		}
		if typ == "" {
			t.Errorf("column %s missing from welcome_config after initWelcomeConfigMissingColumns", col)
		}
	}
}

// TestWelcomeConfigMissingColumns_Idempotent verifies that running
// initWelcomeConfigMissingColumns on a DB that already has the columns is a no-op.
func TestWelcomeConfigMissingColumns_Idempotent(t *testing.T) {
	t.Parallel()
	// Use the current schema (all columns present).
	db := openMemUnifiedStore(t)

	if err := initWelcomeConfigMissingColumns(db); err != nil {
		t.Fatalf("initWelcomeConfigMissingColumns on current schema: %v", err)
	}
}

// TestRepairWelcomeConfigIntegerColumns_OldSchema verifies that the full
// applyUnifiedSchema succeeds on a DB where welcome_config lacks
// region_join_enabled — the ADD COLUMN migration must run first.
func TestRepairWelcomeConfigIntegerColumns_OldSchema(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/old.db"
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	// Create welcome_config without region columns to simulate a very old install.
	if _, err := raw.Exec(`
		CREATE TABLE welcome_config (
			server_id                     INTEGER NOT NULL DEFAULT 1,
			enabled                       INTEGER NOT NULL DEFAULT 0,
			scan_secs                     INTEGER NOT NULL DEFAULT 30,
			active_version                TEXT    NOT NULL DEFAULT '',
			packages_json                 TEXT    NOT NULL DEFAULT '[]',
			welcome_message_enabled       INTEGER NOT NULL DEFAULT 0,
			welcome_message               TEXT    NOT NULL DEFAULT '',
			welcome_whisper_source_player TEXT    NOT NULL DEFAULT '',
			motd_enabled                  INTEGER NOT NULL DEFAULT 0,
			motd_message                  TEXT    NOT NULL DEFAULT '',
			motd_source_player            TEXT    NOT NULL DEFAULT '',
			updated_at                    TEXT    NOT NULL DEFAULT '',
			PRIMARY KEY (server_id)
		)
	`); err != nil {
		_ = raw.Close()
		t.Fatalf("seed old welcome_config: %v", err)
	}
	_ = raw.Close()

	// openUnifiedStore calls applyUnifiedSchema — must not fail despite the old schema.
	db, err := openUnifiedStore(path)
	if err != nil {
		t.Fatalf("openUnifiedStore on old DB without region_join_enabled: %v", err)
	}
	defer func() { _ = db.Close() }()

	// The columns must exist after schema init.
	for _, col := range []string{"region_join_enabled", "region_leave_enabled"} {
		typ, err := columnType(db, "welcome_config", col)
		if err != nil {
			t.Fatalf("columnType(%s): %v", col, err)
		}
		if typ == "" {
			t.Errorf("column %s still missing after openUnifiedStore", col)
		}
	}
}

// ── Bug #3: rebuildLegacyServerIDToInt with missing columns ──────────────────

// TestRebuildLegacyServerIDToInt_SkipsMissingCols verifies that the rebuild
// silently skips columns that don't exist in the source table, preventing the
// "no such column: packages_json" error when the column was already dropped by
// a previous partial migration run.
func TestRebuildLegacyServerIDToInt_SkipsMissingCols(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()

	// Simulate a welcome_config with TEXT server_id and WITHOUT packages_json.
	if _, err := db.Exec(`
		CREATE TABLE servers (id INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '');
		INSERT INTO servers(id, name) VALUES (1, 'Default');
		CREATE TABLE welcome_config (
			server_id  TEXT    NOT NULL DEFAULT 'default',
			enabled    INTEGER NOT NULL DEFAULT 0,
			scan_secs  INTEGER NOT NULL DEFAULT 30,
			updated_at TEXT    NOT NULL DEFAULT ''
		);
		INSERT INTO welcome_config (server_id, enabled, scan_secs, updated_at)
		VALUES ('default', 1, 30, '2024-01-01T00:00:00Z');
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// cols includes packages_json which does NOT exist in the table.
	cols := []string{"enabled", "scan_secs", "updated_at", "packages_json"}
	err = rebuildLegacyServerIDToInt(db, "welcome_config", "welcome_config_int",
		`CREATE TABLE welcome_config_int (
			server_id  INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			enabled    INTEGER NOT NULL DEFAULT 0,
			scan_secs  INTEGER NOT NULL DEFAULT 30,
			packages_json TEXT NOT NULL DEFAULT '[]',
			updated_at TEXT    NOT NULL DEFAULT '',
			PRIMARY KEY (server_id)
		)`, cols, 1)
	if err != nil {
		t.Errorf("rebuildLegacyServerIDToInt with missing col: %v", err)
	}

	// After rebuild, welcome_config must have INTEGER server_id.
	typ, err := columnType(db, "welcome_config", "server_id")
	if err != nil {
		t.Fatalf("columnType after rebuild: %v", err)
	}
	if typ != "INTEGER" {
		t.Errorf("welcome_config.server_id type = %q, want INTEGER", typ)
	}

	// The row must exist with the correct server_id.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM welcome_config WHERE server_id = 1`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("welcome_config rows with server_id=1 = %d, want 1", count)
	}
}

// ── Bug #4: insertTiers idempotency ──────────────────────────────────────────

// TestInsertTiers_Idempotent verifies that calling insertTiers twice (with the
// same tier_key for the same server_id) does not return a UNIQUE constraint
// error — ON CONFLICT DO NOTHING makes it a no-op for duplicates.
func TestInsertTiers_Idempotent(t *testing.T) {
	t.Parallel()
	db := openMemUnifiedStore(t)
	seedDefaultServer(t, db)
	store := newBattlepassStore(db, defaultServerID)

	tiers := []battlepassTier{
		{TierKey: "level:5", Category: "level", Label: "Level 5",
			Signal: battlepassSignalLevel, Threshold: 5, Enabled: true},
		{TierKey: "level:10", Category: "level", Label: "Level 10",
			Signal: battlepassSignalLevel, Threshold: 10, Enabled: true},
	}

	if err := store.insertTiers(tiers); err != nil {
		t.Fatalf("first insertTiers: %v", err)
	}

	// Second insert of the same tiers must be a no-op, not an error.
	if err := store.insertTiers(tiers); err != nil {
		t.Errorf("second insertTiers: %v (want nil — ON CONFLICT DO NOTHING)", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM battlepass_tiers WHERE server_id = ?`, defaultServerID).Scan(&count); err != nil {
		t.Fatalf("count tiers: %v", err)
	}
	if count != 2 {
		t.Errorf("battlepass_tiers count = %d, want 2", count)
	}
}

// TestSeedTiersIfEmpty_PartialPriorSeed verifies that seedTiersIfEmpty succeeds
// even when some (but not all) tiers already exist from a failed previous run.
func TestSeedTiersIfEmpty_PartialPriorSeed(t *testing.T) {
	t.Parallel()
	db := openMemUnifiedStore(t)
	seedDefaultServer(t, db)
	store := newBattlepassStore(db, defaultServerID)

	// Pre-seed level:5 directly (simulating a partially-committed previous run).
	if _, err := db.Exec(`
		INSERT INTO battlepass_tiers
			(server_id, tier_key, category, label, signal, signal_key, threshold, intel, reward_items, enabled, created_at, updated_at)
		VALUES (?, 'level:5', 'level', 'Level 5', 'level', '', 5, 0, '', 1, '', '')`,
		defaultServerID); err != nil {
		t.Fatalf("pre-seed level:5: %v", err)
	}

	catalog := defaultBattlepassCatalog()
	// With partial prior seed: COUNT(*) WHERE server_id=1 = 1 > 0 → early exit.
	n, err := store.seedTiersIfEmpty(catalog)
	if err != nil {
		t.Errorf("seedTiersIfEmpty with partial prior seed: %v", err)
	}
	if n != 0 {
		t.Errorf("seedTiersIfEmpty = %d inserted (want 0, should have exited early)", n)
	}
}
