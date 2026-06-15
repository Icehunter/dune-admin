package main

import (
	"encoding/json"
	"testing"
)

// TestServerColumnsRoundTrip inserts a fully-populated ServerConfig and asserts
// it survives a getServer/listServers round-trip via the typed columns,
// including the DB-assigned id, *bool tri-state, plain bools, ints and secrets.
func TestServerColumnsRoundTrip(t *testing.T) {
	db := openSharedScopeDB(t)
	s := newServersStore(db)

	cfg := ServerConfig{
		Name:             "Prod",
		SSHHost:          "10.0.0.1",
		SSHUser:          "amp",
		DBHost:           "db.internal",
		DBPort:           15432,
		DBPass:           "db-secret",
		DBName:           "dune",
		Control:          "amp",
		AutoDiscover:     true,
		BrokerTLS:        true,
		BrokerPass:       "broker-secret",
		BrokerJWTSecret:  "jwt-secret",
		AmpInstance:      "DuneAwakening01",
		AmpAPIPass:       "amp-secret",
		AmpAPIPort:       8081,
		AmpUseContainer:  boolPtr(true),
		MarketBotEnabled: boolPtr(false),
	}

	id, err := s.insertServer(cfg, 0)
	if err != nil {
		t.Fatalf("insertServer: %v", err)
	}

	got, ok, err := s.getServer(id)
	if err != nil || !ok {
		t.Fatalf("getServer: ok=%v err=%v", ok, err)
	}

	assertServerEqual(t, "getServer", got, cfg, id)

	list, err := s.listServers()
	if err != nil {
		t.Fatalf("listServers: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("listServers len = %d, want 1", len(list))
	}
	assertServerEqual(t, "listServers", list[0], cfg, id)
	if list[0].LegacyID != "" {
		t.Errorf("LegacyID = %q, want empty", list[0].LegacyID)
	}
}

func assertServerEqual(t *testing.T, ctx string, got, want ServerConfig, id int) {
	t.Helper()
	if got.ID != id {
		t.Errorf("%s: ID = %d, want %d", ctx, got.ID, id)
	}
	if got.Name != want.Name {
		t.Errorf("%s: Name = %q, want %q", ctx, got.Name, want.Name)
	}
	if got.SSHHost != want.SSHHost || got.SSHUser != want.SSHUser {
		t.Errorf("%s: ssh = %q/%q, want %q/%q", ctx, got.SSHHost, got.SSHUser, want.SSHHost, want.SSHUser)
	}
	if got.DBHost != want.DBHost || got.DBPort != want.DBPort || got.DBName != want.DBName {
		t.Errorf("%s: db = %q/%d/%q, want %q/%d/%q", ctx, got.DBHost, got.DBPort, got.DBName,
			want.DBHost, want.DBPort, want.DBName)
	}
	if got.Control != want.Control {
		t.Errorf("%s: Control = %q, want %q", ctx, got.Control, want.Control)
	}
	if got.AutoDiscover != want.AutoDiscover {
		t.Errorf("%s: AutoDiscover = %v, want %v", ctx, got.AutoDiscover, want.AutoDiscover)
	}
	if got.BrokerTLS != want.BrokerTLS {
		t.Errorf("%s: BrokerTLS = %v, want %v", ctx, got.BrokerTLS, want.BrokerTLS)
	}
	if got.AmpInstance != want.AmpInstance || got.AmpAPIPort != want.AmpAPIPort {
		t.Errorf("%s: amp = %q/%d, want %q/%d", ctx, got.AmpInstance, got.AmpAPIPort,
			want.AmpInstance, want.AmpAPIPort)
	}
	// Secrets must survive intact.
	if got.DBPass != want.DBPass || got.BrokerPass != want.BrokerPass ||
		got.BrokerJWTSecret != want.BrokerJWTSecret || got.AmpAPIPass != want.AmpAPIPass {
		t.Errorf("%s: secrets not preserved: %+v", ctx, got)
	}
	// *bool tri-state: explicit false must stay false, not nil.
	if got.MarketBotEnabled == nil || *got.MarketBotEnabled {
		t.Errorf("%s: MarketBotEnabled = %v, want explicit false", ctx, got.MarketBotEnabled)
	}
	if got.AmpUseContainer == nil || !*got.AmpUseContainer {
		t.Errorf("%s: AmpUseContainer = %v, want explicit true", ctx, got.AmpUseContainer)
	}
}

// TestServerColumns_PointerBoolNilStaysNil ensures an unset *bool round-trips as
// nil (NULL column), distinct from an explicit false.
func TestServerColumns_PointerBoolNilStaysNil(t *testing.T) {
	db := openSharedScopeDB(t)
	s := newServersStore(db)

	id, err := s.insertServer(ServerConfig{Name: "NoBot", MarketBotEnabled: nil}, 0)
	if err != nil {
		t.Fatalf("insertServer: %v", err)
	}
	got, ok, err := s.getServer(id)
	if err != nil || !ok {
		t.Fatalf("getServer: ok=%v err=%v", ok, err)
	}
	if got.MarketBotEnabled != nil {
		t.Errorf("MarketBotEnabled = %v, want nil (unset)", got.MarketBotEnabled)
	}
	if got.AmpUseContainer != nil {
		t.Errorf("AmpUseContainer = %v, want nil (unset)", got.AmpUseContainer)
	}
}

// TestMigrateServersColumns inserts a legacy row populated only via config_json,
// runs the migration, and asserts the typed columns are backfilled and the
// run-once marker is set.
func TestMigrateServersColumns(t *testing.T) {
	db := openSharedScopeDB(t)

	legacy := ServerConfig{DBHost: "legacy.db", DBPass: "legacy-secret", Name: "S"}
	blob, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res, err := db.Exec(
		`INSERT INTO servers (name, position, config_json, created_at, updated_at) VALUES (?,?,?,?,?)`,
		"S", 0, string(blob), "", "")
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	id64, _ := res.LastInsertId()
	id := int(id64)

	if err := migrateServersColumns(db); err != nil {
		t.Fatalf("migrateServersColumns: %v", err)
	}

	got, err := readServerColumns(db, id)
	if err != nil {
		t.Fatalf("readServerColumns: %v", err)
	}
	if got.DBHost != "legacy.db" || got.DBPass != "legacy-secret" {
		t.Errorf("typed columns not backfilled from blob: %+v", got)
	}
	if m, _ := metaGet(db, "migrated:servers_columns"); m == "" {
		t.Error("migrated:servers_columns marker not set")
	}
}
