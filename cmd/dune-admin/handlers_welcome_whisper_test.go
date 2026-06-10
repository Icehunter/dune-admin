package main

import (
	"context"
	"testing"
	"time"
)

// TestBuildWelcomeRuntimeWithMessage verifies message fields survive buildWelcomeRuntime.
func TestBuildWelcomeRuntimeWithMessage(t *testing.T) {
	t.Parallel()
	rt := buildWelcomeRuntime(
		true, []string{"v1"}, 30,
		[]welcomePackage{{Version: "v1"}},
		welcomeMessageOptions{
			enabled:      true,
			message:      "Welcome to Arrakis!",
			sourcePlayer: "Server#0001",
		},
	)
	if !rt.welcomeMessageEnabled {
		t.Fatal("want welcomeMessageEnabled=true")
	}
	if rt.welcomeMessage != "Welcome to Arrakis!" {
		t.Fatalf("wrong welcome message: %q", rt.welcomeMessage)
	}
	if rt.welcomeWhisperSourcePlayer != "Server#0001" {
		t.Fatalf("wrong source player: %q", rt.welcomeWhisperSourcePlayer)
	}
}

func TestBuildWelcomeRuntimeWithMessage_DisabledByDefault(t *testing.T) {
	t.Parallel()
	rt := buildWelcomeRuntime(true, []string{"v1"}, 30, []welcomePackage{{Version: "v1"}}, welcomeMessageOptions{})
	if rt.welcomeMessageEnabled {
		t.Fatal("want welcomeMessageEnabled=false by default")
	}
}

// TestBuildWelcomeRuntimeWithMotd verifies the optional MOTD options flow into
// the runtime and are independent of the package being enabled.
func TestBuildWelcomeRuntimeWithMotd(t *testing.T) {
	t.Parallel()
	rt := buildWelcomeRuntime(
		false, nil, 30, nil, welcomeMessageOptions{},
		motdOptions{enabled: true, message: "Welcome back, {player}!", sourcePlayer: "gm-1"},
	)
	if !rt.motdEnabled {
		t.Fatal("want motdEnabled=true")
	}
	if rt.motdMessage != "Welcome back, {player}!" {
		t.Fatalf("wrong motd message: %q", rt.motdMessage)
	}
	if rt.motdSourcePlayer != "gm-1" {
		t.Fatalf("wrong motd source player: %q", rt.motdSourcePlayer)
	}
	if rt.enabled {
		t.Fatal("package should remain disabled — MOTD is independent")
	}
}

// TestBuildWelcomeRuntimeBackcompat verifies existing behaviour is preserved.
func TestBuildWelcomeRuntimeBackcompat(t *testing.T) {
	t.Parallel()
	pkgs := []welcomePackage{{Version: "v1"}, {Version: "v2"}}
	tests := []struct {
		name         string
		enabled      bool
		active       []string
		scanSecs     int
		wantActive   string
		wantInterval time.Duration
	}{
		{"defaults active to first package", true, nil, 0, "v1", welcomeDefaultScanInterval},
		{"unknown active falls back to first", true, []string{"vX"}, 0, "v1", welcomeDefaultScanInterval},
		{"explicit active respected", true, []string{"v2"}, 120, "v2", 120 * time.Second},
		{"interval below floor is clamped", false, []string{"v1"}, 1, "v1", welcomeDefaultScanInterval},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := buildWelcomeRuntime(tt.enabled, tt.active, tt.scanSecs, pkgs, welcomeMessageOptions{})
			if rt.enabled != tt.enabled {
				t.Fatalf("enabled: want %v, got %v", tt.enabled, rt.enabled)
			}
			firstActive := ""
			if len(rt.activeVersions) > 0 {
				firstActive = rt.activeVersions[0]
			}
			if firstActive != tt.wantActive {
				t.Fatalf("activeVersion: want %q, got %q", tt.wantActive, firstActive)
			}
			if rt.interval != tt.wantInterval {
				t.Fatalf("interval: want %v, got %v", tt.wantInterval, rt.interval)
			}
		})
	}
}

func TestWelcomeConfigRow_RoundTripsMessageFields(t *testing.T) {
	t.Parallel()
	s := openMemWelcomeStore(t)

	cfg := welcomeConfigRow{
		Enabled:                    true,
		ScanSecs:                   30,
		ActiveVersion:              "v1",
		PackagesJSON:               `[]`,
		WelcomeMessageEnabled:      true,
		WelcomeMessage:             "Welcome!",
		WelcomeWhisperSourcePlayer: "gm-fls-id",
	}
	if err := s.saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}
	got, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !ok {
		t.Fatal("expected config row")
	}
	if !got.WelcomeMessageEnabled {
		t.Fatal("WelcomeMessageEnabled not persisted")
	}
	if got.WelcomeMessage != "Welcome!" {
		t.Fatalf("WelcomeMessage not persisted: %q", got.WelcomeMessage)
	}
	if got.WelcomeWhisperSourcePlayer != "gm-fls-id" {
		t.Fatalf("WelcomeWhisperSourcePlayer not persisted: %q", got.WelcomeWhisperSourcePlayer)
	}
}

// TestWelcomeScanSendsWhisperForNewAccount verifies the whisper dep is called
// exactly once per new account, and not again on subsequent scans.
func TestWelcomeScanSendsWhisperForNewAccount(t *testing.T) {
	t.Parallel()

	store := openMemWelcomeStore(t)
	accounts := []welcomeAccount{{AccountID: 10, FlsID: "fls-abc", CharacterName: "Tester"}}

	var whisperCalls int
	deps := welcomeScanDeps{
		listAccounts: func(context.Context) ([]welcomeAccount, error) { return accounts, nil },
		grant: func(_ context.Context, _ int64, _ string, _ []welcomePackageItem) ([]string, error) {
			return nil, nil
		},
		whisper: func(_ context.Context, accountID int64, _ string, msg string) error {
			whisperCalls++
			return nil
		},
		store: store,
	}

	_, _, _, err := welcomePackageScanOnce(context.Background(), "v1", nil, deps)
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if whisperCalls != 1 {
		t.Fatalf("want 1 whisper call, got %d", whisperCalls)
	}

	// Second scan: already in ledger, must not whisper again.
	whisperCalls = 0
	_, _, _, err = welcomePackageScanOnce(context.Background(), "v1", nil, deps)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if whisperCalls != 0 {
		t.Fatalf("whisper must not be resent, got %d calls", whisperCalls)
	}
}

// TestWelcomeScanSkipsWhisperWhenDepNil verifies the scanner does not panic
// when whisper dep is nil (whisper feature disabled).
func TestWelcomeScanSkipsWhisperWhenDepNil(t *testing.T) {
	t.Parallel()
	store := openMemWelcomeStore(t)
	accounts := []welcomeAccount{{AccountID: 10, FlsID: "fls-abc", CharacterName: "Tester"}}

	_, _, _, err := welcomePackageScanOnce(context.Background(), "v1", nil, welcomeScanDeps{
		listAccounts: func(context.Context) ([]welcomeAccount, error) { return accounts, nil },
		grant: func(_ context.Context, _ int64, _ string, _ []welcomePackageItem) ([]string, error) {
			return nil, nil
		},
		whisper: nil,
		store:   store,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
