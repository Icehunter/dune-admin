package main

import (
	"errors"
	"testing"

	"dune-admin/internal/marketbot"
)

// TestApplyMarketBotConfig_StopClearsBot verifies that when wantEnabled=false,
// applyMarketBotConfig cancels the running bot and sets embeddedBot to nil.
func TestApplyMarketBotConfig_StopClearsBot(t *testing.T) {
	// Not parallel: mutates package-level embeddedBot / globalBotCancel.
	origBot := embeddedBot
	origCancel := globalBotCancel
	t.Cleanup(func() { embeddedBot = origBot; globalBotCancel = origCancel })

	cancelled := false
	globalBotCancel = func() { cancelled = true }
	// Provide a non-nil embeddedBot instance so the running-check passes.
	embeddedBot = new(marketbot.Instance)

	disabled := false
	cfg := appConfig{MarketBotEnabled: &disabled}
	applyMarketBotConfig(cfg)

	if embeddedBot != nil {
		t.Error("embeddedBot should be nil after disabling")
	}
	if globalBotCancel != nil {
		t.Error("globalBotCancel should be nil after disabling")
	}
	if !cancelled {
		t.Error("cancel function should have been called")
	}
}

// TestApplyMarketBotConfig_StartRequiresDB verifies that applyMarketBotConfig
// does NOT attempt to start the embedded bot when globalDB is nil. This
// enforces the ordering contract: applyMarketBotConfig must only be called
// AFTER connectAll() has established globalDB.
func TestApplyMarketBotConfig_StartRequiresDB(t *testing.T) {
	// Not parallel: mutates package-level embeddedBot / globalDB.
	origBot := embeddedBot
	origCancel := globalBotCancel
	origDB := globalDB
	t.Cleanup(func() { embeddedBot = origBot; globalBotCancel = origCancel; globalDB = origDB })

	embeddedBot = nil
	globalBotCancel = nil
	globalDB = nil // simulate pre-connectAll state

	enabled := true
	cfg := appConfig{MarketBotEnabled: &enabled}
	applyMarketBotConfig(cfg)

	// With globalDB nil, startEmbeddedMarketBotIfEnabled should fail and
	// embeddedBot should remain nil rather than holding a broken instance.
	if embeddedBot != nil {
		t.Error("embeddedBot should remain nil when globalDB is nil (connectAll not yet called)")
	}
}

// TestApplyConfig_SetsBrokerCredentials verifies that applyConfig copies broker
// credentials into the package-level globals so hot-apply works without restart.
func TestApplyConfig_SetsBrokerCredentials(t *testing.T) {
	// Not parallel: mutates package-level globals.
	origUser := brokerUser
	origPass := brokerPass
	origLoaded := loadedConfig
	t.Cleanup(func() {
		brokerUser = origUser
		brokerPass = origPass
		loadedConfig = origLoaded
	})

	cfg := appConfig{
		BrokerUser:      "cap_user",
		BrokerPass:      "cap_pass",
		BrokerJWTSecret: "jwt_secret",
	}
	applyConfig(cfg)

	if brokerUser != "cap_user" {
		t.Errorf("brokerUser = %q, want cap_user", brokerUser)
	}
	if brokerPass != "cap_pass" {
		t.Errorf("brokerPass = %q, want cap_pass", brokerPass)
	}
	// BrokerJWTSecret is read from loadedConfig in buildCaptureJWT; confirm it is set there.
	if loadedConfig.BrokerJWTSecret != "jwt_secret" {
		t.Errorf("loadedConfig.BrokerJWTSecret = %q, want jwt_secret", loadedConfig.BrokerJWTSecret)
	}
}

// PreserveMaskedDBPass exercises the preserveMaskedSecrets function for the
// DBPass field specifically. Not parallel because subtests mutate loadedConfig.
func TestPreserveMaskedDBPass(t *testing.T) {
	t.Run("keeps explicit password", func(t *testing.T) {
		cfg := appConfig{DBPass: "new-pass"}
		preserveMaskedSecrets(&cfg, func(string) ([]byte, error) {
			t.Fatalf("readFile should not be called for explicit password")
			return nil, nil
		}, "/tmp/unused")
		if cfg.DBPass != "new-pass" {
			t.Fatalf("expected explicit password to stay unchanged, got %q", cfg.DBPass)
		}
	})

	t.Run("uses existing config password from file", func(t *testing.T) {
		cfg := appConfig{DBPass: "••••••••"}
		preserveMaskedSecrets(&cfg, func(string) ([]byte, error) {
			return []byte("db_pass: stored-pass\n"), nil
		}, "/tmp/config.yaml")
		if cfg.DBPass != "stored-pass" {
			t.Fatalf("expected stored password from config file, got %q", cfg.DBPass)
		}
	})

	t.Run("falls back to loadedConfig when file missing", func(t *testing.T) {
		orig := loadedConfig
		loadedConfig = appConfig{DBPass: "in-memory-pass"}
		t.Cleanup(func() { loadedConfig = orig })

		cfg := appConfig{DBPass: "••••••••"}
		preserveMaskedSecrets(&cfg, func(string) ([]byte, error) {
			return nil, errors.New("no file")
		}, "/tmp/missing.yaml")
		if cfg.DBPass != "in-memory-pass" {
			t.Fatalf("expected in-memory fallback password, got %q", cfg.DBPass)
		}
	})
}
