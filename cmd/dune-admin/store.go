package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // pure-Go sqlite driver (registers "sqlite")
)

// initServerDiscordStatusSchema creates the per-server Discord status-message
// pointer table. Keyed by servers.id with a real FK so deleting a server
// cascades it (replaces the discord_status_message:<scope> meta key + manual
// purge). Idempotent.
func initServerDiscordStatusSchema(db *sql.DB) error {
	// An earlier build may have created this table with a different primary key
	// (keyed by server_id only, before per-guild status). CREATE TABLE IF NOT
	// EXISTS won't migrate that, and the (server_id, guild_id) upsert then fails
	// with "ON CONFLICT ... does not match any PRIMARY KEY or UNIQUE constraint".
	// The table only holds a disposable posted-message pointer (re-posted on the
	// next tick if lost), so drop and recreate it when the key has drifted.
	pk, exists, err := statusTablePKColumns(db)
	if err != nil {
		return err
	}
	pkMatches := len(pk) == 2 && pk[0] == "server_id" && pk[1] == "guild_id"
	if exists && !pkMatches {
		if _, err := db.Exec(`DROP TABLE server_discord_status`); err != nil {
			return fmt.Errorf("drop drifted server_discord_status: %w", err)
		}
	}

	// Keyed by (server_id, guild_id): a server may have several guilds mapped to
	// it, each posting its own status embed to its own channel, so the posted
	// message pointer must be per (server, guild). server_id keeps the FK so
	// deleting a server cascades all its rows.
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS server_discord_status (
			server_id  INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			guild_id   TEXT NOT NULL DEFAULT '',
			channel_id TEXT NOT NULL DEFAULT '',
			message_id TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (server_id, guild_id)
		)`)
	return err
}

// statusTablePKColumns returns the primary-key columns of server_discord_status
// in key order, plus whether the table exists. Used to detect a drifted schema
// from an earlier build that would break the (server_id, guild_id) upsert.
func statusTablePKColumns(db *sql.DB) (cols []string, exists bool, err error) {
	rows, err := db.Query(`PRAGMA table_info(server_discord_status)`)
	if err != nil {
		return nil, false, fmt.Errorf("table_info server_discord_status: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byPos := map[int]string{}
	maxPos := 0
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return nil, false, err
		}
		exists = true
		if pk > 0 {
			byPos[pk] = name
			if pk > maxPos {
				maxPos = pk
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, exists, err
	}
	for i := 1; i <= maxPos; i++ {
		cols = append(cols, byPos[i])
	}
	return cols, exists, nil
}

// withForeignKeysDisabled pins a single connection, turns OFF foreign-key
// enforcement on it, and runs fn inside a single transaction so the whole schema
// rebuild is ATOMIC: if the process dies mid-rebuild (e.g. an air hot-reload
// restart), the transaction rolls back and the original tables are left intact —
// never a half-done DROP/RENAME that corrupts sqlite_master. SQLite's
// foreign_keys pragma is per-connection and ignored inside a transaction, so it
// is set on the connection BEFORE the transaction begins. Migration-only — not
// safe under concurrent use.
func withForeignKeysDisabled(ctx context.Context, db *sql.DB, fn func(*sql.Conn) error) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return fmt.Errorf("disable foreign_keys: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(ctx, `PRAGMA foreign_keys=ON`) }()

	if _, err := conn.ExecContext(ctx, `BEGIN`); err != nil {
		return fmt.Errorf("begin rebuild tx: %w", err)
	}
	if err := fn(conn); err != nil {
		_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		return err
	}
	// Validate FK integrity BEFORE committing; a violation rolls the whole
	// rebuild back rather than persisting a broken reference.
	if err := foreignKeyCheckFailed(ctx, conn); err != nil {
		_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		return err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		return fmt.Errorf("commit rebuild tx: %w", err)
	}
	return nil
}

// foreignKeyCheckFailed returns a non-nil error when PRAGMA foreign_key_check
// reports any dangling reference. The result set is fully drained/closed before
// returning so the caller can immediately COMMIT on the same connection.
func foreignKeyCheckFailed(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		return fmt.Errorf("foreign_key_check reported violations after rebuild")
	}
	return rows.Err()
}

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
	dbPath := resolveStoreDBPath()
	// Snapshot the pristine pre-upgrade DB + config.yaml BEFORE applyUnifiedSchema
	// adds server_id / the config import remaps data — a one-way migration, so
	// this is the operator's recovery / downgrade path.
	backupPreMigration(dbPath)
	db, err := openUnifiedStore(dbPath)
	if err != nil {
		componentLog("store").Error().Err(err).Msg("unified store open failed — falling back to legacy stores")
		return func() {}
	}
	globalStore = db
	authUsersDB = newAuthUserStore(db)
	globalServersStore = newServersStore(db)
	globalSettingsStore = newSettingsStore(db)
	globalDiscordGuildsStore = newDiscordGuildsStore(db)
	if err := migrateLegacyStores(db, defaultLegacySources()); err != nil {
		componentLog("store").Warn().Err(err).Msg("legacy store migration warning")
	}
	// NOTE: the text→int server_id conversion + blob→surrogate child migration
	// runs in hydrateConfigFromStore AFTER the config.yaml server import, so the
	// default server row exists with its numeric id before scoped rows are stamped.
	return func() { _ = globalStore.Close() }
}

// backupPreMigration makes a one-time snapshot of the pre-upgrade SQLite store
// and config.yaml the first time a migrating version boots, so an operator can
// recover or downgrade. Each ".pre-migrate.bak" is its own run-once sentinel:
// once it exists the source is left untouched, so the pristine pre-migration
// state is captured exactly once and never overwritten by a later boot.
func backupPreMigration(dbPath string) {
	backupFileOnce(dbPath)
	backupFileOnce(configPath())
}

// backupFileOnce copies src → src+".pre-migrate.bak" unless that backup already
// exists or src is missing / in-memory. Best-effort: failures warn, never fatal.
func backupFileOnce(src string) {
	if src == "" || src == ":memory:" {
		return
	}
	dst := src + ".pre-migrate.bak"
	// #nosec G304 G703 -- dst derives from configPath()/DUNE_ADMIN_DB (operator/HOME path), not request input.
	if _, err := os.Stat(dst); err == nil {
		return // already snapshotted — never overwrite the pristine backup
	}
	// #nosec G304 G703 -- src is configPath()/DUNE_ADMIN_DB (operator/HOME path), not request input.
	data, err := os.ReadFile(src)
	if err != nil {
		return // nothing to back up (fresh install) or unreadable
	}
	// #nosec G304 G703 -- dst is derived from the same operator/HOME path, not request input.
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		componentLog("store").Warn().Err(err).Str("src", src).Msg("pre-migration backup failed")
		return
	}
	componentLog("store").Info().Str("path", dst).Msg("pre-migration backup written")
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
// handlers) can share a single file without contention. foreign_keys(1) is set
// on every pooled connection so ON DELETE CASCADE is enforced (SQLite defaults
// it OFF, per-connection).
func openUnifiedStore(path string) (*sql.DB, error) {
	memory := path == ":memory:"
	// In-memory: a file: URI + single connection keeps one isolated DB alive for
	// the pool's lifetime (plain ":memory:" would give each pooled connection its
	// own empty DB). The pragma applies to that connection.
	dsn := "file::memory:?_pragma=foreign_keys(1)"
	if !memory {
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open unified store: %w", err)
	}
	if memory {
		db.SetMaxOpenConns(1)
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
	// Order matters: servers must exist before tables that FK-reference it
	// (e.g. server_discord_status). Each entry is idempotent.
	schemas := []struct {
		name string
		init func(*sql.DB) error
	}{
		// servers is the FK parent for every scoped table; create it first.
		{"servers", initServersSchema},
		{"servers columns", initServersColumnsSchema},
		{"session", initSessionSchema},
		{"welcome", initWelcomeSchema},
		{"welcome columns", initWelcomeColumnsSchema},
		{"welcome integer repair", repairWelcomeConfigIntegerColumns},
		{"location", initLocationSchema},
		{"give-packs", initGivePacksSchema},
		{"give-packs columns", initGivePacksColumnsSchema},
		{"events", initEventsSchema},
		{"battlepass", initBattlepassSchema},
		{"auth users", initAuthUsersSchema},
		{"settings", initSettingsSchema},
		{"app_config columns", initAppConfigColumnsSchema},
		{"app permissions", initAppPermissionsSchema},
		{"server_discord_status", initServerDiscordStatusSchema},
		{"discord_guilds", initDiscordGuildsSchema},
		{"server_backup_schedule", initServerBackupScheduleSchema},
		{"server_restart_schedule", initServerRestartScheduleSchema},
	}
	for _, s := range schemas {
		if err := s.init(db); err != nil {
			return fmt.Errorf("unified store: %s schema: %w", s.name, err)
		}
	}
	if err := initLandsraadBotSchema(db); err != nil {
		return fmt.Errorf("unified store: landsraad bot schema: %w", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`); err != nil {
		return fmt.Errorf("unified store: meta schema: %w", err)
	}
	// server_id indexes on tables that exist in pre-scoping (0.39.5) stores are
	// created here only when the column is already present (fresh installs). On a
	// legacy store the column is absent until migrateUnifiedRemodel rebuilds the
	// table, which then re-runs this to create the index. Inlining the index in
	// each table's CREATE would crash on a legacy table (no such column).
	return ensureServerIDIndexes(db)
}

// legacyScopedIndexTargets are the (index, table) pairs for server_id indexes on
// tables that predate per-server scoping. The index is created only once the
// table actually has a server_id column.
var legacyScopedIndexTargets = []struct{ index, table string }{
	{"idx_ps_server", "play_sessions"},
	{"idx_ss_server", "stat_snapshots"},
	{"idx_welcome_grants_server", "welcome_grants"},
	{"idx_event_definitions_server", "event_definitions"},
	{"idx_event_award_claims_server", "event_award_claims"},
	{"idx_battlepass_tiers_server", "battlepass_tiers"},
	{"idx_battlepass_claims_server", "battlepass_claims"},
	{"idx_battlepass_accounts_server", "battlepass_accounts"},
	{"idx_battlepass_grant_ledger_server", "battlepass_grant_ledger"},
}

// ensureServerIDIndexes creates a server_id index on each legacy-scoped table
// that has gained the column. Idempotent; skips tables still in their legacy
// pre-scoping shape (column absent) so it is safe to call before the remodel.
func ensureServerIDIndexes(db *sql.DB) error {
	for _, t := range legacyScopedIndexTargets {
		typ, err := columnType(db, t.table, "server_id")
		if err != nil {
			return err
		}
		if typ == "" {
			continue // legacy shape — index created after migrateUnifiedRemodel
		}
		// #nosec G201 -- index/table are internal literals from legacyScopedIndexTargets
		if _, err := db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s(server_id)`, t.index, t.table)); err != nil {
			return fmt.Errorf("create %s: %w", t.index, err)
		}
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
			name: "give-packs",
			path: filepath.Join(dir, "give-packs.db"),
			// Only the config row (with its legacy packs_json) is imported from a
			// pre-unified standalone file. The typed give_packs/give_pack_items
			// tables are derived afterwards by migrateGivePacksColumns — a legacy
			// file predates them, so copying them directly would fail every boot.
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
	// Copy only the columns common to BOTH the legacy source and the current
	// target. A plain SELECT * breaks when the legacy file's table has drifted
	// from the current schema (e.g. a standalone sessions.db that predates or
	// postdates a column the unified table lacks → "N columns but M values").
	srcCols, err := pragmaColumns(db, alias, table)
	if err != nil {
		return err
	}
	dstCols, err := pragmaColumns(db, "main", table)
	if err != nil {
		return err
	}
	shared := intersectCols(dstCols, srcCols)
	if len(shared) == 0 {
		return nil // no overlap — nothing meaningful to import
	}
	colList := strings.Join(shared, ", ")
	// #nosec G202 -- alias/table are trusted legacySource constants; colList is
	// built from PRAGMA-reported column names of those tables, never user input.
	if _, err := db.Exec(`INSERT OR IGNORE INTO ` + table + ` (` + colList + `) SELECT ` + colList + ` FROM ` + alias + `.` + table); err != nil {
		return fmt.Errorf("insert or ignore into %q: %w", table, err)
	}
	return nil
}

// pragmaColumns returns the column names of schema.table via PRAGMA table_info.
func pragmaColumns(db *sql.DB, schema, table string) ([]string, error) {
	// #nosec G201 -- schema/table are trusted internal constants, not user input.
	rows, err := db.Query(fmt.Sprintf(`PRAGMA %s.table_info(%s)`, schema, table))
	if err != nil {
		return nil, fmt.Errorf("table_info %s.%s: %w", schema, table, err)
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

// intersectCols returns the members of dst that also appear in src, preserving
// dst's order.
func intersectCols(dst, src []string) []string {
	have := make(map[string]bool, len(src))
	for _, c := range src {
		have[c] = true
	}
	out := make([]string, 0, len(dst))
	for _, c := range dst {
		if have[c] {
			out = append(out, c)
		}
	}
	return out
}
