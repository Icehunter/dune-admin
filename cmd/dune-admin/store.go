package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go sqlite driver (registers "sqlite")
)

// globalStore is the single shared SQLite handle used by all four local stores
// (sessions, welcome, locations, give-packs). It is opened once at startup in
// initUnifiedStore and closed when the server shuts down.
var globalStore *sql.DB

// globalServersStore and globalSettingsStore are the DB-backed sources of truth
// for the server list and global settings. Nil when the unified store failed to
// open (legacy config.yaml fallback path).
var globalServersStore *serversStore
var globalSettingsStore *settingsStore

// initUnifiedStoreOnce opens the unified SQLite store, runs legacy migrations,
// sets globalStore, and returns a close func for use with defer. Non-fatal:
// on error a warning is printed and globalStore stays nil so individual stores
// fall back to their own files.
func initUnifiedStoreOnce() func() {
	db, err := openUnifiedStore(resolveStoreDBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "unified store: open failed: %v — falling back to legacy stores\n", err)
		return func() {}
	}
	globalStore = db
	authUsersDB = newAuthUserStore(db)
	globalServersStore = newServersStore(db)
	globalSettingsStore = newSettingsStore(db)
	if err := migrateLegacyStores(db, defaultLegacySources()); err != nil {
		fmt.Fprintf(os.Stderr, "unified store: migration warning: %v\n", err)
	}
	return func() { _ = globalStore.Close() }
}

// resolveStoreDBPath returns the path for the unified SQLite database.
// The env var DUNE_ADMIN_DB overrides the default so operators and K8s can
// redirect it to a persistent volume.
func resolveStoreDBPath() string {
	if p := os.Getenv("DUNE_ADMIN_DB"); p != "" {
		return p
	}
	return filepath.Join(configDir(), "dune-admin.db")
}

// openUnifiedStore opens (or creates) the unified SQLite database at path,
// applies all store schemas, and returns the shared handle. path may be
// ":memory:" for tests. The WAL journal mode and a 5-second busy-timeout are
// applied so concurrent writers (session poller, welcome scanner, CRUD
// handlers) can share a single file without contention.
func openUnifiedStore(path string) (*sql.DB, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open unified store: %w", err)
	}
	if err := applyUnifiedSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// applyUnifiedSchema creates all store tables and the meta table in db.
// Safe to call multiple times (all statements use IF NOT EXISTS / ALTER TABLE
// with duplicate-column guards).
func applyUnifiedSchema(db *sql.DB) error {
	if err := initSessionSchema(db); err != nil {
		return fmt.Errorf("unified store: session schema: %w", err)
	}
	if err := initWelcomeSchema(db); err != nil {
		return fmt.Errorf("unified store: welcome schema: %w", err)
	}
	if err := initLocationSchema(db); err != nil {
		return fmt.Errorf("unified store: location schema: %w", err)
	}
	if err := initGivePacksSchema(db); err != nil {
		return fmt.Errorf("unified store: give-packs schema: %w", err)
	}
	if err := initEventsSchema(db); err != nil {
		return fmt.Errorf("unified store: events schema: %w", err)
	}
	if err := initBattlepassSchema(db); err != nil {
		return fmt.Errorf("unified store: battlepass schema: %w", err)
	}
	if err := initAuthUsersSchema(db); err != nil {
		return fmt.Errorf("unified store: auth users schema: %w", err)
	}
	if err := initServersSchema(db); err != nil {
		return fmt.Errorf("unified store: servers schema: %w", err)
	}
	if err := initSettingsSchema(db); err != nil {
		return fmt.Errorf("unified store: settings schema: %w", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`); err != nil {
		return fmt.Errorf("unified store: meta schema: %w", err)
	}
	return nil
}

// legacySource describes one legacy SQLite file that should be imported into
// the unified store.
type legacySource struct {
	name   string   // short label used as the migration marker key
	path   string   // filesystem path of the legacy db file
	tables []string // tables to copy (INSERT OR IGNORE … SELECT *)
}

// defaultLegacySources returns the four legacy sources resolved from the
// current config directory (and respecting DUNE_ADMIN_SESSIONS_DB).
func defaultLegacySources() []legacySource {
	dir := configDir()
	return []legacySource{
		{
			name:   "sessions",
			path:   resolveSessionDBPath(), // honors DUNE_ADMIN_SESSIONS_DB
			tables: []string{"play_sessions", "stat_snapshots"},
		},
		{
			name:   "welcome",
			path:   filepath.Join(dir, "welcome-package.db"),
			tables: []string{"welcome_grants", "welcome_config"},
		},
		{
			name:   "locations",
			path:   filepath.Join(dir, "locations.db"),
			tables: []string{"map_locations"},
		},
		{
			name:   "give-packs",
			path:   filepath.Join(dir, "give-packs.db"),
			tables: []string{"give_packs_config"},
		},
	}
}

// migrateLegacyStores imports data from legacy store files into the unified db.
// For each source:
//   - If the file does not exist it is silently skipped (fresh install).
//   - If the migration marker "migrated:<name>" is already present in meta the
//     source is skipped (idempotent — never double-imports).
//   - Otherwise the source is ATTACHed, all listed tables are copied with
//     INSERT OR IGNORE (so rows already in the unified DB from a partial import
//     are not duplicated), then the marker is written and the source DETACHed.
//
// Legacy files are left on disk untouched so a rollback can revert to them.
func migrateLegacyStores(db *sql.DB, sources []legacySource) error {
	for _, src := range sources {
		if err := migrateSingleStore(db, src); err != nil {
			return err
		}
	}
	return nil
}

// migrateSingleStore migrates one legacy source into db.
func migrateSingleStore(db *sql.DB, src legacySource) error {
	// Skip if already migrated.
	markerKey := "migrated:" + src.name
	var existing string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, markerKey).Scan(&existing)
	if err == nil {
		return nil // marker present — already imported
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check migration marker %q: %w", src.name, err)
	}

	// Skip missing files silently (fresh install or feature never used).
	if _, statErr := os.Stat(src.path); os.IsNotExist(statErr) {
		return nil
	}

	// ATTACH the legacy file as a read-only alias. The alias name must be a
	// valid SQLite identifier. We use a fixed literal per source so there is no
	// dynamic SQL construction that would trip gosec.
	const alias = "legacy_src"
	// Use file:<path>?mode=ro so we never write to the legacy file.
	attachDSN := "file:" + src.path + "?mode=ro"
	if _, err := db.Exec(`ATTACH DATABASE ? AS `+alias, attachDSN); err != nil { // #nosec G202 -- alias is a hardcoded constant, not user input
		return fmt.Errorf("attach legacy store %q: %w", src.name, err)
	}
	defer func() {
		_, _ = db.Exec(`DETACH DATABASE ` + alias) // #nosec G202 -- constant alias
	}()

	for _, tbl := range src.tables {
		if err := copyTable(db, alias, tbl); err != nil {
			return fmt.Errorf("copy table %q from %q: %w", tbl, src.name, err)
		}
	}

	if _, err := db.Exec(
		`INSERT INTO meta(key, value) VALUES(?, 'done')
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		markerKey); err != nil {
		return fmt.Errorf("write migration marker %q: %w", src.name, err)
	}
	return nil
}

// copyTable copies all rows from alias.table into table using INSERT OR IGNORE
// so existing rows (e.g. from a partial prior import) are silently skipped.
// Table name comes from a trusted hard-coded list (see legacySource.tables in
// defaultLegacySources) — it is never derived from user input.
func copyTable(db *sql.DB, alias, table string) error {
	// #nosec G202 -- table and alias are both trusted constants from legacySource
	_, err := db.Exec(`INSERT OR IGNORE INTO ` + table + ` SELECT * FROM ` + alias + `.` + table)
	if err != nil {
		return fmt.Errorf("insert or ignore into %q: %w", table, err)
	}
	return nil
}
