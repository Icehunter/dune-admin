package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// setLocalPassword updates the local dashboard login credentials. This is the
// lockout-recovery path: it works without a running server or a reachable
// Discord API. It writes config.yaml (the recovery seed / downgrade path) and,
// when a migrated unified store exists, the live app_settings row too.
func setLocalPassword(path, username, plain string) error {
	return setLocalPasswordToStore(path, resolveStoreDBPath(), username, plain)
}

// setLocalPasswordToStore is setLocalPassword with an explicit store-DB path so
// it is testable. It hashes plain once and writes the same hash to both sinks:
// config.yaml and — if a migrated store exists at storeDBPath — the DB's
// app_settings row. Post-migration the DB is the source of truth and config.yaml
// is never re-read, so without the store write a --set-password change silently
// no-ops and the operator stays locked out.
func setLocalPasswordToStore(configFilePath, storeDBPath, username, plain string) error {
	if username == "" {
		return fmt.Errorf("username must not be empty")
	}
	if plain == "" {
		return fmt.Errorf("password must not be empty")
	}
	hash, err := hashPassword(plain)
	if err != nil {
		return err
	}
	if err := writeLocalCredsToConfigFile(configFilePath, username, hash); err != nil {
		return err
	}
	if err := syncLocalCredsToStore(storeDBPath, username, hash); err != nil {
		return fmt.Errorf("update store credentials: %w", err)
	}
	return nil
}

// writeLocalCredsToConfigFile merges the credentials into config.yaml at path
// (creating it if missing) without disturbing any other configured fields.
func writeLocalCredsToConfigFile(path, username, hash string) error {
	var cfg appConfig
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parse existing config: %w", err)
		}
	}
	cfg.AuthLocalUsername = username
	cfg.AuthLocalPasswordHash = hash
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// syncLocalCredsToStore updates the DB-backed global-settings credentials when a
// migrated unified store already exists at dbPath. It is a no-op when there is no
// store yet (fresh / pre-migration install): config.yaml is authoritative then
// and its credentials are carried into the DB on the first migrating boot. Only
// the auth fields are touched; all other global settings are preserved.
func syncLocalCredsToStore(dbPath, username, hash string) error {
	if dbPath == "" || dbPath == ":memory:" {
		return nil
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil // no unified store yet → config.yaml import will carry creds
	}
	db, err := openUnifiedStore(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	store := newSettingsStore(db)
	cfg, ok, err := store.loadSettings()
	if err != nil {
		return err
	}
	if !ok {
		// Store file exists but settings were never migrated; config.yaml remains
		// the seed and will be imported on first boot. Nothing to update here.
		return nil
	}
	cfg.AuthLocalUsername = username
	cfg.AuthLocalPasswordHash = hash
	return store.saveSettings(cfg)
}

// applyNewLocalPassword hashes the write-only auth_local_password_new field
// into auth_local_password_hash and clears the plaintext. Called by
// handleSaveConfig before persisting, so the plaintext never reaches disk.
func applyNewLocalPassword(cfg *appConfig) error {
	if cfg.AuthLocalPasswordNew == "" {
		return nil
	}
	hash, err := hashPassword(cfg.AuthLocalPasswordNew)
	if err != nil {
		return err
	}
	cfg.AuthLocalPasswordHash = hash
	cfg.AuthLocalPasswordNew = ""
	return nil
}

// runSetPasswordMode implements the --set-password CLI flag: prompts for a
// username and password on stdin, writes them to config.yaml, and exits.
func runSetPasswordMode() error {
	r := bufio.NewReader(os.Stdin)
	fmt.Print("Dashboard username [admin]: ")
	username, _ := r.ReadString('\n')
	username = strings.TrimSpace(username)
	if username == "" {
		username = "admin"
	}
	fmt.Print("New password: ")
	password, _ := r.ReadString('\n')
	password = strings.TrimSpace(password)
	if err := setLocalPassword(configPath(), username, password); err != nil {
		return err
	}
	fmt.Printf("Local login updated for %q in %s\n", username, configPath())
	fmt.Println("Set auth_enabled: true in the config to enforce dashboard login.")
	return nil
}
