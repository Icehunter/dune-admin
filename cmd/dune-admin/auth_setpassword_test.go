package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSetLocalPassword(t *testing.T) {
	// Isolate the unified-store path so the config-only subtests never touch the
	// operator's real ~/.dune-admin/dune-admin.db. The file does not exist, so the
	// store-sync step is a no-op and only config.yaml is written.
	t.Setenv("DUNE_ADMIN_DB", filepath.Join(t.TempDir(), "absent-store.db"))

	t.Run("writes hash and username to existing config", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte("db_host: 1.2.3.4\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := setLocalPassword(path, "admin", "s3cret"); err != nil {
			t.Fatalf("setLocalPassword: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var cfg appConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.DBHost != "1.2.3.4" {
			t.Errorf("existing config lost: db_host = %q", cfg.DBHost)
		}
		if cfg.AuthLocalUsername != "admin" {
			t.Errorf("username = %q, want admin", cfg.AuthLocalUsername)
		}
		if !checkPassword(cfg.AuthLocalPasswordHash, "s3cret") {
			t.Error("stored hash does not match password")
		}
	})

	t.Run("creates config when missing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := setLocalPassword(path, "owner", "pw"); err != nil {
			t.Fatalf("setLocalPassword: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var cfg appConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			t.Fatal(err)
		}
		if !checkPassword(cfg.AuthLocalPasswordHash, "pw") {
			t.Error("stored hash does not match password")
		}
	})

	t.Run("rejects empty password", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := setLocalPassword(path, "admin", ""); err == nil {
			t.Error("empty password accepted")
		}
	})

	t.Run("rejects empty username", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := setLocalPassword(path, "", "pw"); err == nil {
			t.Error("empty username accepted")
		}
	})
}

// TestSetLocalPassword_UpdatesMigratedStore reproduces the post-migration
// lockout-recovery bug: once config.yaml has been imported into the DB, the DB
// is the live source of truth and config.yaml is never re-read, so a password
// set via --set-password must be written into the typed app_config_auth row too.
func TestSetLocalPassword_UpdatesMigratedStore(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	storeDB := filepath.Join(dir, "dune-admin.db")

	// Seed a migrated store: an app_settings row with global settings + stale
	// credentials, exactly as it looks after the config.yaml → DB migration.
	db, err := openUnifiedStore(storeDB)
	if err != nil {
		t.Fatalf("openUnifiedStore: %v", err)
	}
	seedHash, _ := hashPassword("oldpw")
	if err := newSettingsStore(db).saveSettings(appConfig{
		ListenAddr:            ":9999",
		AuthLocalUsername:     "oldadmin",
		AuthLocalPasswordHash: seedHash,
	}); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	_ = db.Close()

	if err := setLocalPasswordToStore(configFile, storeDB, "newadmin", "newpw"); err != nil {
		t.Fatalf("setLocalPasswordToStore: %v", err)
	}

	// The live DB row must carry the new credentials, and unrelated global
	// settings must survive untouched.
	db2, err := openUnifiedStore(storeDB)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer func() { _ = db2.Close() }()
	got, ok, err := newSettingsStore(db2).loadSettings()
	if err != nil || !ok {
		t.Fatalf("loadSettings: ok=%v err=%v", ok, err)
	}
	if got.AuthLocalUsername != "newadmin" {
		t.Errorf("store username = %q, want newadmin", got.AuthLocalUsername)
	}
	if !checkPassword(got.AuthLocalPasswordHash, "newpw") {
		t.Error("store hash does not match the new password")
	}
	if got.ListenAddr != ":9999" {
		t.Errorf("unrelated setting lost: listen_addr = %q, want :9999", got.ListenAddr)
	}

	// The credentials must land in the typed app_config_auth table specifically —
	// not a JSON blob — so they are queryable and the live read path sees them.
	var colUser, colHash string
	if err := db2.QueryRow(
		`SELECT auth_local_username, auth_local_password_hash FROM app_config_auth WHERE id = 1`).
		Scan(&colUser, &colHash); err != nil {
		t.Fatalf("read app_config_auth: %v", err)
	}
	if colUser != "newadmin" || !checkPassword(colHash, "newpw") {
		t.Errorf("app_config_auth not updated: user=%q hash-matches=%v", colUser, checkPassword(colHash, "newpw"))
	}

	// config.yaml is still written as the recovery seed / downgrade path.
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	var cfg appConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if !checkPassword(cfg.AuthLocalPasswordHash, "newpw") {
		t.Error("config.yaml hash does not match the new password")
	}
}

// TestSetLocalPasswordToStore_NoStore confirms that without a unified store the
// call still succeeds and writes config.yaml only (fresh / pre-migration path).
func TestSetLocalPasswordToStore_NoStore(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	absentDB := filepath.Join(dir, "does-not-exist.db")

	if err := setLocalPasswordToStore(configFile, absentDB, "admin", "pw"); err != nil {
		t.Fatalf("setLocalPasswordToStore: %v", err)
	}
	if _, err := os.Stat(absentDB); err == nil {
		t.Error("store DB was created when it should have been left absent")
	}
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	var cfg appConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if !checkPassword(cfg.AuthLocalPasswordHash, "pw") {
		t.Error("config.yaml hash does not match password")
	}
}

func TestApplyNewLocalPassword(t *testing.T) {
	t.Run("hashes plaintext into hash field and clears it", func(t *testing.T) {
		cfg := appConfig{AuthLocalPasswordNew: "newpw", AuthLocalPasswordHash: "old"}
		if err := applyNewLocalPassword(&cfg); err != nil {
			t.Fatalf("applyNewLocalPassword: %v", err)
		}
		if cfg.AuthLocalPasswordNew != "" {
			t.Error("plaintext field not cleared")
		}
		if !checkPassword(cfg.AuthLocalPasswordHash, "newpw") {
			t.Error("hash does not match new password")
		}
	})

	t.Run("no-op when plaintext absent", func(t *testing.T) {
		cfg := appConfig{AuthLocalPasswordHash: "keep"}
		if err := applyNewLocalPassword(&cfg); err != nil {
			t.Fatalf("applyNewLocalPassword: %v", err)
		}
		if cfg.AuthLocalPasswordHash != "keep" {
			t.Errorf("hash changed: %q", cfg.AuthLocalPasswordHash)
		}
	})
}
