package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── serverRegistry.Remove ─────────────────────────────────────────────────────

func TestServerRegistry_Remove_Existing(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "s1", Name: "One"})
	reg.Register(&ServerContext{ID: "s2", Name: "Two"})
	_ = reg.SetActive("s1")

	if !reg.Remove("s2") {
		t.Fatal("Remove returned false for existing server")
	}
	if reg.Get("s2") != nil {
		t.Error("s2 still present after remove")
	}
	if len(reg.All()) != 1 {
		t.Errorf("All() len = %d, want 1", len(reg.All()))
	}
}

func TestServerRegistry_Remove_NotFound(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "s1", Name: "One"})

	if reg.Remove("ghost") {
		t.Error("Remove returned true for non-existent server")
	}
}

func TestServerRegistry_Remove_ActiveReassigned(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "s1", Name: "One"})
	reg.Register(&ServerContext{ID: "s2", Name: "Two"})
	_ = reg.SetActive("s1")

	// Remove the active server → active should shift to s2
	if !reg.Remove("s1") {
		t.Fatal("Remove returned false")
	}
	if reg.ActiveID() != "s2" {
		t.Errorf("ActiveID = %q after removing active; want s2", reg.ActiveID())
	}
}

func TestServerRegistry_Remove_Last(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "s1", Name: "One"})

	if !reg.Remove("s1") {
		t.Fatal("Remove returned false")
	}
	if len(reg.All()) != 0 {
		t.Error("registry not empty after removing last server")
	}
	if reg.ActiveID() != "" {
		t.Errorf("ActiveID = %q after removing last; want empty", reg.ActiveID())
	}
}

// ── DELETE /api/v1/servers/{id} ───────────────────────────────────────────────

func TestHandleDeleteServer_Success(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "s1", Name: "One"})
	reg.Register(&ServerContext{ID: "s2", Name: "Two"})
	_ = reg.SetActive("s1")
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	origCfg := loadedConfig
	loadedConfig = appConfig{
		Servers: []ServerConfig{
			{ID: "s1"}, {ID: "s2"},
		},
	}
	defer func() { loadedConfig = origCfg }()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/servers/s2", nil)
	req.SetPathValue("id", "s2")
	rr := httptest.NewRecorder()
	handleDeleteServer(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if reg.Get("s2") != nil {
		t.Error("s2 still in registry after delete")
	}
}

// Deleting the active server is now allowed: the registry reassigns active to
// the next server and the global aliases follow it.
func TestHandleDeleteServer_ActiveAllowedReassigns(t *testing.T) {
	t.Setenv("DUNE_ADMIN_CONFIG_DIR", t.TempDir())
	ctrl := &stubControlPlane{}
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "s1", Name: "One", Control: ctrl})
	reg.Register(&ServerContext{ID: "s2", Name: "Two"})
	_ = reg.SetActive("s2")
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	origCfg := loadedConfig
	origCtrl := globalControl
	loadedConfig = appConfig{Servers: []ServerConfig{{ID: "s1"}, {ID: "s2"}}}
	defer func() { loadedConfig = origCfg; globalControl = origCtrl }()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/servers/s2", nil)
	req.SetPathValue("id", "s2")
	rr := httptest.NewRecorder()
	handleDeleteServer(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if reg.ActiveID() != "s1" {
		t.Errorf("active = %q after deleting active s2, want s1", reg.ActiveID())
	}
	if globalControl != ctrl {
		t.Error("globalControl not reassigned to the new active server")
	}
}

// Deleting the last remaining (legacy "default") server clears the flat
// connection config so needsSetup() flips true → SPA returns to the wizard.
func TestHandleDeleteServer_LastResetsToSetup(t *testing.T) {
	t.Setenv("DUNE_ADMIN_CONFIG_DIR", t.TempDir())
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "default", Name: "Default"})
	_ = reg.SetActive("default")
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	origCfg := loadedConfig
	origDBPass := dbPass
	origAutoDiscover := autoDiscover
	dbPass = "secret"
	loadedConfig = appConfig{DBPass: "secret", DBHost: "127.0.0.1", ListenAddr: ":9999"}
	defer func() {
		loadedConfig = origCfg
		dbPass = origDBPass
		autoDiscover = origAutoDiscover
	}()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/servers/default", nil)
	req.SetPathValue("id", "default")
	rr := httptest.NewRecorder()
	handleDeleteServer(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if reg.ActiveID() != "" {
		t.Errorf("registry not empty after deleting last server (active = %q)", reg.ActiveID())
	}
	if dbPass != "" {
		t.Errorf("dbPass = %q after deleting last server, want empty", dbPass)
	}
	if !needsSetup() {
		t.Error("needsSetup() should be true after deleting the last server")
	}
	if loadedConfig.ListenAddr != ":9999" {
		t.Errorf("global ListenAddr = %q, want :9999 preserved", loadedConfig.ListenAddr)
	}
}

func TestHandleDeleteServer_NotFound(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "s1", Name: "One"})
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/servers/ghost", nil)
	req.SetPathValue("id", "ghost")
	rr := httptest.NewRecorder()
	handleDeleteServer(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// ── POST /api/v1/servers ─────────────────────────────────────────────────────

func TestHandleAddServer_Success(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "s1", Name: "One"})
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	origCfg := loadedConfig
	loadedConfig = appConfig{Servers: []ServerConfig{{ID: "s1"}}}
	defer func() { loadedConfig = origCfg }()

	body, _ := json.Marshal(map[string]string{"id": "s2", "name": "Two", "control": "local"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleAddServer(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if reg.Get("s2") == nil {
		t.Error("s2 not in registry after add")
	}
}

func TestHandleAddServer_MissingID(t *testing.T) {
	reg := newServerRegistry(nil)
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	body, _ := json.Marshal(map[string]string{"name": "No ID"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleAddServer(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleAddServer_DuplicateID(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "s1", Name: "One"})
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	body, _ := json.Marshal(map[string]string{"id": "s1", "name": "Duplicate"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleAddServer(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
}

// ── cmdPurgeServerData ────────────────────────────────────────────────────────

func TestCmdPurgeServerData_ClearsAllScopedTables(t *testing.T) {
	db := openSharedScopeDB(t)

	sA := newWelcomeStore(db, "srv-a")
	if err := sA.insertGranted("FLS1", "v1", 1, "Paul"); err != nil {
		t.Fatalf("insertGranted: %v", err)
	}

	epA := newEventStore(db, "srv-a")
	if err := epA.recordGranted(1, 1, 42); err != nil {
		t.Fatalf("recordGranted: %v", err)
	}

	if err := cmdPurgeServerData(context.Background(), db, "srv-a"); err != nil {
		t.Fatalf("purge: %v", err)
	}

	ex, err := sA.grantExists("FLS1", "v1", 1)
	if err != nil {
		t.Fatalf("grantExists after purge: %v", err)
	}
	if ex {
		t.Error("welcome_grants should be purged for srv-a")
	}

	ex, err = epA.claimExists(1, 1, 42)
	if err != nil {
		t.Fatalf("claimExists after purge: %v", err)
	}
	if ex {
		t.Error("event_award_claims should be purged for srv-a")
	}
}

func TestCmdPurgeServerData_DoesNotAffectOtherServer(t *testing.T) {
	db := openSharedScopeDB(t)

	sA := newWelcomeStore(db, "srv-a")
	sB := newWelcomeStore(db, "srv-b")

	if err := sA.insertGranted("FLS1", "v1", 1, "Paul"); err != nil {
		t.Fatalf("sA.insertGranted: %v", err)
	}
	if err := sB.insertGranted("FLS2", "v1", 2, "Chani"); err != nil {
		t.Fatalf("sB.insertGranted: %v", err)
	}

	if err := cmdPurgeServerData(context.Background(), db, "srv-a"); err != nil {
		t.Fatalf("purge srv-a: %v", err)
	}

	// srv-b data must be untouched
	ex, err := sB.grantExists("FLS2", "v1", 2)
	if err != nil {
		t.Fatalf("sB.grantExists: %v", err)
	}
	if !ex {
		t.Error("srv-b welcome_grants should NOT be purged when srv-a is deleted")
	}
}
