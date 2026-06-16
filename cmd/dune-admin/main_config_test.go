package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigDirEnvOverride(t *testing.T) {
	t.Setenv("DUNE_ADMIN_CONFIG_DIR", "/tmp/test-override")
	got := configDir()
	if got != "/tmp/test-override" {
		t.Errorf("configDir() = %q, want %q", got, "/tmp/test-override")
	}
}

// needsSetupConfigured must return false once any per-server entry exists, so a
// multi-server install (or a server added via POST /servers) isn't stuck on the
// setup gate just because the legacy flat db_pass is empty.
func TestNeedsSetupConfigured_MultiServer(t *testing.T) {
	origCfg := loadedConfig
	origPass := dbPass
	t.Cleanup(func() { loadedConfig = origCfg; dbPass = origPass })

	dbPass = ""
	loadedConfig = appConfig{Servers: []ServerConfig{{ID: 1}}}
	if needsSetupConfigured() {
		t.Error("needsSetupConfigured() = true with Servers[]; want false")
	}

	loadedConfig = appConfig{} // no servers, no flat pass
	if !needsSetupConfigured() {
		t.Error("needsSetupConfigured() = false with empty config; want true")
	}
}

func TestConfigDirDefault(t *testing.T) {
	// Ensure no override is set for this test.
	t.Setenv("DUNE_ADMIN_CONFIG_DIR", "")
	got := configDir()
	if got == "" || got == "/tmp/test-override" {
		t.Errorf("configDir() = %q, expected a home-based path", got)
	}
}

func TestPreserveMaskedSecrets(t *testing.T) {
	t.Parallel()

	const mask = "••••••••"

	write := func(t *testing.T, cfg appConfig) string {
		t.Helper()
		dir := t.TempDir()
		p := filepath.Join(dir, "config.yaml")
		data, err := yaml.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// NOTE: db/broker/amp secrets are per-server now and are NOT handled by
	// preserveMaskedSecrets (they round-trip via the servers store). This test
	// only covers the app-level secrets the function still restores.

	t.Run("DiscordBotToken placeholder is preserved from file", func(t *testing.T) {
		t.Parallel()
		path := write(t, appConfig{DiscordBotToken: "real-discord-token"})
		cfg := appConfig{DiscordBotToken: mask}
		preserveMaskedSecrets(&cfg, os.ReadFile, path)
		if cfg.DiscordBotToken != "real-discord-token" {
			t.Fatalf("expected real-discord-token, got %q", cfg.DiscordBotToken)
		}
	})

	t.Run("MarketBotRemoteToken placeholder is preserved from file", func(t *testing.T) {
		t.Parallel()
		path := write(t, appConfig{MarketBotRemoteToken: "real-token"})
		cfg := appConfig{MarketBotRemoteToken: mask}
		preserveMaskedSecrets(&cfg, os.ReadFile, path)
		if cfg.MarketBotRemoteToken != "real-token" {
			t.Fatalf("expected real-token, got %q", cfg.MarketBotRemoteToken)
		}
	})

	t.Run("non-masked values pass through unchanged", func(t *testing.T) {
		t.Parallel()
		path := write(t, appConfig{DiscordBotToken: "old", MarketBotRemoteToken: "old"})
		cfg := appConfig{DiscordBotToken: "new", MarketBotRemoteToken: "new"}
		preserveMaskedSecrets(&cfg, os.ReadFile, path)
		if cfg.DiscordBotToken != "new" || cfg.MarketBotRemoteToken != "new" {
			t.Fatal("non-masked values should not be changed")
		}
	})

	t.Run("missing file does not write mask string to config", func(t *testing.T) {
		t.Parallel()
		cfg := appConfig{
			DiscordBotToken:      mask,
			MarketBotRemoteToken: mask,
		}
		preserveMaskedSecrets(&cfg, os.ReadFile, "/nonexistent/path/config.yaml")
		if cfg.DiscordBotToken == mask || cfg.MarketBotRemoteToken == mask {
			t.Fatal("mask placeholder must never be written to config file")
		}
	})
}

func TestHandleGetConfigMasksSecrets(t *testing.T) {
	// Not parallel: mutates package-level dbPass global.
	orig := dbPass
	dbPass = "supersecret"
	t.Cleanup(func() { dbPass = orig })

	cfg := buildCurrentConfig()
	if cfg.DBPass != "••••••••" {
		t.Fatalf("expected masked DBPass, got %q", cfg.DBPass)
	}
}
