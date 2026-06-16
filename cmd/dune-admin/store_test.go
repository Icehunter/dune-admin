package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// openMemUnifiedStore opens an in-memory unified store for testing.
func openMemUnifiedStore(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openUnifiedStore(":memory:")
	if err != nil {
		t.Fatalf("openUnifiedStore: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedDefaultServer inserts the default server row (id 1) so scoped rows imported
// from legacy files satisfy their server_id FK in the FK-enforced unified store.
func seedDefaultServer(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO servers (id, name, position) VALUES (1, 'Default', 0)`); err != nil {
		t.Fatalf("seed default server: %v", err)
	}
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var found string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, name).Scan(&found)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("query table %q: %v", name, err)
	}
	return found == name
}

func TestOpenUnifiedStore_CreatesAllTables(t *testing.T) {
	t.Parallel()
	db := openMemUnifiedStore(t)

	for _, tbl := range []string{
		"play_sessions", "stat_snapshots", "welcome_grants", "welcome_config",
		"map_locations", "give_packs_config", "meta",
	} {
		if !tableExists(t, db, tbl) {
			t.Errorf("expected table %q to exist in unified store", tbl)
		}
	}
}

// seedLegacyStores creates the four legacy SQLite files in dir with sample rows
// and returns the source descriptors pointing at them.
func seedLegacyStores(t *testing.T, dir string) []legacySource {
	t.Helper()

	sessionsPath := filepath.Join(dir, "sessions.db")
	welcomePath := filepath.Join(dir, "welcome-package.db")
	locationsPath := filepath.Join(dir, "locations.db")
	givePacksPath := filepath.Join(dir, "give-packs.db")

	// Sessions: one completed play session. server_id is stamped to the default
	// server id (1) just as a real per-server legacy file would carry it.
	sdb, err := openSessionDB(sessionsPath)
	if err != nil {
		t.Fatalf("openSessionDB: %v", err)
	}
	if _, err := sdb.Exec(
		`INSERT INTO play_sessions(server_id, account_id, started_at, ended_at, duration_secs)
		 VALUES (1, 29, '2026-01-01T00:00:00Z', '2026-01-01T01:00:00Z', 3600)`); err != nil {
		t.Fatalf("seed play_sessions: %v", err)
	}
	_ = sdb.Close()

	// Welcome: one config row + one grant.
	ws, err := openWelcomeStore(welcomePath)
	if err != nil {
		t.Fatalf("openWelcomeStore: %v", err)
	}
	if err := ws.saveConfig(welcomeConfigRow{Enabled: true, ScanSecs: 30, PackagesJSON: "[]"}); err != nil {
		t.Fatalf("seed welcome config: %v", err)
	}
	if err := ws.insertGranted("fls-1", "v1", 29, "Narisa"); err != nil {
		t.Fatalf("seed welcome grant: %v", err)
	}
	_ = ws.close()

	// Locations: a custom location (on top of the cheatLocations seed).
	ls, err := openLocationStore(locationsPath)
	if err != nil {
		t.Fatalf("openLocationStore: %v", err)
	}
	if err := ls.upsert("Test Pad", 1, 2, 3); err != nil {
		t.Fatalf("seed location: %v", err)
	}
	_ = ls.close()

	// Give-packs: a config row stamped to the default server id. The packs
	// themselves live in the typed give_packs/give_pack_items tables now; the
	// config row only carries base_packs_loaded + updated_at presence.
	gps, err := openGivePacksStore(givePacksPath)
	if err != nil {
		t.Fatalf("openGivePacksStore: %v", err)
	}
	if _, err := gps.db.Exec(
		`INSERT INTO give_packs_config (server_id, base_packs_loaded, updated_at)
		 VALUES (1, 1, '')`); err != nil {
		t.Fatalf("seed give-packs: %v", err)
	}
	_ = gps.close()

	return []legacySource{
		{name: "sessions", path: sessionsPath, tables: []string{"play_sessions", "stat_snapshots"}},
		{name: "welcome", path: welcomePath, tables: []string{"welcome_grants", "welcome_config"}},
		{name: "locations", path: locationsPath, tables: []string{"map_locations"}},
		{name: "give-packs", path: givePacksPath, tables: []string{"give_packs_config"}},
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	// #nosec G201 -- table is a hardcoded test constant, never user input
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestMigrateLegacyStores_ImportsData(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sources := seedLegacyStores(t, dir)

	db := openMemUnifiedStore(t)
	seedDefaultServer(t, db)
	if err := migrateLegacyStores(db, sources); err != nil {
		t.Fatalf("migrateLegacyStores: %v", err)
	}

	if n := countRows(t, db, "play_sessions"); n != 1 {
		t.Errorf("play_sessions: want 1, got %d", n)
	}
	if n := countRows(t, db, "welcome_grants"); n != 1 {
		t.Errorf("welcome_grants: want 1, got %d", n)
	}
	if n := countRows(t, db, "welcome_config"); n != 1 {
		t.Errorf("welcome_config: want 1, got %d", n)
	}
	// give-packs config row imported and stamped to the default server id. Packs
	// themselves live in the typed give_packs tables (no packs_json blob anymore).
	var basePacksLoaded int
	if err := db.QueryRow(`SELECT base_packs_loaded FROM give_packs_config WHERE server_id = 1`).Scan(&basePacksLoaded); err != nil {
		t.Fatalf("read imported give_packs_config: %v", err)
	}
	if basePacksLoaded != 1 {
		t.Errorf("expected imported give_packs_config base_packs_loaded=1, got %d", basePacksLoaded)
	}
	// the custom location should be present.
	var locCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM map_locations WHERE name = 'Test Pad'`).Scan(&locCount); err != nil {
		t.Fatalf("query imported location: %v", err)
	}
	if locCount != 1 {
		t.Errorf("expected imported 'Test Pad' location, got %d", locCount)
	}

	// markers recorded for every source.
	for _, src := range sources {
		var marker string
		err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, "migrated:"+src.name).Scan(&marker)
		if err != nil {
			t.Errorf("expected migration marker for %q, got err: %v", src.name, err)
		}
	}
}

func TestMigrateLegacyStores_Idempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sources := seedLegacyStores(t, dir)

	db := openMemUnifiedStore(t)
	seedDefaultServer(t, db)
	if err := migrateLegacyStores(db, sources); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	firstSessions := countRows(t, db, "play_sessions")
	firstLocations := countRows(t, db, "map_locations")

	if err := migrateLegacyStores(db, sources); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if got := countRows(t, db, "play_sessions"); got != firstSessions {
		t.Errorf("play_sessions duplicated: want %d, got %d", firstSessions, got)
	}
	if got := countRows(t, db, "map_locations"); got != firstLocations {
		t.Errorf("map_locations duplicated: want %d, got %d", firstLocations, got)
	}
}

func TestMigrateLegacyStores_MissingFilesSkip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sources := []legacySource{
		{name: "sessions", path: filepath.Join(dir, "nope.db"), tables: []string{"play_sessions"}},
	}

	db := openMemUnifiedStore(t)
	if err := migrateLegacyStores(db, sources); err != nil {
		t.Fatalf("expected nil error for missing legacy files, got %v", err)
	}
	// No marker should be written for a file that does not exist.
	var marker string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, "migrated:sessions").Scan(&marker)
	if err != sql.ErrNoRows {
		t.Errorf("expected no marker for missing file, got marker=%q err=%v", marker, err)
	}
}
