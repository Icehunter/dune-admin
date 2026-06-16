package main

import "testing"

// TestAppConfigColumnsRoundTrip exercises the app-level config round-trip. Per
// the re-model db/broker/amp secrets are per-server (servers table), so only the
// app-wide secrets (market-bot remote token, Discord token, auth secrets) live
// here. globalSettingsOnly clears the per-server fields before save.
func TestAppConfigColumnsRoundTrip(t *testing.T) {
	db := openSharedScopeDB(t)
	tru := true
	in := globalSettingsOnly(appConfig{
		ListenAddr: ":8080", ScripCurrency: 7,
		DBPass:                      "should-be-cleared-by-globalSettingsOnly", // connection field → cleared
		MarketBotThresh:             1.5,
		MarketBotRemoteToken:        "tok",
		DiscordBotToken:             "dtok",
		DiscordStatusEnabled:        &tru,
		AuthEnabled:                 &tru,
		AuthLocalUsername:           "admin",
		AuthLocalPasswordHash:       "hash",
		AuthDiscordClientSecret:     "secret",
		AuthSessionTTLHours:         24,
		BattlepassEnabled:           &tru,
		BattlepassPollSeconds:       60,
		WelcomePackageEnabled:       &tru,
		WelcomePackageActiveVersion: "v2",
		EventsEnabled:               &tru,
	})
	if err := saveAppConfigColumns(db, in); err != nil {
		t.Fatalf("saveAppConfigColumns: %v", err)
	}
	got, ok, err := loadAppConfigColumns(db)
	if err != nil || !ok {
		t.Fatalf("loadAppConfigColumns: ok=%v err=%v", ok, err)
	}
	// App-level secrets survive.
	if got.AuthLocalPasswordHash != "hash" || got.AuthDiscordClientSecret != "secret" ||
		got.MarketBotRemoteToken != "tok" || got.DiscordBotToken != "dtok" {
		t.Errorf("secret lost in round-trip: %+v", got)
	}
	// Per-server secrets are not stored at app level.
	if got.DBPass != "" {
		t.Errorf("DBPass should be cleared (per-server now), got %q", got.DBPass)
	}
	// Tri-state *bool survives as true.
	for name, p := range map[string]*bool{
		"AuthEnabled": got.AuthEnabled, "DiscordStatusEnabled": got.DiscordStatusEnabled,
		"BattlepassEnabled": got.BattlepassEnabled, "WelcomePackageEnabled": got.WelcomePackageEnabled,
		"EventsEnabled": got.EventsEnabled,
	} {
		if p == nil || !*p {
			t.Errorf("%s *bool lost (want true, got %v)", name, p)
		}
	}
	// Numerics + strings.
	if got.MarketBotThresh != 1.5 || got.AuthSessionTTLHours != 24 ||
		got.ScripCurrency != 7 || got.ListenAddr != ":8080" || got.WelcomePackageActiveVersion != "v2" {
		t.Errorf("scalar lost: %+v", got)
	}
}

func TestAppConfigColumns_BoolPtrFalseAndNil(t *testing.T) {
	db := openSharedScopeDB(t)
	fls := false
	if err := saveAppConfigColumns(db, appConfig{AuthEnabled: &fls}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, _, err := loadAppConfigColumns(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.AuthEnabled == nil || *got.AuthEnabled {
		t.Errorf("AuthEnabled = %v, want explicit false", got.AuthEnabled)
	}
	// DiscordBotEnabled was never set → must round-trip as nil (unset), not false.
	if got.DiscordBotEnabled != nil {
		t.Errorf("DiscordBotEnabled = %v, want nil (unset)", got.DiscordBotEnabled)
	}
}
