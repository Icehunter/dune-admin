package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
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
// the settings blob holds only global config (auth, Discord, market-bot tuning,
// feature flags, listen addr, scrip currency).
func globalSettingsOnly(cfg appConfig) appConfig {
	cfg.Servers = nil
	cfg.DefaultServer = ""
	cfg.DefaultServerName = ""
	clearFlatConnectionConfig(&cfg) // drop flat connection + secrets (per-server)
	return cfg
}

// saveSettings upserts the single global-settings row (id = 1).
func (s *settingsStore) saveSettings(cfg appConfig) error {
	now := time.Now().UTC().Format(time.RFC3339)
	blob, err := json.Marshal(globalSettingsOnly(cfg))
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if _, err := s.db.Exec(
		`INSERT INTO app_settings (id, config_json, updated_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET config_json = excluded.config_json, updated_at = excluded.updated_at`,
		string(blob), now); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}

// loadSettings reads the global-settings row. ok=false on first boot (no row).
func (s *settingsStore) loadSettings() (appConfig, bool, error) {
	var blob string
	err := s.db.QueryRow(`SELECT config_json FROM app_settings WHERE id = 1`).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return appConfig{}, false, nil
	}
	if err != nil {
		return appConfig{}, false, fmt.Errorf("load settings: %w", err)
	}
	var cfg appConfig
	if err := json.Unmarshal([]byte(blob), &cfg); err != nil {
		return appConfig{}, false, fmt.Errorf("unmarshal settings: %w", err)
	}
	return cfg, true, nil
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
