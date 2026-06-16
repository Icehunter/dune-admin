package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestWelcomeStoreLifecycle(t *testing.T) {
	s, err := openWelcomeStore(filepath.Join(t.TempDir(), "welcome.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = s.close() }()

	// Nothing granted initially.
	if ex, err := s.grantExists("P1", "v1", 10); err != nil || ex {
		t.Fatalf("expected no grant initially (ex=%v err=%v)", ex, err)
	}

	// A granted row registers as existing — the once-each gate.
	if err := s.insertGranted("P0", "v1", 9, "Duncan"); err != nil {
		t.Fatalf("insertGranted: %v", err)
	}
	if ex, _ := s.grantExists("P0", "v1", 9); !ex {
		t.Fatal("expected granted row to exist")
	}

	// A failed row ALSO gates retries (so we don't spam a broken account).
	if err := s.insertFailed("P1", "v1", 10, "Chani", "db timeout"); err != nil {
		t.Fatalf("insertFailed: %v", err)
	}
	if ex, _ := s.grantExists("P1", "v1", 10); !ex {
		t.Fatal("expected failed row to exist (gates retries)")
	}

	rows, err := s.listGrants(10)
	if err != nil {
		t.Fatalf("listGrants: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	var failed *welcomeGrantRecord
	for i := range rows {
		if rows[i].FlsID == "P1" {
			failed = &rows[i]
		}
	}
	if failed == nil || failed.Status != "failed" || failed.LastError != "db timeout" {
		t.Fatalf("failed row not recorded correctly: %+v", failed)
	}

	// Retry clears ONLY the failed row so the next scan re-attempts it.
	if n, err := s.deleteFailed("P1", "v1", 10); err != nil || n != 1 {
		t.Fatalf("deleteFailed on failed row: n=%d err=%v (want 1)", n, err)
	}
	if ex, _ := s.grantExists("P1", "v1", 10); ex {
		t.Fatal("failed row should be cleared after retry")
	}

	// A granted row is NEVER removed by retry — items can't duplicate.
	if n, _ := s.deleteFailed("P0", "v1", 9); n != 0 {
		t.Fatalf("granted row must not be deletable via retry, got %d", n)
	}
	if ex, _ := s.grantExists("P0", "v1", 9); !ex {
		t.Fatal("granted row must remain")
	}

	// Version re-issue: bumping the package version makes the same player
	// eligible again (the ledger key includes the version).
	if ex, _ := s.grantExists("P0", "v2", 9); ex {
		t.Fatal("a new package version should not be granted yet")
	}
}

// TestLoadConfigToleratesEmptyStringIntegers verifies that a DB where a legacy
// TEXT→INTEGER migration left ” in INTEGER columns doesn't cause a scan crash.
// SQLite's loose type affinity allows storing ” in an INTEGER column, and the
// rebuildLegacyServerIDToInt migration can propagate this from old schemas.
func TestLoadConfigToleratesEmptyStringIntegers(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "w.sqlite"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := initWelcomeSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	// Bypass Go-layer type safety to insert empty strings directly into INTEGER
	// columns, replicating what rebuildLegacyServerIDToInt can leave behind.
	_, err = db.Exec(`
		INSERT INTO welcome_config
			(server_id, enabled, scan_secs, active_version,
			 welcome_message_enabled, welcome_message, welcome_whisper_source_player,
			 motd_enabled, motd_message, motd_source_player,
			 region_join_enabled, region_leave_enabled, region_join_template, region_leave_template,
			 region_chat_channel, updated_at)
		VALUES (1, '', 30, '', '', '', '', '', '', '', '', '', '', '', 'whisper', '')`)
	if err != nil {
		t.Fatalf("insert legacy row with empty-string integers: %v", err)
	}
	s := &welcomeStore{db: db, serverID: 1}
	cfg, found, err := s.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !found {
		t.Fatal("expected config row to be found")
	}
	if cfg.Enabled {
		t.Error("Enabled: want false for empty string, got true")
	}
	if cfg.WelcomeMessageEnabled {
		t.Error("WelcomeMessageEnabled: want false for empty string, got true")
	}
	if cfg.MotdEnabled {
		t.Error("MotdEnabled: want false for empty string, got true")
	}
	if cfg.RegionJoinEnabled {
		t.Error("RegionJoinEnabled: want false for empty string, got true")
	}
	if cfg.RegionLeaveEnabled {
		t.Error("RegionLeaveEnabled: want false for empty string, got true")
	}
}

// TestRepairWelcomeConfigIntegerColumns verifies the data-repair migration
// converts ” INTEGER columns to their zero values in existing rows.
func TestRepairWelcomeConfigIntegerColumns(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "repair.sqlite"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := initWelcomeSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	_, err = db.Exec(`
		INSERT INTO welcome_config
			(server_id, enabled, scan_secs, active_version,
			 welcome_message_enabled, welcome_message, welcome_whisper_source_player,
			 motd_enabled, motd_message, motd_source_player,
			 region_join_enabled, region_leave_enabled, region_join_template, region_leave_template,
			 region_chat_channel, updated_at)
		VALUES (1, '', 30, '', '', '', '', '', '', '', '', '', '', '', 'whisper', '')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := repairWelcomeConfigIntegerColumns(db); err != nil {
		t.Fatalf("repair: %v", err)
	}
	// After repair the column must hold a proper integer (0), not a string.
	var v int
	if err := db.QueryRow(`SELECT welcome_message_enabled FROM welcome_config WHERE server_id = 1`).Scan(&v); err != nil {
		t.Fatalf("scan after repair: %v", err)
	}
	if v != 0 {
		t.Errorf("welcome_message_enabled after repair: want 0, got %d", v)
	}
}
