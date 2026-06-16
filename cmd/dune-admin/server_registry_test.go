package main

import (
	"sync"
	"testing"
)

// TestServerRegistry_Get tests the Get method: known id returns the context,
// unknown id returns nil, empty registry returns nil.
func TestServerRegistry_Get(t *testing.T) {
	reg := newServerRegistry(nil)
	sc1 := &ServerContext{ID: "srv-a", Name: "Server A", StoreScope: 1}
	sc2 := &ServerContext{ID: "srv-b", Name: "Server B", StoreScope: 2}
	reg.Register(sc1)
	reg.Register(sc2)

	tests := []struct {
		name    string
		id      string
		wantNil bool
	}{
		{"known id srv-a", "srv-a", false},
		{"known id srv-b", "srv-b", false},
		{"unknown id", "srv-c", true},
		{"empty id", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reg.Get(tt.id)
			if tt.wantNil && got != nil {
				t.Errorf("Get(%q) = %v, want nil", tt.id, got)
			}
			if !tt.wantNil && got == nil {
				t.Errorf("Get(%q) = nil, want non-nil", tt.id)
			}
			if got != nil && got.ID != tt.id {
				t.Errorf("Get(%q).ID = %q, want %q", tt.id, got.ID, tt.id)
			}
		})
	}
}

// TestServerRegistry_Active tests that Active returns the active server and
// SetActive switches it correctly.
func TestServerRegistry_Active(t *testing.T) {
	reg := newServerRegistry(nil)
	sc1 := &ServerContext{ID: "alpha", Name: "Alpha", StoreScope: 1}
	sc2 := &ServerContext{ID: "beta", Name: "Beta", StoreScope: 2}

	t.Run("empty registry returns nil", func(t *testing.T) {
		if got := reg.Active(); got != nil {
			t.Errorf("Active() on empty registry = %v, want nil", got)
		}
	})

	t.Run("first registered becomes active", func(t *testing.T) {
		reg.Register(sc1)
		got := reg.Active()
		if got == nil {
			t.Fatal("Active() = nil after first Register, want non-nil")
		}
		if got.ID != "alpha" {
			t.Errorf("Active().ID = %q, want %q", got.ID, "alpha")
		}
	})

	t.Run("second register does not change active", func(t *testing.T) {
		reg.Register(sc2)
		got := reg.Active()
		if got.ID != "alpha" {
			t.Errorf("Active().ID = %q after second Register, want %q", got.ID, "alpha")
		}
	})

	t.Run("SetActive switches active server", func(t *testing.T) {
		if err := reg.SetActive("beta"); err != nil {
			t.Fatalf("SetActive(%q) = %v, want nil", "beta", err)
		}
		got := reg.Active()
		if got == nil || got.ID != "beta" {
			t.Errorf("Active().ID = %v, want %q", got, "beta")
		}
	})

	t.Run("SetActive unknown id returns error", func(t *testing.T) {
		if err := reg.SetActive("nonexistent"); err == nil {
			t.Error("SetActive with unknown id: want error, got nil")
		}
		// active should remain beta
		if got := reg.Active(); got == nil || got.ID != "beta" {
			t.Errorf("Active().ID after failed SetActive = %v, want %q", got, "beta")
		}
	})

	t.Run("ActiveID returns current active id", func(t *testing.T) {
		if id := reg.ActiveID(); id != "beta" {
			t.Errorf("ActiveID() = %q, want %q", id, "beta")
		}
	})
}

// TestServerRegistry_All tests that All returns all contexts in registration order.
func TestServerRegistry_All(t *testing.T) {
	reg := newServerRegistry(nil)

	t.Run("empty registry returns empty slice", func(t *testing.T) {
		all := reg.All()
		if len(all) != 0 {
			t.Errorf("All() len = %d, want 0", len(all))
		}
	})

	ids := []string{"one", "two", "three"}
	for i, id := range ids {
		reg.Register(&ServerContext{ID: id, StoreScope: i + 1})
	}

	t.Run("returns all in registration order", func(t *testing.T) {
		all := reg.All()
		if len(all) != len(ids) {
			t.Fatalf("All() len = %d, want %d", len(all), len(ids))
		}
		for i, sc := range all {
			if sc.ID != ids[i] {
				t.Errorf("All()[%d].ID = %q, want %q", i, sc.ID, ids[i])
			}
		}
	})

	t.Run("re-registering same id does not duplicate", func(t *testing.T) {
		reg.Register(&ServerContext{ID: "one", Name: "One Updated", StoreScope: 1})
		all := reg.All()
		if len(all) != len(ids) {
			t.Errorf("All() len = %d after re-register, want %d", len(all), len(ids))
		}
		// Should carry the updated name
		got := reg.Get("one")
		if got.Name != "One Updated" {
			t.Errorf("Get(%q).Name = %q after re-register, want %q", "one", got.Name, "One Updated")
		}
	})
}

// ── connectAll registry tests ─────────────────────────────────────────────────

// TestConnectAll_PopulatesRegistry verifies that after connectAll, the
// globalRegistry contains exactly one server with id="default" and that the
// active server's control plane is set (even when the DB fails to connect).
func TestConnectAll_PopulatesRegistry(t *testing.T) {
	origCP, origSSH := controlPlane, sshHost
	origDBHost, origDBPort := dbHost, dbPort
	origDBUser, origDBPass, origDBName, origDBSchema := dbUser, dbPass, dbName, dbSchema
	origCfg := loadedConfig
	origDB, origExec, origCtl := globalDB, globalExecutor, globalControl
	origReg := globalRegistry
	t.Cleanup(func() {
		controlPlane, sshHost = origCP, origSSH
		dbHost, dbPort = origDBHost, origDBPort
		dbUser, dbPass, dbName, dbSchema = origDBUser, origDBPass, origDBName, origDBSchema
		loadedConfig = origCfg
		globalDB, globalExecutor, globalControl = origDB, origExec, origCtl
		globalRegistry = origReg
	})

	controlPlane = "local"
	sshHost = ""
	dbHost, dbPort = "127.0.0.1", 1 // unreachable
	dbUser, dbPass, dbName, dbSchema = "t", "t", "t", "t"
	loadedConfig = appConfig{}
	globalDB, globalExecutor, globalControl = nil, nil, nil
	globalRegistry = newServerRegistry(nil)

	_ = connectAll() // error expected (bad DB) but not fatal

	all := globalRegistry.All()
	if len(all) != 1 {
		t.Fatalf("globalRegistry has %d servers after connectAll, want 1", len(all))
	}
	if all[0].ID != "default" {
		t.Errorf("registry server ID = %q, want %q", all[0].ID, "default")
	}
	if globalRegistry.Active() == nil {
		t.Error("globalRegistry.Active() = nil, want non-nil")
	}
	if globalRegistry.Active().Control == nil {
		t.Error("Active().Control must be set despite DB failure")
	}
	if globalRegistry.ActiveID() != "default" {
		t.Errorf("ActiveID() = %q, want %q", globalRegistry.ActiveID(), "default")
	}
}

// ── legacyServerFromFlat tests ────────────────────────────────────────────────

// TestLegacyServerFromFlat_SynthesisesDefaultServer verifies that when flag-
// globals are set (as they are after init/flag.Parse), legacyServerFromFlat
// builds a ServerConfig with id="default" and the correct field values.
func TestLegacyServerFromFlat_SynthesisesDefaultServer(t *testing.T) {
	// Save and restore all flag-globals that legacyServerFromFlat reads.
	orig := struct {
		sshHost, sshUser, sshMode, sshExtraOpts  string
		autoDiscover                             bool
		dbHost, dbUser, dbPass, dbName, dbSchema string
		dbPort                                   int
		controlPlane, controlNS                  string
		brokerGame, brokerAdmin                  string
		brokerTLS                                bool
		backupDir, serverIniDir                  string
	}{
		sshHost: sshHost, sshUser: sshUser, sshMode: sshMode, sshExtraOpts: sshExtraOpts,
		autoDiscover: autoDiscover,
		dbHost:       dbHost, dbUser: dbUser, dbPass: dbPass, dbName: dbName, dbSchema: dbSchema,
		dbPort: dbPort, controlPlane: controlPlane, controlNS: controlNS,
		brokerGame: brokerGameAddr, brokerAdmin: brokerAdminAddr, brokerTLS: brokerTLS,
		backupDir: backupDir, serverIniDir: serverIniDir,
	}
	t.Cleanup(func() {
		sshHost = orig.sshHost
		sshUser = orig.sshUser
		sshMode = orig.sshMode
		sshExtraOpts = orig.sshExtraOpts
		autoDiscover = orig.autoDiscover
		dbHost = orig.dbHost
		dbUser = orig.dbUser
		dbPass = orig.dbPass
		dbName = orig.dbName
		dbSchema = orig.dbSchema
		dbPort = orig.dbPort
		controlPlane = orig.controlPlane
		controlNS = orig.controlNS
		brokerGameAddr = orig.brokerGame
		brokerAdminAddr = orig.brokerAdmin
		brokerTLS = orig.brokerTLS
		backupDir = orig.backupDir
		serverIniDir = orig.serverIniDir
	})

	// Set up synthetic flag-globals to assert they're picked up.
	sshHost = ""
	sshUser = ""
	sshMode = ""
	sshExtraOpts = ""
	autoDiscover = false
	dbHost = "db.example"
	dbPort = 5432
	dbUser = "gameuser"
	dbPass = "secret"
	dbName = "game"
	dbSchema = "dune"
	controlPlane = "local"
	controlNS = ""
	brokerGameAddr = "rmq:5672"
	brokerAdminAddr = "rmq:15672"
	brokerTLS = false
	backupDir = "/backups"
	serverIniDir = "/ini"

	ac := appConfig{
		DockerGameserver: "game-container",
		AmpInstance:      "DuneAwakening01",
		CmdStart:         "start.sh",
	}

	sc := legacyServerFromFlat(ac)

	if sc.LegacyID != "default" {
		t.Errorf("LegacyID = %q, want %q", sc.LegacyID, "default")
	}
	if sc.Name != "Default" {
		t.Errorf("Name = %q, want %q", sc.Name, "Default")
	}
	if sc.DBHost != "db.example" {
		t.Errorf("DBHost = %q, want %q", sc.DBHost, "db.example")
	}
	if sc.DBPort != 5432 {
		t.Errorf("DBPort = %d, want 5432", sc.DBPort)
	}
	if sc.DBUser != "gameuser" {
		t.Errorf("DBUser = %q, want %q", sc.DBUser, "gameuser")
	}
	if sc.DBSchema != "dune" {
		t.Errorf("DBSchema = %q, want %q", sc.DBSchema, "dune")
	}
	if sc.Control != "local" {
		t.Errorf("Control = %q, want %q", sc.Control, "local")
	}
	if sc.BackupDir != "/backups" {
		t.Errorf("BackupDir = %q, want %q", sc.BackupDir, "/backups")
	}
	if sc.BrokerGameAddr != "rmq:5672" {
		t.Errorf("BrokerGameAddr = %q, want %q", sc.BrokerGameAddr, "rmq:5672")
	}
	// Provider-specific fields with no flag override come from appConfig.
	if sc.DockerGameserver != "game-container" {
		t.Errorf("DockerGameserver = %q, want %q", sc.DockerGameserver, "game-container")
	}
	if sc.AmpInstance != "DuneAwakening01" {
		t.Errorf("AmpInstance = %q, want %q", sc.AmpInstance, "DuneAwakening01")
	}
	if sc.CmdStart != "start.sh" {
		t.Errorf("CmdStart = %q, want %q", sc.CmdStart, "start.sh")
	}
}

// TestLegacyServerFromFlat_SSHTriggersKubectl verifies that a non-empty sshHost
// causes the resolved control to be "kubectl" when controlPlane is unset.
func TestLegacyServerFromFlat_SSHTriggersKubectl(t *testing.T) {
	origSSH, origCtrl := sshHost, controlPlane
	t.Cleanup(func() { sshHost = origSSH; controlPlane = origCtrl })

	sshHost = "vm.example:22"
	controlPlane = ""

	sc := legacyServerFromFlat(appConfig{})
	if sc.Control != "kubectl" {
		t.Errorf("Control = %q, want %q (SSH host should imply kubectl)", sc.Control, "kubectl")
	}
}

// ── connectServer tests ────────────────────────────────────────────────────────

// TestConnectServer_ControlPlaneSurvivesDBFailure mirrors the existing
// TestConnectAll_ControlPlaneSurvivesDBFailure but for the per-server API.
// The DB fails to connect (nothing on :1), but the control plane and executor
// must still be returned in the ServerContext so that control operations work
// without a DB.
func TestConnectServer_ControlPlaneSurvivesDBFailure(t *testing.T) {
	cfg := ServerConfig{
		ID:       7,
		Name:     "Test Server",
		Control:  "local",
		DBHost:   "127.0.0.1",
		DBPort:   1, // nothing listens here → immediate connection refused
		DBUser:   "t",
		DBPass:   "t",
		DBName:   "t",
		DBSchema: "t",
	}

	sc, err := connectServer(cfg)

	if err == nil {
		t.Fatal("expected error on unreachable DB port, got nil")
	}
	if sc == nil {
		t.Fatal("expected non-nil ServerContext even on DB failure")
	}
	if sc.Control == nil {
		t.Error("control plane must be established despite DB failure")
	}
	if sc.Executor == nil {
		t.Error("executor must remain set despite DB failure")
	}
	if sc.DB != nil {
		t.Error("DB must be nil when DB connect failed")
	}
	if sc.ID != serverScope(cfg.ID) {
		t.Errorf("ServerContext.ID = %q, want %q", sc.ID, serverScope(cfg.ID))
	}
	if sc.StoreScope != storeScopeForID(cfg.ID) {
		t.Errorf("StoreScope = %d, want %d", sc.StoreScope, storeScopeForID(cfg.ID))
	}
}

// TestConnectServer_IDAndNamePropagated ensures the ServerContext reflects the
// input ServerConfig's ID and Name correctly.
func TestConnectServer_IDAndNamePropagated(t *testing.T) {
	cfg := ServerConfig{
		ID:      42,
		Name:    "My Server",
		Control: "local",
		DBHost:  "127.0.0.1",
		DBPort:  1,
		DBUser:  "u", DBPass: "p", DBName: "d", DBSchema: "s",
	}

	sc, _ := connectServer(cfg)

	if sc.ID != "42" {
		t.Errorf("ID = %q, want %q", sc.ID, "42")
	}
	if sc.Name != "My Server" {
		t.Errorf("Name = %q, want %q", sc.Name, "My Server")
	}
	if sc.StoreScope != 42 {
		t.Errorf("StoreScope = %d, want 42", sc.StoreScope)
	}
}

// TestConnectServer_ConcurrentSafety exercises the registry under concurrent
// reads and writes to catch races with -race.
func TestServerRegistry_ConcurrentSafety(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "init", StoreScope: 1})

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "srv-" + string(rune('a'+n%5))
			reg.Register(&ServerContext{ID: id, StoreScope: n%5 + 1})
			_ = reg.Get(id)
			_ = reg.Active()
			_ = reg.All()
		}(i)
	}
	wg.Wait()
}
