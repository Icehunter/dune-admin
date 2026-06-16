package main

import (
	"database/sql"
	"errors"
	"fmt"
)

// settingsStore persists the global (non-per-server) settings as a single-row
// JSON blob in the unified store — the live source of truth (config.yaml is only
// a first-boot import seed). Per-server fields (Servers, DefaultServer, the flat
// connection fields) are stripped before save; they live in the servers table.
type settingsStore struct{ db *sql.DB }

const settingsStoreSchema = `
CREATE TABLE IF NOT EXISTS app_settings (
	id          INTEGER PRIMARY KEY CHECK (id = 1),
	config_json TEXT    NOT NULL DEFAULT '{}',
	updated_at  TEXT    NOT NULL DEFAULT ''
);`

func initSettingsSchema(db *sql.DB) error {
	if _, err := db.Exec(settingsStoreSchema); err != nil {
		return fmt.Errorf("init settings schema: %w", err)
	}
	return nil
}

func newSettingsStore(db *sql.DB) *settingsStore { return &settingsStore{db: db} }

// globalSettingsOnly returns a copy of cfg with all per-server fields cleared so
// the app-config tables hold only app-level config (auth, Discord, market-bot
// tuning, feature flags, listen addr, scrip currency). DefaultServerName is kept
// as it is an app-level display field.
func globalSettingsOnly(cfg appConfig) appConfig {
	cfg.Servers = nil
	cfg.DefaultServer = ""
	clearFlatConnectionConfig(&cfg) // drop flat connection + secrets (per-server)
	return cfg
}

// saveSettings upserts the app-level settings into the app_config_* tables
// (per-server/connection fields stripped via globalSettingsOnly).
func (s *settingsStore) saveSettings(cfg appConfig) error {
	return saveAppConfigColumns(s.db, globalSettingsOnly(cfg))
}

// loadSettings reads the app-level settings from the app_config_* tables.
// ok=false on first boot (no settings persisted yet).
func (s *settingsStore) loadSettings() (appConfig, bool, error) {
	return loadAppConfigColumns(s.db)
}

// active server id (string scope form) persisted across restarts via meta.

func metaGet(db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("meta get %q: %w", key, err)
	}
	return v, nil
}

func metaSet(db *sql.DB, key, value string) error {
	if _, err := db.Exec(
		`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value); err != nil {
		return fmt.Errorf("meta set %q: %w", key, err)
	}
	return nil
}
