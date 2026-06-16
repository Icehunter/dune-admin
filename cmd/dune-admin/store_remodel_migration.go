package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// store_remodel_migration.go performs the one-way 0.39.5 → unified-schema
// migration that must run AFTER the config.yaml server import (so the default
// server row exists with its numeric id). On a fresh install every scoped table
// is created with the final int-FK schema, so these helpers detect that and
// no-op. On a 0.39.5 store the scoped tables are TEXT-server_id (or unscoped)
// blobs; they are rebuilt to int-FK and their JSON blobs decomposed into the
// surrogate-id child tables. All steps are marker-gated for idempotency.

// migrateUnifiedRemodel runs the text→int conversion and blob→surrogate-id
// child-table decomposition against the default server id. Non-fatal: a failure
// warns and leaves markers unset so the next boot retries. db must be the
// unified store and the servers table must already hold the default server.
func migrateUnifiedRemodel(db *sql.DB, defaultID int) {
	if err := convertScopedTablesToInt(db, defaultID); err != nil {
		componentLog("store").Error().Err(err).Msg("remodel: scoped server_id int conversion")
	}
	if err := migrateLegacyWelcomeBlobs(db, defaultID); err != nil {
		componentLog("store").Error().Err(err).Msg("remodel: welcome blob → surrogate tables")
	}
	if err := migrateLegacyGivePacksBlobs(db, defaultID); err != nil {
		componentLog("store").Error().Err(err).Msg("remodel: give-packs blob → surrogate tables")
	}
	// File→DB migration of the three legacy file stores. Permissions are
	// app-level (no server_id); the two schedules are stamped to defaultID.
	if err := migrateLegacyPermissions(db); err != nil {
		componentLog("store").Error().Err(err).Msg("remodel: permissions.yaml → DB")
	}
	if err := migrateLegacyBackupSchedule(db, defaultID); err != nil {
		componentLog("store").Error().Err(err).Msg("remodel: scheduled-backups.json → DB")
	}
	if err := migrateLegacyRestartSchedule(db, defaultID); err != nil {
		componentLog("store").Error().Err(err).Msg("remodel: scheduled-restarts.json → DB")
	}
	if err := seedDiscordGuilds(db, defaultID); err != nil {
		componentLog("store").Error().Err(err).Msg("remodel: discord_guilds seed")
	}
	// Legacy tables now have an int server_id column; create their server_id
	// indexes (skipped at schema time when the column was still absent).
	if err := ensureServerIDIndexes(db); err != nil {
		componentLog("store").Error().Err(err).Msg("remodel: ensure server_id indexes")
	}
	// Now that the welcome / give-packs blobs have been decomposed into their
	// typed tables, drop those vestigial JSON columns. Must run LAST: the
	// migrateLegacy*Blobs steps above read them. (event config/reward stay JSON.)
	if err := dropVestigialBlobColumns(db); err != nil {
		componentLog("store").Error().Err(err).Msg("remodel: drop vestigial blob columns")
	}
}

// vestigialBlobColumn names a JSON-blob column that the storage remodel left
// behind once its data was decomposed into typed tables.
type vestigialBlobColumn struct {
	table  string
	column string
}

// vestigialBlobColumns is the full set dropped by dropVestigialBlobColumns.
// event_definitions.config_json / reward_json are intentionally NOT here: they
// are opaque frontend-owned documents (richer than any backend struct, e.g.
// reward.faction_scrip) and stay as JSON columns.
var vestigialBlobColumns = []vestigialBlobColumn{
	{"give_packs_config", "packs_json"},
	{"welcome_config", "packages_json"},
	{"welcome_config", "active_versions_json"},
}

// dropVestigialBlobColumns drops the five JSON-blob columns the remodel
// superseded with typed child tables. Marker-gated so it runs once; the drops
// happen inside that marker's transaction. Each column is checked for presence
// first so the step is safe on fresh installs (columns already absent) and on
// re-runs. None of these columns is a PK or indexed, so DROP COLUMN is safe.
func dropVestigialBlobColumns(db *sql.DB) error {
	return runColumnMigrationOnce(db, "migrated:drop_blob_columns", func(tx *sql.Tx) error {
		for _, c := range vestigialBlobColumns {
			typ, err := columnTypeTx(tx, c.table, c.column)
			if err != nil {
				return err
			}
			if typ == "" {
				continue // already absent
			}
			if _, err := tx.Exec(fmt.Sprintf( // #nosec G201 -- internal literal table/column names
				`ALTER TABLE %s DROP COLUMN %s`, c.table, c.column)); err != nil {
				return fmt.Errorf("drop %s.%s: %w", c.table, c.column, err)
			}
		}
		return nil
	})
}

// convertScopedTablesToInt rebuilds each legacy TEXT-server_id scoped table to
// the int-FK schema, stamping defaultID. Each call is a no-op when the table is
// already int (fresh install / already-migrated).
func convertScopedTablesToInt(db *sql.DB, defaultID int) error {
	if err := rebuildLegacyServerIDToInt(db, "welcome_grants", "welcome_grants_int",
		`CREATE TABLE welcome_grants_int (
			server_id       INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			fls_id          TEXT    NOT NULL,
			package_version TEXT    NOT NULL,
			account_id      INTEGER NOT NULL,
			character_name  TEXT    NOT NULL DEFAULT '',
			status          TEXT    NOT NULL,
			granted_at      TEXT    NOT NULL DEFAULT '',
			attempts        INTEGER NOT NULL DEFAULT 1,
			last_error      TEXT    NOT NULL DEFAULT '',
			detected_at     TEXT    NOT NULL DEFAULT '',
			updated_at      TEXT    NOT NULL DEFAULT '',
			PRIMARY KEY (server_id, fls_id, package_version, account_id)
		)`,
		[]string{"fls_id", "package_version", "account_id", "character_name", "status",
			"granted_at", "attempts", "last_error", "detected_at", "updated_at"}, defaultID); err != nil {
		return err
	}
	if err := rebuildLegacyServerIDToInt(db, "event_award_claims", "event_award_claims_int",
		`CREATE TABLE event_award_claims_int (
			server_id       INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			event_id        INTEGER NOT NULL,
			version         INTEGER NOT NULL,
			account_id      INTEGER NOT NULL,
			status          TEXT    NOT NULL,
			claimed_at      TEXT    NOT NULL DEFAULT '',
			attempts        INTEGER NOT NULL DEFAULT 1,
			last_error      TEXT    NOT NULL DEFAULT '',
			next_attempt_at TEXT    NOT NULL DEFAULT '',
			updated_at      TEXT    NOT NULL,
			PRIMARY KEY (server_id, event_id, version, account_id)
		)`,
		[]string{"event_id", "version", "account_id", "status", "claimed_at", "attempts",
			"last_error", "next_attempt_at", "updated_at"}, defaultID); err != nil {
		return err
	}
	if err := rebuildLegacyServerIDToInt(db, "battlepass_claims", "battlepass_claims_int",
		`CREATE TABLE battlepass_claims_int (
			server_id  INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			tier_key   TEXT    NOT NULL,
			account_id INTEGER NOT NULL,
			status     TEXT    NOT NULL,
			intel      INTEGER NOT NULL DEFAULT 0,
			earned_at  TEXT    NOT NULL DEFAULT '',
			granted_at TEXT    NOT NULL DEFAULT '',
			attempts   INTEGER NOT NULL DEFAULT 0,
			last_error TEXT    NOT NULL DEFAULT '',
			updated_at TEXT    NOT NULL,
			PRIMARY KEY (server_id, tier_key, account_id)
		)`,
		[]string{"tier_key", "account_id", "status", "intel", "earned_at", "granted_at",
			"attempts", "last_error", "updated_at"}, defaultID); err != nil {
		return err
	}
	if err := rebuildLegacyServerIDToInt(db, "battlepass_accounts", "battlepass_accounts_int",
		`CREATE TABLE battlepass_accounts_int (
			server_id    INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			account_id   INTEGER NOT NULL,
			baselined_at TEXT    NOT NULL,
			PRIMARY KEY (server_id, account_id)
		)`,
		[]string{"account_id", "baselined_at"}, defaultID); err != nil {
		return err
	}
	return convertScopedTablesToIntRest(db, defaultID)
}

// convertScopedTablesToIntRest handles the remaining scoped tables; split out to
// keep convertScopedTablesToInt within the cognitive-complexity gate.
func convertScopedTablesToIntRest(db *sql.DB, defaultID int) error {
	if err := rebuildLegacyServerIDToInt(db, "battlepass_grant_ledger", "battlepass_grant_ledger_int",
		`CREATE TABLE battlepass_grant_ledger_int (
			server_id       INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			tier_key        TEXT    NOT NULL,
			account_id      INTEGER NOT NULL,
			status          TEXT    NOT NULL DEFAULT 'pending',
			attempts        INTEGER NOT NULL DEFAULT 0,
			last_error      TEXT    NOT NULL DEFAULT '',
			next_attempt_at TEXT    NOT NULL DEFAULT '',
			updated_at      TEXT    NOT NULL,
			PRIMARY KEY (server_id, tier_key, account_id)
		)`,
		[]string{"tier_key", "account_id", "status", "attempts", "last_error",
			"next_attempt_at", "updated_at"}, defaultID); err != nil {
		return err
	}
	if err := rebuildLegacyServerIDToInt(db, "play_sessions", "play_sessions_int",
		`CREATE TABLE play_sessions_int (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id     INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			account_id    INTEGER NOT NULL,
			started_at    TEXT    NOT NULL,
			ended_at      TEXT,
			duration_secs INTEGER
		)`,
		[]string{"account_id", "started_at", "ended_at", "duration_secs"}, defaultID); err != nil {
		return err
	}
	if err := rebuildLegacyServerIDToInt(db, "stat_snapshots", "stat_snapshots_int",
		`CREATE TABLE stat_snapshots_int (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id       INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			account_id      INTEGER NOT NULL,
			snapped_at      TEXT    NOT NULL,
			char_xp         INTEGER, skill_points INTEGER, intel_points INTEGER,
			combat_xp       INTEGER, crafting_xp INTEGER, gathering_xp INTEGER,
			exploration_xp  INTEGER, sabotage_xp INTEGER, solaris_balance INTEGER
		)`,
		[]string{"account_id", "snapped_at", "char_xp", "skill_points", "intel_points",
			"combat_xp", "crafting_xp", "gathering_xp", "exploration_xp", "sabotage_xp",
			"solaris_balance"}, defaultID); err != nil {
		return err
	}
	// welcome_config / give_packs_config / event_definitions / battlepass_tiers
	// are handled separately: welcome/give blobs are decomposed below (which also
	// drops their blob columns), and event/tier catalogs gain server_id during
	// their own rebuilds in the blob path. Convert the two config blob tables now
	// so they carry int server_id before the blob decomposition reads them.
	return convertConfigBlobTablesToInt(db, defaultID)
}

// convertConfigBlobTablesToInt rebuilds welcome_config / give_packs_config /
// event_definitions / battlepass_tiers to int server_id, preserving the legacy
// blob columns on the config tables so migrateLegacy*Blobs can still read them.
func convertConfigBlobTablesToInt(db *sql.DB, defaultID int) error {
	wcType, err := columnType(db, "welcome_config", "server_id")
	if err != nil {
		return err
	}
	if wcType != "INTEGER" {
		if err := rebuildLegacyServerIDToInt(db, "welcome_config", "welcome_config_int",
			`CREATE TABLE welcome_config_int (
				server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
				enabled INTEGER NOT NULL DEFAULT 0, scan_secs INTEGER NOT NULL DEFAULT 30,
				active_version TEXT NOT NULL DEFAULT '',
				active_versions_json TEXT NOT NULL DEFAULT '', packages_json TEXT NOT NULL DEFAULT '[]',
				welcome_message_enabled INTEGER NOT NULL DEFAULT 0, welcome_message TEXT NOT NULL DEFAULT '',
				welcome_whisper_source_player TEXT NOT NULL DEFAULT '',
				motd_enabled INTEGER NOT NULL DEFAULT 0, motd_message TEXT NOT NULL DEFAULT '',
				motd_source_player TEXT NOT NULL DEFAULT '',
				region_join_enabled INTEGER NOT NULL DEFAULT 0, region_leave_enabled INTEGER NOT NULL DEFAULT 0,
				region_join_template TEXT NOT NULL DEFAULT '', region_leave_template TEXT NOT NULL DEFAULT '',
				region_chat_channel TEXT NOT NULL DEFAULT 'whisper', updated_at TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (server_id)
			)`,
			[]string{"enabled", "scan_secs", "active_version", "active_versions_json", "packages_json",
				"welcome_message_enabled", "welcome_message", "welcome_whisper_source_player",
				"motd_enabled", "motd_message", "motd_source_player", "region_join_enabled",
				"region_leave_enabled", "region_join_template", "region_leave_template",
				"region_chat_channel", "updated_at"}, defaultID); err != nil {
			return err
		}
	}
	return convertGiveAndCatalogBlobTablesToInt(db, defaultID)
}

func convertGiveAndCatalogBlobTablesToInt(db *sql.DB, defaultID int) error {
	gpType, err := columnType(db, "give_packs_config", "server_id")
	if err != nil {
		return err
	}
	if gpType != "INTEGER" {
		if err := rebuildLegacyServerIDToInt(db, "give_packs_config", "give_packs_config_int",
			`CREATE TABLE give_packs_config_int (
				server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
				base_packs_loaded INTEGER NOT NULL DEFAULT 0,
				packs_json TEXT NOT NULL DEFAULT '[]', updated_at TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (server_id)
			)`,
			[]string{"base_packs_loaded", "packs_json", "updated_at"}, defaultID); err != nil {
			return err
		}
	}
	// event_definitions / battlepass_tiers: a 0.39.5 store has no server_id on
	// these (catalogs were global). Full-rebuild them to the int-FK schema (NOT a
	// plain ALTER) so they ON DELETE CASCADE like every other scoped table. Their
	// id / tier_key are preserved so child rows (event config/reward, claims)
	// stay attached.
	if err := rebuildCatalogServerIDToInt(db, "event_definitions", "event_definitions_int",
		`CREATE TABLE event_definitions_int (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id           INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			name                TEXT    NOT NULL DEFAULT '',
			type                TEXT    NOT NULL DEFAULT '',
			enabled             INTEGER NOT NULL DEFAULT 0,
			version             INTEGER NOT NULL DEFAULT 1,
			config_json         TEXT    NOT NULL DEFAULT '{}',
			reward_json         TEXT    NOT NULL DEFAULT '',
			announce_channel_id TEXT    NOT NULL DEFAULT '',
			announce_template   TEXT    NOT NULL DEFAULT '',
			poll_seconds        INTEGER NOT NULL DEFAULT 7,
			jitter_seconds      INTEGER NOT NULL DEFAULT 3,
			created_at          TEXT    NOT NULL DEFAULT '',
			updated_at          TEXT    NOT NULL DEFAULT ''
		)`,
		[]string{"id", "name", "type", "enabled", "version", "config_json", "reward_json",
			"announce_channel_id", "announce_template", "poll_seconds", "jitter_seconds",
			"created_at", "updated_at"}, defaultID); err != nil {
		return err
	}
	return rebuildCatalogServerIDToInt(db, "battlepass_tiers", "battlepass_tiers_int",
		`CREATE TABLE battlepass_tiers_int (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id    INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			tier_key     TEXT    NOT NULL,
			category     TEXT    NOT NULL DEFAULT '',
			label        TEXT    NOT NULL DEFAULT '',
			signal       TEXT    NOT NULL DEFAULT '',
			signal_key   TEXT    NOT NULL DEFAULT '',
			threshold    INTEGER NOT NULL DEFAULT 0,
			intel        INTEGER NOT NULL DEFAULT 0,
			reward_items TEXT    NOT NULL DEFAULT '',
			enabled      INTEGER NOT NULL DEFAULT 1,
			created_at   TEXT    NOT NULL DEFAULT '',
			updated_at   TEXT    NOT NULL DEFAULT '',
			UNIQUE (server_id, tier_key)
		)`,
		[]string{"id", "tier_key", "category", "label", "signal", "signal_key", "threshold",
			"intel", "reward_items", "enabled", "created_at", "updated_at"}, defaultID)
}

// rebuildCatalogServerIDToInt rebuilds a pre-scoping catalog table (no server_id,
// or text server_id) to the int-FK schema in newDDL, stamping defaultID. Unlike
// rebuildLegacyServerIDToInt it copies only the candidateCols that actually exist
// in the old table, tolerating schema drift across versions (older catalogs may
// lack newer columns — those take the new DDL's defaults). candidateCols MUST
// include the business key (id / tier_key) so child references stay valid.
func rebuildCatalogServerIDToInt(db *sql.DB, table, tmpName, newDDL string, candidateCols []string, defaultID int) error {
	typ, err := columnType(db, table, "server_id")
	if err != nil {
		return err
	}
	if typ == "INTEGER" {
		return nil // already migrated
	}
	present := make([]string, 0, len(candidateCols))
	for _, c := range candidateCols {
		ct, err := columnType(db, table, c)
		if err != nil {
			return err
		}
		if ct != "" {
			present = append(present, c)
		}
	}
	colList := strings.Join(present, ", ")
	return withForeignKeysDisabled(context.Background(), db, func(conn *sql.Conn) error {
		ctx := context.Background()
		if _, err := conn.ExecContext(ctx, newDDL); err != nil {
			return fmt.Errorf("rebuild %s: create %s: %w", table, tmpName, err)
		}
		if _, err := conn.ExecContext(ctx, fmt.Sprintf(
			`INSERT INTO %s (server_id, %s) SELECT %d, %s FROM %s`,
			tmpName, colList, defaultID, colList, table,
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

// legacyDiscordGuild holds the legacy single-guild config read from
// app_config_discord, decomposed into the new shape: a guild row (roles) plus its
// one server link (default server → that guild, channels + status).
type legacyDiscordGuild struct {
	guild  discordGuild
	server discordServerLink
}

// seedDiscordGuilds seeds ONE discord_guilds row (roles) + ONE discord_servers
// row (default server → that guild, legacy announce/status channels + status)
// from the legacy single-guild config, so a single-guild install behaves exactly
// as before. Run-once (migrated:discord_guilds_seed); a no-op when discord_guilds
// already has rows or no legacy guild_id is set. The legacy fields are read from
// app_config_discord (the config.yaml import populated them).
func seedDiscordGuilds(db *sql.DB, defaultID int) error {
	return runColumnMigrationOnce(db, "migrated:discord_guilds_seed", func(tx *sql.Tx) error {
		var existing int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM discord_guilds`).Scan(&existing); err != nil {
			return fmt.Errorf("count discord_guilds: %w", err)
		}
		if existing > 0 {
			return nil // already populated (e.g. via API) — don't overwrite
		}
		lg, ok, err := readLegacyDiscordGuild(tx, defaultID)
		if err != nil {
			return err
		}
		if !ok {
			return nil // no legacy guild_id → nothing to seed (fresh / Discord unused)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := tx.Exec(`
			INSERT INTO discord_guilds (guild_id, roles_viewer, roles_economy, roles_admin, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			lg.guild.GuildID, lg.guild.RolesViewer, lg.guild.RolesEconomy, lg.guild.RolesAdmin, now, now); err != nil {
			return fmt.Errorf("seed discord guild %q: %w", lg.guild.GuildID, err)
		}
		statusEnabled := 0
		if lg.server.StatusEnabled {
			statusEnabled = 1
		}
		if _, err := tx.Exec(`
			INSERT INTO discord_servers
				(server_id, guild_id, announce_channel_id, status_channel_id, status_enabled, status_interval_seconds)
			VALUES (?, ?, ?, ?, ?, ?)`,
			lg.server.ServerID, lg.server.GuildID, lg.server.AnnounceChannelID, lg.server.StatusChannelID,
			statusEnabled, lg.server.StatusIntervalSeconds); err != nil {
			return fmt.Errorf("seed discord server %d→%q: %w", lg.server.ServerID, lg.server.GuildID, err)
		}
		return nil
	})
}

// readLegacyDiscordGuild reads the legacy single-guild fields out of
// app_config_discord into the new shape scoped to defaultID. ok=false when the
// table/row is absent or no guild_id is configured.
func readLegacyDiscordGuild(tx *sql.Tx, defaultID int) (legacyDiscordGuild, bool, error) {
	if typ, err := columnTypeTx(tx, "app_config_discord", "guild_id"); err != nil || typ == "" {
		return legacyDiscordGuild{}, false, err
	}
	var guildID, announce, statusChannel string
	var roleViewer, roleEconomy, roleAdmin string
	var intervalSeconds int
	var statusEnabled sql.NullInt64
	err := tx.QueryRow(`
		SELECT guild_id, announce_channel_id, status_channel_id, status_enabled,
		       status_interval_seconds, roles_viewer, roles_economy, roles_admin
		FROM app_config_discord WHERE id = 1`).Scan(
		&guildID, &announce, &statusChannel, &statusEnabled,
		&intervalSeconds, &roleViewer, &roleEconomy, &roleAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return legacyDiscordGuild{}, false, nil
	}
	if err != nil {
		return legacyDiscordGuild{}, false, fmt.Errorf("read legacy discord config: %w", err)
	}
	if guildID == "" {
		return legacyDiscordGuild{}, false, nil
	}
	return legacyDiscordGuild{
		guild: discordGuild{
			GuildID: guildID, RolesViewer: roleViewer, RolesEconomy: roleEconomy, RolesAdmin: roleAdmin,
		},
		server: discordServerLink{
			ServerID: defaultID, GuildID: guildID,
			AnnounceChannelID: announce, StatusChannelID: statusChannel,
			StatusEnabled:         statusEnabled.Valid && statusEnabled.Int64 != 0,
			StatusIntervalSeconds: intervalSeconds,
		},
	}, true, nil
}

// legacyUserLink is one row copied from the legacy Postgres dune.discord_links
// table into the SQLite discord_user_links store.
type legacyUserLink struct {
	discordUserID string
	accountID     int64
	characterName string
	avatarURL     string
}

// legacyLinkReader reads the legacy Postgres dune.discord_links rows. Injected
// so the migration is unit-testable without a live Postgres; production binds it
// to cmdReadLegacyDiscordLinks against the default server's pool.
type legacyLinkReader func(ctx context.Context) ([]legacyUserLink, error)

// migrateLegacyDiscordUserLinks copies existing registrations from the legacy
// Postgres dune.discord_links table into the unified SQLite discord_user_links
// store, scoped to defaultID, so single-character registrations survive the move
// off Postgres. Best-effort: a missing table or unreachable Postgres is logged
// and the marker is NOT set, so a later boot with Postgres available retries.
// Marker-gated (migrated:discord_user_links) so it copies at most once.
func migrateLegacyDiscordUserLinks(db *sql.DB, defaultID int, read legacyLinkReader) {
	if db == nil {
		return
	}
	marker, err := metaGet(db, "migrated:discord_user_links")
	if err != nil {
		componentLog("store").Warn().Err(err).Msg("discord_user_links: read marker")
		return
	}
	if marker != "" {
		return // already migrated
	}
	if read == nil {
		return // no Postgres pool available this boot — retry next time
	}
	links, err := read(context.Background())
	if err != nil {
		componentLog("store").Warn().Err(err).Msg("discord_user_links: read legacy Postgres links (best-effort) — will retry")
		return
	}
	if err := writeLegacyUserLinks(db, defaultID, links); err != nil {
		componentLog("store").Warn().Err(err).Msg("discord_user_links: write to SQLite")
		return
	}
	if err := metaSet(db, "migrated:discord_user_links", "done"); err != nil {
		componentLog("store").Warn().Err(err).Msg("discord_user_links: set marker")
		return
	}
	componentLog("store").Info().Int("count", len(links)).Msg("discord_user_links: migrated legacy registrations")
}

// writeLegacyUserLinks inserts the legacy links into discord_user_links, scoped
// to defaultID. Skipped rows (empty user id) are ignored. The whole copy is one
// transaction so a partial failure rolls back and the marker stays unset.
func writeLegacyUserLinks(db *sql.DB, defaultID int, links []legacyUserLink) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, l := range links {
		if l.discordUserID == "" {
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO discord_user_links
				(discord_user_id, server_id, account_id, character_name, avatar_url, registered_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(discord_user_id, server_id) DO NOTHING`,
			l.discordUserID, defaultID, l.accountID, l.characterName, l.avatarURL, now); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert legacy link %q: %w", l.discordUserID, err)
		}
	}
	return tx.Commit()
}

// legacyWelcomeBlobRow is one decoded 0.39.5 welcome_config blob row.
type legacyWelcomeBlobRow struct {
	serverID       int
	packages       []welcomePackage
	activeVersions []string
}

// readLegacyWelcomeBlobs buffers + decodes every welcome_config blob row.
func readLegacyWelcomeBlobs(tx *sql.Tx) ([]legacyWelcomeBlobRow, error) {
	rows, err := tx.Query(`SELECT server_id, packages_json, active_versions_json FROM welcome_config`)
	if err != nil {
		return nil, fmt.Errorf("read legacy welcome blobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var all []legacyWelcomeBlobRow
	for rows.Next() {
		var b legacyWelcomeBlobRow
		var pkgJSON, avJSON string
		if err := rows.Scan(&b.serverID, &pkgJSON, &avJSON); err != nil {
			return nil, fmt.Errorf("scan legacy welcome: %w", err)
		}
		if err := decodeJSONList(pkgJSON, &b.packages); err != nil {
			return nil, fmt.Errorf("decode welcome packages %d: %w", b.serverID, err)
		}
		if err := decodeJSONList(avJSON, &b.activeVersions); err != nil {
			return nil, fmt.Errorf("decode welcome active versions %d: %w", b.serverID, err)
		}
		all = append(all, b)
	}
	return all, rows.Err()
}

// migrateLegacyWelcomeBlobs decomposes the 0.39.5 welcome_config blob columns
// (packages_json / active_versions_json) into the surrogate-id child tables,
// once, guarded by migrated:welcome_v2.
func migrateLegacyWelcomeBlobs(db *sql.DB, _ int) error {
	has, err := columnType(db, "welcome_config", "packages_json")
	if err != nil {
		return err
	}
	if has == "" {
		return nil // no legacy blob columns (fresh install)
	}
	return runColumnMigrationOnce(db, "migrated:welcome_v2", func(tx *sql.Tx) error {
		all, err := readLegacyWelcomeBlobs(tx)
		if err != nil {
			return err
		}
		for _, b := range all {
			if err := saveWelcomePackagesColumns(tx, b.serverID, b.packages, b.activeVersions); err != nil {
				return err
			}
		}
		return nil
	})
}

// legacyGivePacksBlobRow is one decoded 0.39.5 give_packs_config blob row.
type legacyGivePacksBlobRow struct {
	serverID int
	packs    []givePack
}

// readLegacyGivePacksBlobs buffers + decodes every give_packs_config blob row.
func readLegacyGivePacksBlobs(tx *sql.Tx) ([]legacyGivePacksBlobRow, error) {
	rows, err := tx.Query(`SELECT server_id, packs_json FROM give_packs_config`)
	if err != nil {
		return nil, fmt.Errorf("read legacy give_packs blobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var all []legacyGivePacksBlobRow
	for rows.Next() {
		var b legacyGivePacksBlobRow
		var packsJSON string
		if err := rows.Scan(&b.serverID, &packsJSON); err != nil {
			return nil, fmt.Errorf("scan legacy give_packs: %w", err)
		}
		if packsJSON != "" {
			if err := json.Unmarshal([]byte(packsJSON), &b.packs); err != nil {
				return nil, fmt.Errorf("decode give_packs %d: %w", b.serverID, err)
			}
		}
		all = append(all, b)
	}
	return all, rows.Err()
}

// migrateLegacyGivePacksBlobs decomposes the 0.39.5 give_packs_config.packs_json
// blob into the surrogate-id child tables, once, guarded by migrated:give_packs_v2.
func migrateLegacyGivePacksBlobs(db *sql.DB, _ int) error {
	has, err := columnType(db, "give_packs_config", "packs_json")
	if err != nil {
		return err
	}
	if has == "" {
		return nil
	}
	return runColumnMigrationOnce(db, "migrated:give_packs_v2", func(tx *sql.Tx) error {
		all, err := readLegacyGivePacksBlobs(tx)
		if err != nil {
			return err
		}
		for _, b := range all {
			if err := saveGivePacksColumns(tx, b.serverID, b.packs); err != nil {
				return err
			}
		}
		return nil
	})
}
