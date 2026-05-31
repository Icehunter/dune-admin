package main

import "testing"

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
