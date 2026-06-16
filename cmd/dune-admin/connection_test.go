package main

import (
	"context"
	"testing"
)

// TestConnectDB_NilExecutorReturnsError verifies that connectDB returns a clean
// error (not a nil-pointer panic) when globalExecutor has not been set.
func TestConnectDB_NilExecutorReturnsError(t *testing.T) {
	origExec := globalExecutor
	origPodIP := globalPodIP
	origDBPort := dbPort
	origDBUser, origDBPass, origDBName, origDBSchema := dbUser, dbPass, dbName, dbSchema
	t.Cleanup(func() {
		globalExecutor = origExec
		globalPodIP = origPodIP
		dbPort = origDBPort
		dbUser, dbPass, dbName, dbSchema = origDBUser, origDBPass, origDBName, origDBSchema
	})

	globalExecutor = nil
	globalPodIP = "127.0.0.1"
	dbPort = 1
	dbUser, dbPass, dbName, dbSchema = "t", "t", "t", "t"

	_, err := connectDB(context.Background(), "t", "t")
	if err == nil {
		t.Fatal("expected error when globalExecutor is nil, got nil")
	}
}

// TestConnectAll_ControlPlaneSurvivesDBFailure verifies that a DB connection
// failure leaves the control plane and executor established. The control plane
// (logs / battlegroup / server control) does not depend on the database, so a
// DB outage must not disable it — the DB can be re-established later via
// /api/v1/reconnect without losing control-plane functionality.
func TestConnectAll_ControlPlaneSurvivesDBFailure(t *testing.T) {
	// connectAll mutates package-level globals — must not run in parallel.
	origCP, origSSH := controlPlane, sshHost
	origDBHost, origDBPort := dbHost, dbPort
	origDBUser, origDBPass, origDBName, origDBSchema := dbUser, dbPass, dbName, dbSchema
	origCfg := loadedConfig
	origDB, origExec, origCtl := globalDB, globalExecutor, globalControl
	t.Cleanup(func() {
		controlPlane, sshHost = origCP, origSSH
		dbHost, dbPort = origDBHost, origDBPort
		dbUser, dbPass, dbName, dbSchema = origDBUser, origDBPass, origDBName, origDBSchema
		loadedConfig = origCfg
		globalDB, globalExecutor, globalControl = origDB, origExec, origCtl
	})

	controlPlane = "local" // local executor needs no network; control plane is pure
	sshHost = ""
	dbHost, dbPort = "127.0.0.1", 1 // nothing listens on :1 -> immediate connection refused
	dbUser, dbPass, dbName, dbSchema = "t", "t", "t", "t"
	loadedConfig = appConfig{}
	globalDB, globalExecutor, globalControl = nil, nil, nil

	err := connectAll()

	if err == nil {
		t.Fatal("expected connectAll to report the DB failure (closed port)")
	}
	if globalControl == nil {
		t.Error("control plane must be established despite DB failure (it does not depend on the DB)")
	}
	if globalExecutor == nil {
		t.Error("executor must remain established despite DB failure")
	}
	if globalDB != nil {
		t.Error("globalDB must be nil when the DB connect failed")
	}
}

func TestResolveDBPort(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 15432},
		{5432, 5432},
		{15432, 15432},
		{1234, 1234},
	}
	for _, tt := range tests {
		got := resolveDBPort(tt.input)
		if got != tt.want {
			t.Errorf("resolveDBPort(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestBuildCurrentConfig_MasksOnlyNonEmptyPassword(t *testing.T) {
	origPass := dbPass
	t.Cleanup(func() { dbPass = origPass })

	dbPass = ""
	cfg := buildCurrentConfig()
	if cfg.DBPass != "" {
		t.Errorf("buildCurrentConfig with empty dbPass: DBPass = %q, want %q", cfg.DBPass, "")
	}

	dbPass = "secret"
	cfg = buildCurrentConfig()
	if cfg.DBPass != masked {
		t.Errorf("buildCurrentConfig with non-empty dbPass: DBPass = %q, want %q", cfg.DBPass, masked)
	}
}

func TestResolveDBHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "127.0.0.1"},
		{"127.0.0.1", "127.0.0.1"},
		{"192.168.0.59", "192.168.0.59"},
		{"db.example.com", "db.example.com"},
	}
	for _, tt := range tests {
		t.Run("input_"+tt.input, func(t *testing.T) {
			got := resolveDBHost(tt.input)
			if got != tt.want {
				t.Errorf("resolveDBHost(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveControl(t *testing.T) {
	origControlPlane := controlPlane
	origSSHHost := sshHost
	t.Cleanup(func() {
		controlPlane = origControlPlane
		sshHost = origSSHHost
	})

	controlPlane = "amp"
	sshHost = ""
	if got := resolveControl(); got != "amp" {
		t.Fatalf("expected explicit control plane to win, got %q", got)
	}

	controlPlane = ""
	sshHost = "vm.example:22"
	if got := resolveControl(); got != "kubectl" {
		t.Fatalf("expected ssh host to default control to kubectl, got %q", got)
	}

	controlPlane = ""
	sshHost = ""
	if got := resolveControl(); got != "local" {
		t.Fatalf("expected local default without ssh/control flags, got %q", got)
	}
}
