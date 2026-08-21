package main

import (
	"testing"
)

// TestServerColumnsRoundTrip inserts a fully-populated ServerConfig and asserts
// it survives a getServer/listServers round-trip via the typed columns,
// including the DB-assigned id, *bool tri-state, plain bools, ints and secrets.
func TestServerColumnsRoundTrip(t *testing.T) {
	db := openMemUnifiedStore(t)
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
	db := openMemUnifiedStore(t)
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

// Note: the legacy servers config_json → typed-column migration
// (migrateServersColumns / readServerColumns) was removed; servers are now
// written to typed columns directly via the servers store (covered by
// TestServerColumnsRoundTrip above).

// The servers table stores DockerGameservers comma-joined (typed columns, not
// JSON), so the split/join pair must round-trip cleanly — including the empty
// case, which must yield a nil slice so auto-detection stays enabled (#311).
func TestContainerListRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"empty", nil, ""},
		{"single", []string{"dune-server-overmap"}, "dune-server-overmap"},
		{"multiple", []string{"dune-server-overmap", "dune-server-survival-1"},
			"dune-server-overmap,dune-server-survival-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := joinContainerList(tc.in)
			if got != tc.want {
				t.Fatalf("joinContainerList = %q, want %q", got, tc.want)
			}
			back := splitContainerList(got)
			if len(back) != len(tc.in) {
				t.Fatalf("round-trip = %v, want %v", back, tc.in)
			}
			for i := range back {
				if back[i] != tc.in[i] {
					t.Errorf("round-trip[%d] = %q, want %q", i, back[i], tc.in[i])
				}
			}
		})
	}
}

// Hand-edited config should survive stray whitespace and empty entries.
func TestSplitContainerList_TrimsAndDropsBlanks(t *testing.T) {
	got := splitContainerList(" a , ,b ,")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("splitContainerList = %v, want [a b]", got)
	}
}
