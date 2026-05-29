package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

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

	t.Run("BrokerPass placeholder is preserved from file", func(t *testing.T) {
		t.Parallel()
		path := write(t, appConfig{BrokerPass: "real-broker-pass"})
		cfg := appConfig{BrokerPass: mask}
		preserveMaskedSecrets(&cfg, os.ReadFile, path)
		if cfg.BrokerPass != "real-broker-pass" {
			t.Fatalf("expected real-broker-pass, got %q", cfg.BrokerPass)
		}
	})

	t.Run("BrokerJWTSecret placeholder is preserved from file", func(t *testing.T) {
		t.Parallel()
		path := write(t, appConfig{BrokerJWTSecret: "real-jwt-secret"})
		cfg := appConfig{BrokerJWTSecret: mask}
		preserveMaskedSecrets(&cfg, os.ReadFile, path)
		if cfg.BrokerJWTSecret != "real-jwt-secret" {
			t.Fatalf("expected real-jwt-secret, got %q", cfg.BrokerJWTSecret)
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
		path := write(t, appConfig{BrokerPass: "old", BrokerJWTSecret: "old", MarketBotRemoteToken: "old"})
		cfg := appConfig{BrokerPass: "new", BrokerJWTSecret: "new", MarketBotRemoteToken: "new"}
		preserveMaskedSecrets(&cfg, os.ReadFile, path)
		if cfg.BrokerPass != "new" || cfg.BrokerJWTSecret != "new" || cfg.MarketBotRemoteToken != "new" {
			t.Fatal("non-masked values should not be changed")
		}
	})

	t.Run("missing file does not write mask string to config", func(t *testing.T) {
		t.Parallel()
		cfg := appConfig{
			DBPass:               mask,
			BrokerPass:           mask,
			BrokerJWTSecret:      mask,
			MarketBotRemoteToken: mask,
		}
		preserveMaskedSecrets(&cfg, os.ReadFile, "/nonexistent/path/config.yaml")
		if cfg.DBPass == mask || cfg.BrokerPass == mask || cfg.BrokerJWTSecret == mask || cfg.MarketBotRemoteToken == mask {
			t.Fatal("mask placeholder must never be written to config file")
		}
	})
}

func TestHandleGetConfigMasksSecrets(t *testing.T) {
	t.Parallel()
	orig := dbPass
	dbPass = "supersecret"
	t.Cleanup(func() { dbPass = orig })

	cfg := buildCurrentConfig()
	if cfg.DBPass != "••••••••" {
		t.Fatalf("expected masked DBPass, got %q", cfg.DBPass)
	}
}
