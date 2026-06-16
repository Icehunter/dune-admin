package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// useTestServerStores points the global DB-backed stores at db for the duration
// of the test and restores the originals on cleanup. The DB is the source of
// truth for servers + global settings, so handler tests need it wired up.
func useTestServerStores(t *testing.T, db *sql.DB) {
	t.Helper()
	origStore, origServers, origSettings := globalStore, globalServersStore, globalSettingsStore
	globalStore = db
	globalServersStore = newServersStore(db)
	globalSettingsStore = newSettingsStore(db)
	t.Cleanup(func() {
		globalStore = origStore
		globalServersStore = origServers
		globalSettingsStore = origSettings
	})
}

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
	db := openSharedScopeDB(t)
	useTestServerStores(t, db)
	id1, _ := globalServersStore.insertServer(ServerConfig{Name: "One"}, 0)
	id2, _ := globalServersStore.insertServer(ServerConfig{Name: "Two"}, 1)
	s1, s2 := serverScope(id1), serverScope(id2)

	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: s1, Name: "One"})
	reg.Register(&ServerContext{ID: s2, Name: "Two"})
	_ = reg.SetActive(s1)
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	origCfg := loadedConfig
	loadedConfig = appConfig{Servers: []ServerConfig{{ID: id1}, {ID: id2}}}
	defer func() { loadedConfig = origCfg }()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/servers/"+s2, nil)
	req.SetPathValue("id", s2)
	rr := httptest.NewRecorder()
	handleDeleteServer(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if reg.Get(s2) != nil {
		t.Error("server still in registry after delete")
	}
	if _, ok, _ := globalServersStore.getServer(id2); ok {
		t.Error("server row still in DB after delete")
	}
}

// Deleting the active server is now allowed: the registry reassigns active to
// the next server and the global aliases follow it.
func TestHandleDeleteServer_ActiveAllowedReassigns(t *testing.T) {
	db := openSharedScopeDB(t)
	useTestServerStores(t, db)
	id1, _ := globalServersStore.insertServer(ServerConfig{Name: "One"}, 0)
	id2, _ := globalServersStore.insertServer(ServerConfig{Name: "Two"}, 1)
	s1, s2 := serverScope(id1), serverScope(id2)

	ctrl := &stubControlPlane{}
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: s1, Name: "One", Control: ctrl})
	reg.Register(&ServerContext{ID: s2, Name: "Two"})
	_ = reg.SetActive(s2)
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	origCfg := loadedConfig
	origCtrl := globalControl
	loadedConfig = appConfig{Servers: []ServerConfig{{ID: id1}, {ID: id2}}}
	defer func() { loadedConfig = origCfg; globalControl = origCtrl }()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/servers/"+s2, nil)
	req.SetPathValue("id", s2)
	rr := httptest.NewRecorder()
	handleDeleteServer(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if reg.ActiveID() != s1 {
		t.Errorf("active = %q after deleting active, want %q", reg.ActiveID(), s1)
	}
	if globalControl != ctrl {
		t.Error("globalControl not reassigned to the new active server")
	}
}

// Deleting the last remaining server empties the store so needsSetup() flips
// true and the SPA returns to the wizard. Global settings are preserved.
func TestHandleDeleteServer_LastResetsToSetup(t *testing.T) {
	db := openMemUnifiedStore(t)
	useTestServerStores(t, db)
	id1, _ := globalServersStore.insertServer(ServerConfig{Name: "Default"}, 0)
	s1 := serverScope(id1)

	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: s1, Name: "Default"})
	_ = reg.SetActive(s1)
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	origCfg := loadedConfig
	loadedConfig = appConfig{ListenAddr: ":9999"}
	defer func() { loadedConfig = origCfg }()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/servers/"+s1, nil)
	req.SetPathValue("id", s1)
	rr := httptest.NewRecorder()
	handleDeleteServer(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if reg.ActiveID() != "" {
		t.Errorf("registry not empty after deleting last server (active = %q)", reg.ActiveID())
	}
	if has, _ := globalServersStore.hasAnyServer(); has {
		t.Error("store should have no servers after deleting the last one")
	}
	if !needsSetup() {
		t.Error("needsSetup() should be true after deleting the last server")
	}
	if loadedConfig.ListenAddr != ":9999" {
		t.Errorf("global ListenAddr = %q, want :9999 preserved", loadedConfig.ListenAddr)
	}
}

func TestHandleDeleteServer_NotFound(t *testing.T) {
	db := openSharedScopeDB(t)
	useTestServerStores(t, db)
	id1, _ := globalServersStore.insertServer(ServerConfig{Name: "One"}, 0)
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: serverScope(id1), Name: "One"})
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/servers/999", nil)
	req.SetPathValue("id", "999")
	rr := httptest.NewRecorder()
	handleDeleteServer(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleDeleteServer_BadID(t *testing.T) {
	reg := newServerRegistry(nil)
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/servers/ghost", nil)
	req.SetPathValue("id", "ghost")
	rr := httptest.NewRecorder()
	handleDeleteServer(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for non-numeric id", rr.Code)
	}
}

// ── POST /api/v1/servers ─────────────────────────────────────────────────────

func TestHandleAddServer_Success(t *testing.T) {
	db := openSharedScopeDB(t)
	useTestServerStores(t, db)
	reg := newServerRegistry(nil)
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	origCfg := loadedConfig
	loadedConfig = appConfig{}
	defer func() { loadedConfig = origCfg }()

	body, _ := json.Marshal(map[string]any{"name": "Two", "control": "local"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleAddServer(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	// The DB assigns the id; it comes back as a real JSON number.
	var out serverListItem
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ID == 0 {
		t.Errorf("expected a DB-assigned id, got %d", out.ID)
	}
	if reg.Get(serverScope(out.ID)) == nil {
		t.Errorf("server %d not in registry after add", out.ID)
	}
	if _, ok, _ := globalServersStore.getServer(out.ID); !ok {
		t.Errorf("server %d not persisted to DB after add", out.ID)
	}
}

// The client-supplied id is ignored — the DB assigns the authoritative id.
func TestHandleAddServer_IgnoresClientID(t *testing.T) {
	db := openSharedScopeDB(t)
	useTestServerStores(t, db)
	reg := newServerRegistry(nil)
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()
	origCfg := loadedConfig
	loadedConfig = appConfig{}
	defer func() { loadedConfig = origCfg }()

	body, _ := json.Marshal(map[string]any{"id": 999, "name": "Bogus", "control": "local"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleAddServer(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	var out serverListItem
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	if out.ID == 999 {
		t.Error("client-supplied id should be ignored; DB assigns the id")
	}
}

func TestHandleAddServer_StoreUnavailable(t *testing.T) {
	origStore := globalServersStore
	globalServersStore = nil
	defer func() { globalServersStore = origStore }()

	body, _ := json.Marshal(map[string]any{"name": "No Store"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleAddServer(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when store is unavailable", rr.Code)
	}
}

// Note: per-server purge is now covered by FK ON DELETE CASCADE; see
// TestServerDeleteCascade in store_server_scope_test.go. The former
// cmdPurgeServerData tests were removed with that helper.
