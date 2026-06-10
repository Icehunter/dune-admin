package main

import (
	"testing"
)

// openMemWelcomeStore opens an in-memory welcome store for testing.
func openMemWelcomeStore(t *testing.T) *welcomeStore {
	t.Helper()
	s, err := openWelcomeStore(":memory:")
	if err != nil {
		t.Fatalf("openWelcomeStore: %v", err)
	}
	t.Cleanup(func() { _ = s.close() })
	return s
}

func TestWelcomeConfigStore_SaveAndLoad(t *testing.T) {
	t.Parallel()
	s := openMemWelcomeStore(t)

	cfg := welcomeConfigRow{
		Enabled:       true,
		ScanSecs:      60,
		ActiveVersion: "v2",
		PackagesJSON:  `[{"version":"v2","items":[]}]`,
	}
	if err := s.saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	got, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !ok {
		t.Fatal("expected config to be present after save")
	}
	if got.Enabled != true || got.ScanSecs != 60 || got.ActiveVersion != "v2" || got.PackagesJSON != `[{"version":"v2","items":[]}]` {
		t.Fatalf("loaded config mismatch: %+v", got)
	}
}

func TestWelcomeConfigStore_LoadMissingReturnsNotOK(t *testing.T) {
	t.Parallel()
	s := openMemWelcomeStore(t)

	_, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for empty store, got true")
	}
}

func TestWelcomeConfigStore_WelcomeMessageFieldsRoundTrip(t *testing.T) {
	t.Parallel()
	s := openMemWelcomeStore(t)

	cfg := welcomeConfigRow{
		Enabled:                    true,
		ScanSecs:                   30,
		ActiveVersion:              "v1",
		PackagesJSON:               `[]`,
		WelcomeMessageEnabled:      true,
		WelcomeMessage:             "Welcome to the server! Enjoy your starter pack.",
		WelcomeWhisperSourcePlayer: "some-fls-id-123",
	}
	if err := s.saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	got, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !ok {
		t.Fatal("expected config to be present after save")
	}
	if !got.WelcomeMessageEnabled {
		t.Error("WelcomeMessageEnabled: want true, got false")
	}
	if got.WelcomeMessage != cfg.WelcomeMessage {
		t.Errorf("WelcomeMessage: want %q, got %q", cfg.WelcomeMessage, got.WelcomeMessage)
	}
	if got.WelcomeWhisperSourcePlayer != cfg.WelcomeWhisperSourcePlayer {
		t.Errorf("WelcomeWhisperSourcePlayer: want %q, got %q", cfg.WelcomeWhisperSourcePlayer, got.WelcomeWhisperSourcePlayer)
	}
}

func TestWelcomeConfigStore_MotdFieldsRoundTrip(t *testing.T) {
	t.Parallel()
	s := openMemWelcomeStore(t)

	cfg := welcomeConfigRow{
		Enabled:          false, // MOTD is independent of the package being enabled
		ScanSecs:         30,
		PackagesJSON:     `[]`,
		MotdEnabled:      true,
		MotdMessage:      "Welcome back, {player}!",
		MotdSourcePlayer: "gm-fls-id-789",
	}
	if err := s.saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	got, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !ok {
		t.Fatal("expected config to be present after save")
	}
	if !got.MotdEnabled {
		t.Error("MotdEnabled: want true, got false")
	}
	if got.MotdMessage != cfg.MotdMessage {
		t.Errorf("MotdMessage: want %q, got %q", cfg.MotdMessage, got.MotdMessage)
	}
	if got.MotdSourcePlayer != cfg.MotdSourcePlayer {
		t.Errorf("MotdSourcePlayer: want %q, got %q", cfg.MotdSourcePlayer, got.MotdSourcePlayer)
	}
}

func TestWelcomeConfigStore_ActiveVersionsRoundTrip(t *testing.T) {
	t.Parallel()
	s := openMemWelcomeStore(t)

	cfg := welcomeConfigRow{
		Enabled:        true,
		ScanSecs:       30,
		ActiveVersion:  "v1",
		ActiveVersions: []string{"v1", "v2"},
		PackagesJSON:   `[{"version":"v1","items":[]},{"version":"v2","items":[]}]`,
	}
	if err := s.saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	got, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !ok {
		t.Fatal("expected config after save")
	}
	if len(got.ActiveVersions) != 2 || got.ActiveVersions[0] != "v1" || got.ActiveVersions[1] != "v2" {
		t.Fatalf("ActiveVersions: want [v1 v2], got %v", got.ActiveVersions)
	}
	if got.ActiveVersion != "v1" {
		t.Fatalf("ActiveVersion compat: want v1, got %q", got.ActiveVersion)
	}
}

func TestWelcomeConfigStore_ActiveVersionsCompatFallback(t *testing.T) {
	t.Parallel()
	s := openMemWelcomeStore(t)

	// Simulate a legacy row: active_versions_json is empty, active_version is set.
	cfg := welcomeConfigRow{
		Enabled:        true,
		ScanSecs:       30,
		ActiveVersion:  "v1",
		ActiveVersions: nil, // not set — old-style
		PackagesJSON:   `[{"version":"v1","items":[]}]`,
	}
	if err := s.saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	got, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !ok {
		t.Fatal("expected config after save")
	}
	// loadConfig should promote active_version into ActiveVersions when json is empty.
	if len(got.ActiveVersions) != 1 || got.ActiveVersions[0] != "v1" {
		t.Fatalf("compat fallback: want [v1], got %v", got.ActiveVersions)
	}
}

func TestWelcomeConfigStore_OverwriteWithSave(t *testing.T) {
	t.Parallel()
	s := openMemWelcomeStore(t)

	first := welcomeConfigRow{Enabled: false, ScanSecs: 30, ActiveVersion: "v1", PackagesJSON: `[]`}
	second := welcomeConfigRow{Enabled: true, ScanSecs: 120, ActiveVersion: "v2", PackagesJSON: `[{"version":"v2","items":[]}]`}

	if err := s.saveConfig(first); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := s.saveConfig(second); err != nil {
		t.Fatalf("second save: %v", err)
	}

	got, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !ok {
		t.Fatal("expected config after second save")
	}
	if got.ActiveVersion != "v2" || got.ScanSecs != 120 {
		t.Fatalf("second save did not overwrite: %+v", got)
	}
}
