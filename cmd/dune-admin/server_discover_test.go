package main

import (
	"strings"
	"testing"
)

// gameProcLine is a representative DuneSandboxServer command line carrying the
// connection args discovery parses.
const gameProcLine = "1234 /game/DuneSandboxServer -DatabaseUser=dune -DatabasePassword=seabass " +
	"-DatabaseName=dune -RMQGameHostname=mq-game.svc -RMQGamePort=5672 " +
	"-RMQAdminHostname=mq-admin.svc -RMQAdminPort=5673 -RMQGameTlsEnabled=true " +
	"-battlegroup-director-url=http://bgd.svc:11717"

func TestAssembleServerDiscovery_Amp(t *testing.T) {
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "pgrep") {
			return gameProcLine, nil
		}
		return "", nil
	}}
	got := assembleServerDiscovery(exec, "amp")

	if got["db_user"] != "dune" || got["db_pass"] != "seabass" || got["db_name"] != "dune" {
		t.Errorf("amp DB discovery wrong: %+v", got)
	}
	// AMP is not kubectl — broker/namespace stay empty (no cluster pod IPs).
	if got["broker_game_addr"] != "" {
		t.Errorf("amp should not resolve broker addr from pod IPs, got %v", got["broker_game_addr"])
	}
}

func TestAssembleServerDiscovery_Kubectl(t *testing.T) {
	// fetchClusterPodIPs returns "name ip" lines (namespace is not included).
	podList := "mq-game-0 10.0.0.1\nmq-admin-0 10.0.0.2\nbgd-0 10.0.0.3\n"
	dbPod := "ns1 game-db-dbdepl-sts-0 10.0.0.9\n"
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "pgrep"):
			return gameProcLine, nil
		case strings.Contains(cmd, "db-dbdepl-sts"):
			return dbPod, nil
		case strings.Contains(cmd, "get pods"):
			return podList, nil
		}
		return "", nil
	}}
	got := assembleServerDiscovery(exec, "kubectl")

	if got["db_user"] != "dune" {
		t.Errorf("db_user = %v, want dune", got["db_user"])
	}
	if got["control_namespace"] != "ns1" {
		t.Errorf("control_namespace = %v, want ns1", got["control_namespace"])
	}
	if got["broker_game_addr"] != "10.0.0.1:5672" {
		t.Errorf("broker_game_addr = %v, want 10.0.0.1:5672", got["broker_game_addr"])
	}
	if got["broker_admin_addr"] != "10.0.0.2:5673" {
		t.Errorf("broker_admin_addr = %v, want 10.0.0.2:5673", got["broker_admin_addr"])
	}
	if got["broker_tls"] != true {
		t.Errorf("broker_tls = %v, want true", got["broker_tls"])
	}
	if got["director_url"] != "http://10.0.0.3:11717" {
		t.Errorf("director_url = %v, want http://10.0.0.3:11717", got["director_url"])
	}
}

// No running game server → discovery returns an empty (non-nil) map, never errors.
func TestAssembleServerDiscovery_NoGameProc(t *testing.T) {
	exec := &fnExecutor{fn: func(string) (string, error) { return "", nil }}
	got := assembleServerDiscovery(exec, "local")
	if got == nil {
		t.Fatal("discovery map must not be nil")
	}
	if got["db_user"] != "" {
		t.Errorf("expected empty db_user with no game proc, got %v", got["db_user"])
	}
}

// db_pass must be redacted in logs but returned in the payload (the wizard
// submits it back when creating the server).
func TestDiscoveryMaskForLog(t *testing.T) {
	if discoveryMaskForLog(map[string]any{"db_pass": "secret"}) == "secret" {
		t.Error("db_pass must not appear verbatim in the log summary")
	}
}
