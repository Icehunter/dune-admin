package main

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAppConfigAutoDiscoverRoundTrip(t *testing.T) {
	var cfg appConfig
	if err := yaml.Unmarshal([]byte("auto_discover: true\n"), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !cfg.AutoDiscover {
		t.Errorf("AutoDiscover = false, want true")
	}
}

func TestNeedsSetupAutoDiscoverAware(t *testing.T) {
	origPass, origAuto, origCtrl := dbPass, autoDiscover, controlPlane
	origCfg := loadedConfig
	defer func() {
		dbPass, autoDiscover, controlPlane = origPass, origAuto, origCtrl
		loadedConfig = origCfg
	}()
	// needsSetupConfigured short-circuits when Servers[] is non-empty; this test
	// exercises the flat-config path, so ensure no per-server entries leak in.
	loadedConfig = appConfig{}

	// auto-discover + kubectl + empty pass → no setup (discovery supplies it).
	dbPass, autoDiscover, controlPlane = "", true, "kubectl"
	if needsSetupConfigured() {
		t.Errorf("auto_discover=true + kubectl + empty dbPass: want no setup")
	}
	// auto-discover on but not kubectl → setup still required.
	dbPass, autoDiscover, controlPlane = "", true, "local"
	if !needsSetupConfigured() {
		t.Errorf("auto_discover=true + non-kubectl + empty dbPass: want setup")
	}
	// auto-discover off → setup required.
	dbPass, autoDiscover, controlPlane = "", false, "kubectl"
	if !needsSetupConfigured() {
		t.Errorf("auto_discover=false + empty dbPass: want setup")
	}
}

func TestPodIPByPattern(t *testing.T) {
	list := "ns-mq-game-svc-0 10.0.0.5\nns-mq-admin-svc-0 10.0.0.6\nns-bgd-svc-0 10.0.0.7\nns-db-dbdepl-sts-0 10.0.0.8\n"
	cases := map[string]string{"mq-game": "10.0.0.5", "mq-admin": "10.0.0.6", "bgd": "10.0.0.7", "nope": ""}
	for pattern, want := range cases {
		if got := podIPByPattern(list, pattern); got != want {
			t.Errorf("podIPByPattern(%q) = %q, want %q", pattern, got, want)
		}
	}
}

const sampleCmdline = `su dune -c ./DuneSandboxServer.sh Overmap -FarmRegion=Europe ` +
	`-ini:engine:[FuncomLiveServices]:ServiceAuthToken=eyJTOKEN -RMQGameTlsEnabled=true ` +
	`ServerName=sh-xxx -MultiHome=192.168.33.65 -DatabaseName=dune ` +
	`-DatabaseHost=ns-db-dbdepl-svc:15432 -DatabaseUser=dune -DatabasePassword=SECRET123 ` +
	`-PartitionIndex=2 -battlegroup-director-url=ns-bgd-svc:11717 ` +
	`--RMQGameHostname=ns-mq-game-svc --RMQGamePort=5672 ` +
	`--RMQAdminHostname=ns-mq-admin-svc --RMQAdminPort=5672`

func TestParseGameServerArgs(t *testing.T) {
	g := parseGameServerArgs(sampleCmdline)
	checks := map[string]string{
		"DBUser": g.DBUser, "DBPass": g.DBPass, "DBName": g.DBName,
		"RMQGameHost": g.RMQGameHost, "RMQGamePort": g.RMQGamePort,
		"RMQAdminHost": g.RMQAdminHost, "RMQAdminPort": g.RMQAdminPort,
		"DirectorURL": g.DirectorURL,
	}
	want := map[string]string{
		"DBUser": "dune", "DBPass": "SECRET123", "DBName": "dune",
		"RMQGameHost": "ns-mq-game-svc", "RMQGamePort": "5672",
		"RMQAdminHost": "ns-mq-admin-svc", "RMQAdminPort": "5672",
		"DirectorURL": "ns-bgd-svc:11717",
	}
	for k, v := range want {
		if checks[k] != v {
			t.Errorf("%s = %q, want %q", k, checks[k], v)
		}
	}
	if !g.RMQTLS {
		t.Errorf("RMQTLS = false, want true")
	}
}

func TestParseGameServerArgsMissing(t *testing.T) {
	g := parseGameServerArgs("DuneSandboxServer -DatabaseUser=dune")
	if g.DBUser != "dune" {
		t.Errorf("DBUser = %q, want dune", g.DBUser)
	}
	if g.DBPass != "" || g.RMQGameHost != "" || g.RMQTLS {
		t.Errorf("absent fields should be zero values: %+v", g)
	}
}

func TestApplyDiscoveredFillsGapsOnly(t *testing.T) {
	g := gameServerArgs{DBUser: "dune", DBPass: "SECRET123", DBName: "dune"}

	cfg := appConfig{}
	applyDiscovered(&cfg, g)
	if cfg.DBUser != "dune" || cfg.DBPass != "SECRET123" || cfg.DBName != "dune" {
		t.Errorf("empty config not filled: %+v", cfg)
	}

	cfg2 := appConfig{DBUser: "admin", DBPass: "manual"}
	applyDiscovered(&cfg2, g)
	if cfg2.DBUser != "admin" || cfg2.DBPass != "manual" {
		t.Errorf("explicit values overwritten: %+v", cfg2)
	}
	if cfg2.DBName != "dune" {
		t.Errorf("empty DBName should still be filled: %+v", cfg2)
	}
}

func TestApplyDiscoveredRMQDirector(t *testing.T) {
	g := gameServerArgs{
		RMQGamePort: "5672", RMQAdminPort: "5672", RMQTLS: true,
		DirectorURL: "ns-bgd-svc:11717",
	}
	cfg := appConfig{}
	applyDiscoveredEndpoints(&cfg, g, "10.0.0.5", "10.0.0.6", "10.0.0.7")
	if cfg.BrokerGameAddr != "10.0.0.5:5672" {
		t.Errorf("BrokerGameAddr = %q", cfg.BrokerGameAddr)
	}
	if cfg.BrokerAdminAddr != "10.0.0.6:5672" {
		t.Errorf("BrokerAdminAddr = %q", cfg.BrokerAdminAddr)
	}
	if !cfg.BrokerTLS {
		t.Errorf("BrokerTLS = false, want true")
	}
	if cfg.DirectorURL != "http://10.0.0.7:11717" {
		t.Errorf("DirectorURL = %q", cfg.DirectorURL)
	}

	cfg2 := appConfig{BrokerGameAddr: "manual:1234"}
	applyDiscoveredEndpoints(&cfg2, g, "10.0.0.5", "10.0.0.6", "10.0.0.7")
	if cfg2.BrokerGameAddr != "manual:1234" {
		t.Errorf("explicit BrokerGameAddr overwritten: %q", cfg2.BrokerGameAddr)
	}
}

func TestPersistDiscoveredWritesGaps(t *testing.T) {
	g := gameServerArgs{DBUser: "dune", DBPass: "SECRET123", DBName: "dune"}
	cfg := appConfig{DBUser: "keep"}
	out := persistDiscoveredConfig(cfg, g, "", "", "")
	if out.DBUser != "keep" {
		t.Errorf("explicit DBUser overwritten: %q", out.DBUser)
	}
	if out.DBPass != "SECRET123" || out.DBName != "dune" {
		t.Errorf("gaps not filled: %+v", out)
	}
}

// Integration: requires a reachable command-mode target running the game server.
//
//	SSH_CMD_TARGET=vm-dune-01 go test -run TestDiscoverGameConfigIntegration ./...
func TestDiscoverGameConfigIntegration(t *testing.T) {
	target := os.Getenv("SSH_CMD_TARGET")
	if target == "" {
		t.Skip("set SSH_CMD_TARGET to run the discovery integration test")
	}
	exec, err := newSSHCommandExecutor(target, "", "", "")
	if err != nil {
		t.Fatalf("executor: %v", err)
	}
	defer exec.Close()
	g, err := discoverGameConfig(exec)
	if err != nil {
		t.Fatalf("discoverGameConfig: %v", err)
	}
	if g.DBUser == "" || g.DBPass == "" {
		t.Errorf("expected DB user+pass from live args, got user=%q passLen=%d", g.DBUser, len(g.DBPass))
	}
	t.Logf("discovered DBUser=%s DBName=%s RMQGameHost=%s", g.DBUser, g.DBName, g.RMQGameHost)
}
