package main

import (
	"database/sql"
	"errors"
	"fmt"
)

// app_config_columns.go stores the APP-LEVEL (non-per-server) settings as typed
// rows across the app_config_* tables (id=1 each). Per-server connection /
// provider / market-bot-enable fields live on the servers table, NOT here. The
// appConfig struct and its json/yaml tags are unchanged; only storage moves to
// columns.

// dbExecer / dbRowQueryer are satisfied by both *sql.DB and *sql.Tx, so the
// column writers/readers work inside a migration transaction or standalone.
type dbExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}
type dbRowQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

const appConfigColumnsSchema = `
CREATE TABLE IF NOT EXISTS app_config_market_bot (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	cache_db TEXT NOT NULL DEFAULT '',
	item_data TEXT NOT NULL DEFAULT '', state TEXT NOT NULL DEFAULT '',
	buy_interval TEXT NOT NULL DEFAULT '', list_interval TEXT NOT NULL DEFAULT '',
	buy_threshold REAL NOT NULL DEFAULT 0, max_buys INTEGER NOT NULL DEFAULT 0,
	remote_url TEXT NOT NULL DEFAULT '', remote_token TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS app_config_discord (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	bot_enabled INTEGER, bot_token TEXT NOT NULL DEFAULT '',
	guild_id TEXT NOT NULL DEFAULT '', roles_viewer TEXT NOT NULL DEFAULT '',
	roles_economy TEXT NOT NULL DEFAULT '', roles_admin TEXT NOT NULL DEFAULT '',
	announce_channel_id TEXT NOT NULL DEFAULT '', status_enabled INTEGER,
	status_channel_id TEXT NOT NULL DEFAULT '', status_interval_seconds INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS app_config_auth (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	auth_enabled INTEGER, auth_local_username TEXT NOT NULL DEFAULT '',
	auth_local_password_hash TEXT NOT NULL DEFAULT '', auth_discord_enabled INTEGER,
	auth_discord_client_id TEXT NOT NULL DEFAULT '', auth_discord_client_secret TEXT NOT NULL DEFAULT '',
	auth_discord_redirect_url TEXT NOT NULL DEFAULT '', auth_owner_discord_ids TEXT NOT NULL DEFAULT '',
	auth_owner_role_ids TEXT NOT NULL DEFAULT '', auth_session_ttl_hours INTEGER NOT NULL DEFAULT 0,
	auth_guest_enabled INTEGER, auth_cookie_samesite TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS app_config_battlepass (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	enabled INTEGER, award_past INTEGER, auto_grant INTEGER,
	poll_seconds INTEGER NOT NULL DEFAULT 0, scan_pace_ms INTEGER NOT NULL DEFAULT 0,
	scan_start_delay_ms INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS app_config_welcome (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	enabled INTEGER, scan_interval_secs INTEGER NOT NULL DEFAULT 0,
	active_version TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS app_config_misc (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	listen_addr TEXT NOT NULL DEFAULT '', scrip_currency INTEGER NOT NULL DEFAULT 0,
	events_enabled INTEGER, default_server_name TEXT NOT NULL DEFAULT ''
);`

// initAppConfigColumnsSchema creates the app_config_* tables. Idempotent.
func initAppConfigColumnsSchema(db *sql.DB) error {
	if _, err := db.Exec(appConfigColumnsSchema); err != nil {
		return fmt.Errorf("init app_config columns schema: %w", err)
	}
	return nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// saveAppConfigColumns upserts the app-level fields of cfg across the
// app_config_* tables (id=1 each). Per-server / connection fields are ignored
// (they live on the servers table).
func saveAppConfigColumns(db dbExecer, cfg appConfig) error {
	stmts := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO app_config_market_bot (id, cache_db, item_data, state,
			buy_interval, list_interval, buy_threshold, max_buys, remote_url, remote_token)
			VALUES (1,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET
			cache_db=excluded.cache_db, item_data=excluded.item_data, state=excluded.state,
			buy_interval=excluded.buy_interval, list_interval=excluded.list_interval,
			buy_threshold=excluded.buy_threshold, max_buys=excluded.max_buys, remote_url=excluded.remote_url,
			remote_token=excluded.remote_token`,
			[]any{cfg.MarketBotCacheDB, cfg.MarketBotItemData,
				cfg.MarketBotState, cfg.MarketBotBuyInt, cfg.MarketBotListInt, cfg.MarketBotThresh,
				cfg.MarketBotMaxBuys, cfg.MarketBotRemoteURL, cfg.MarketBotRemoteToken}},

		{`INSERT INTO app_config_discord (id, bot_enabled, bot_token, guild_id, roles_viewer, roles_economy,
			roles_admin, announce_channel_id, status_enabled, status_channel_id, status_interval_seconds)
			VALUES (1,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET bot_enabled=excluded.bot_enabled, bot_token=excluded.bot_token,
			guild_id=excluded.guild_id, roles_viewer=excluded.roles_viewer, roles_economy=excluded.roles_economy,
			roles_admin=excluded.roles_admin, announce_channel_id=excluded.announce_channel_id,
			status_enabled=excluded.status_enabled, status_channel_id=excluded.status_channel_id,
			status_interval_seconds=excluded.status_interval_seconds`,
			[]any{boolPtrToNullInt(cfg.DiscordBotEnabled), cfg.DiscordBotToken, cfg.DiscordGuildID,
				cfg.DiscordRolesViewer, cfg.DiscordRolesEconomy, cfg.DiscordRolesAdmin,
				cfg.DiscordAnnounceChannelID, boolPtrToNullInt(cfg.DiscordStatusEnabled),
				cfg.DiscordStatusChannelID, cfg.DiscordStatusIntervalSeconds}},

		{`INSERT INTO app_config_auth (id, auth_enabled, auth_local_username, auth_local_password_hash,
			auth_discord_enabled, auth_discord_client_id, auth_discord_client_secret, auth_discord_redirect_url,
			auth_owner_discord_ids, auth_owner_role_ids, auth_session_ttl_hours, auth_guest_enabled,
			auth_cookie_samesite)
			VALUES (1,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET auth_enabled=excluded.auth_enabled,
			auth_local_username=excluded.auth_local_username,
			auth_local_password_hash=excluded.auth_local_password_hash,
			auth_discord_enabled=excluded.auth_discord_enabled,
			auth_discord_client_id=excluded.auth_discord_client_id,
			auth_discord_client_secret=excluded.auth_discord_client_secret,
			auth_discord_redirect_url=excluded.auth_discord_redirect_url,
			auth_owner_discord_ids=excluded.auth_owner_discord_ids,
			auth_owner_role_ids=excluded.auth_owner_role_ids,
			auth_session_ttl_hours=excluded.auth_session_ttl_hours,
			auth_guest_enabled=excluded.auth_guest_enabled,
			auth_cookie_samesite=excluded.auth_cookie_samesite`,
			[]any{boolPtrToNullInt(cfg.AuthEnabled), cfg.AuthLocalUsername, cfg.AuthLocalPasswordHash,
				boolPtrToNullInt(cfg.AuthDiscordEnabled), cfg.AuthDiscordClientID, cfg.AuthDiscordClientSecret,
				cfg.AuthDiscordRedirectURL, cfg.AuthOwnerDiscordIDs, cfg.AuthOwnerRoleIDs,
				cfg.AuthSessionTTLHours, boolPtrToNullInt(cfg.AuthGuestEnabled), cfg.AuthCookieSameSite}},

		{`INSERT INTO app_config_battlepass (id, enabled, award_past, auto_grant, poll_seconds, scan_pace_ms,
			scan_start_delay_ms)
			VALUES (1,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET enabled=excluded.enabled, award_past=excluded.award_past,
			auto_grant=excluded.auto_grant, poll_seconds=excluded.poll_seconds, scan_pace_ms=excluded.scan_pace_ms,
			scan_start_delay_ms=excluded.scan_start_delay_ms`,
			[]any{boolPtrToNullInt(cfg.BattlepassEnabled), boolPtrToNullInt(cfg.BattlepassAwardPast),
				boolPtrToNullInt(cfg.BattlepassAutoGrant), cfg.BattlepassPollSeconds, cfg.BattlepassScanPaceMs,
				cfg.BattlepassScanStartDelayMs}},

		{`INSERT INTO app_config_welcome (id, enabled, scan_interval_secs, active_version)
			VALUES (1,?,?,?)
			ON CONFLICT(id) DO UPDATE SET enabled=excluded.enabled,
			scan_interval_secs=excluded.scan_interval_secs, active_version=excluded.active_version`,
			[]any{boolPtrToNullInt(cfg.WelcomePackageEnabled), cfg.WelcomePackageScanSecs,
				cfg.WelcomePackageActiveVersion}},

		{`INSERT INTO app_config_misc (id, listen_addr, scrip_currency, events_enabled, default_server_name)
			VALUES (1,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET listen_addr=excluded.listen_addr,
			scrip_currency=excluded.scrip_currency,
			events_enabled=excluded.events_enabled,
			default_server_name=excluded.default_server_name`,
			[]any{cfg.ListenAddr, cfg.ScripCurrency,
				boolPtrToNullInt(cfg.EventsEnabled), cfg.DefaultServerName}},
	}
	for _, st := range stmts {
		if _, err := db.Exec(st.sql, st.args...); err != nil {
			return fmt.Errorf("save app_config columns: %w", err)
		}
	}
	return nil
}

// optRow tolerates a missing row: an absent app_config_* row leaves the struct's
// zero values, matching the legacy blob's "field unset" semantics.
func optRow(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

// loadAppConfigColumns reads the app-level settings from the app_config_* tables.
// ok=false on first boot (no app_config_misc row yet — nothing persisted).
func loadAppConfigColumns(db dbRowQueryer) (appConfig, bool, error) {
	cfg, ok, err := loadMiscAppConfig(db)
	if err != nil || !ok {
		return appConfig{}, ok, err
	}
	loaders := []func(dbRowQueryer, *appConfig) error{
		loadMarketBotAppConfig, loadDiscordAppConfig, loadAuthAppConfig,
		loadBattlepassAppConfig, loadWelcomeAppConfig,
	}
	for _, load := range loaders {
		if err := load(db, &cfg); err != nil {
			return appConfig{}, false, err
		}
	}
	return cfg, true, nil
}

// loadMiscAppConfig reads app_config_misc, the canonical presence marker: every
// saveAppConfigColumns writes it, so its absence means "nothing persisted yet".
func loadMiscAppConfig(db dbRowQueryer) (appConfig, bool, error) {
	var cfg appConfig
	var eventsEnabled sql.NullInt64
	err := db.QueryRow(`SELECT listen_addr, scrip_currency,
		events_enabled, default_server_name FROM app_config_misc WHERE id = 1`).Scan(
		&cfg.ListenAddr, &cfg.ScripCurrency,
		&eventsEnabled, &cfg.DefaultServerName)
	if errors.Is(err, sql.ErrNoRows) {
		return appConfig{}, false, nil
	}
	if err != nil {
		return appConfig{}, false, fmt.Errorf("load app_config_misc: %w", err)
	}
	cfg.EventsEnabled = nullIntToBoolPtr(eventsEnabled)
	return cfg, true, nil
}

func loadMarketBotAppConfig(db dbRowQueryer, cfg *appConfig) error {
	err := db.QueryRow(`SELECT cache_db, item_data, state, buy_interval,
		list_interval, buy_threshold, max_buys, remote_url, remote_token FROM app_config_market_bot
		WHERE id = 1`).Scan(
		&cfg.MarketBotCacheDB, &cfg.MarketBotItemData, &cfg.MarketBotState,
		&cfg.MarketBotBuyInt, &cfg.MarketBotListInt, &cfg.MarketBotThresh, &cfg.MarketBotMaxBuys,
		&cfg.MarketBotRemoteURL, &cfg.MarketBotRemoteToken)
	return optRow(err)
}

func loadDiscordAppConfig(db dbRowQueryer, cfg *appConfig) error {
	var discordBotEnabled, discordStatusEnabled sql.NullInt64
	err := db.QueryRow(`SELECT bot_enabled, bot_token, guild_id, roles_viewer, roles_economy,
		roles_admin, announce_channel_id, status_enabled, status_channel_id, status_interval_seconds
		FROM app_config_discord WHERE id = 1`).Scan(
		&discordBotEnabled, &cfg.DiscordBotToken, &cfg.DiscordGuildID, &cfg.DiscordRolesViewer,
		&cfg.DiscordRolesEconomy, &cfg.DiscordRolesAdmin, &cfg.DiscordAnnounceChannelID,
		&discordStatusEnabled, &cfg.DiscordStatusChannelID, &cfg.DiscordStatusIntervalSeconds)
	cfg.DiscordBotEnabled = nullIntToBoolPtr(discordBotEnabled)
	cfg.DiscordStatusEnabled = nullIntToBoolPtr(discordStatusEnabled)
	return optRow(err)
}

func loadAuthAppConfig(db dbRowQueryer, cfg *appConfig) error {
	var authEnabled, authDiscordEnabled, authGuestEnabled sql.NullInt64
	err := db.QueryRow(`SELECT auth_enabled, auth_local_username, auth_local_password_hash,
		auth_discord_enabled, auth_discord_client_id, auth_discord_client_secret, auth_discord_redirect_url,
		auth_owner_discord_ids, auth_owner_role_ids, auth_session_ttl_hours, auth_guest_enabled,
		auth_cookie_samesite FROM app_config_auth WHERE id = 1`).Scan(
		&authEnabled, &cfg.AuthLocalUsername, &cfg.AuthLocalPasswordHash, &authDiscordEnabled,
		&cfg.AuthDiscordClientID, &cfg.AuthDiscordClientSecret, &cfg.AuthDiscordRedirectURL,
		&cfg.AuthOwnerDiscordIDs, &cfg.AuthOwnerRoleIDs, &cfg.AuthSessionTTLHours, &authGuestEnabled,
		&cfg.AuthCookieSameSite)
	cfg.AuthEnabled = nullIntToBoolPtr(authEnabled)
	cfg.AuthDiscordEnabled = nullIntToBoolPtr(authDiscordEnabled)
	cfg.AuthGuestEnabled = nullIntToBoolPtr(authGuestEnabled)
	return optRow(err)
}

func loadBattlepassAppConfig(db dbRowQueryer, cfg *appConfig) error {
	var bpEnabled, bpAwardPast, bpAutoGrant sql.NullInt64
	err := db.QueryRow(`SELECT enabled, award_past, auto_grant, poll_seconds, scan_pace_ms,
		scan_start_delay_ms FROM app_config_battlepass WHERE id = 1`).Scan(
		&bpEnabled, &bpAwardPast, &bpAutoGrant, &cfg.BattlepassPollSeconds, &cfg.BattlepassScanPaceMs,
		&cfg.BattlepassScanStartDelayMs)
	cfg.BattlepassEnabled = nullIntToBoolPtr(bpEnabled)
	cfg.BattlepassAwardPast = nullIntToBoolPtr(bpAwardPast)
	cfg.BattlepassAutoGrant = nullIntToBoolPtr(bpAutoGrant)
	return optRow(err)
}

func loadWelcomeAppConfig(db dbRowQueryer, cfg *appConfig) error {
	var welcomeEnabled sql.NullInt64
	err := db.QueryRow(`SELECT enabled, scan_interval_secs, active_version FROM app_config_welcome
		WHERE id = 1`).Scan(
		&welcomeEnabled, &cfg.WelcomePackageScanSecs, &cfg.WelcomePackageActiveVersion)
	cfg.WelcomePackageEnabled = nullIntToBoolPtr(welcomeEnabled)
	return optRow(err)
}
