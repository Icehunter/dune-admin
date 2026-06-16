package main

import "testing"

func TestSettingsStore_LoadEmptyFirstBoot(t *testing.T) {
	db := openSharedScopeDB(t)
	s := newSettingsStore(db)

	_, ok, err := s.loadSettings()
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	if ok {
		t.Error("loadSettings ok=true on first boot; want false (no row)")
	}
}

func TestSettingsStore_SaveStripsPerServerFields(t *testing.T) {
	db := openSharedScopeDB(t)
	s := newSettingsStore(db)

	cfg := appConfig{
		// Global settings — must survive.
		ListenAddr:      ":9090",
		DiscordBotToken: "tok",
		ScripCurrency:   42,
		// DefaultServerName is an app-level display field — it must survive.
		DefaultServerName: "Three",
		// Per-server / flat connection — must be stripped.
		DBHost:        "10.0.0.1",
		DBPass:        "secret",
		Control:       "amp",
		DefaultServer: "3",
		Servers:       []ServerConfig{{ID: 1, Name: "One"}},
	}
	if err := s.saveSettings(cfg); err != nil {
		t.Fatalf("saveSettings: %v", err)
	}

	got, ok, err := s.loadSettings()
	if err != nil || !ok {
		t.Fatalf("loadSettings: ok=%v err=%v", ok, err)
	}
	// Global fields preserved (DefaultServerName is app-level and survives).
	if got.ListenAddr != ":9090" || got.DiscordBotToken != "tok" || got.ScripCurrency != 42 {
		t.Errorf("global fields not preserved: %+v", got)
	}
	if got.DefaultServerName != "Three" {
		t.Errorf("DefaultServerName = %q, want Three (app-level field must survive)", got.DefaultServerName)
	}
	// Per-server / flat fields stripped.
	if got.DBHost != "" || got.DBPass != "" || got.Control != "" {
		t.Errorf("flat connection fields not stripped: DBHost=%q DBPass=%q Control=%q", got.DBHost, got.DBPass, got.Control)
	}
	if len(got.Servers) != 0 || got.DefaultServer != "" {
		t.Errorf("per-server fields not stripped: Servers=%d DefaultServer=%q", len(got.Servers), got.DefaultServer)
	}
}

func TestSettingsStore_SaveIsUpsert(t *testing.T) {
	db := openSharedScopeDB(t)
	s := newSettingsStore(db)

	if err := s.saveSettings(appConfig{ListenAddr: ":1"}); err != nil {
		t.Fatalf("saveSettings 1: %v", err)
	}
	if err := s.saveSettings(appConfig{ListenAddr: ":2"}); err != nil {
		t.Fatalf("saveSettings 2: %v", err)
	}
	got, _, _ := s.loadSettings()
	if got.ListenAddr != ":2" {
		t.Errorf("ListenAddr = %q after second save, want :2 (single-row upsert)", got.ListenAddr)
	}

	// Exactly one row must exist (CHECK id = 1). Settings now live in the typed
	// app_config_* tables; app_config_misc is the canonical single row.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM app_config_misc`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("app_config_misc has %d rows, want 1", n)
	}
}

func TestMetaGetSet(t *testing.T) {
	db := openSharedScopeDB(t)

	v, err := metaGet(db, "missing")
	if err != nil || v != "" {
		t.Errorf("metaGet missing = %q, %v; want empty, nil", v, err)
	}
	if err := metaSet(db, "k", "v1"); err != nil {
		t.Fatalf("metaSet: %v", err)
	}
	if v, _ := metaGet(db, "k"); v != "v1" {
		t.Errorf("metaGet = %q, want v1", v)
	}
	// Upsert.
	_ = metaSet(db, "k", "v2")
	if v, _ := metaGet(db, "k"); v != "v2" {
		t.Errorf("metaGet after upsert = %q, want v2", v)
	}
}
