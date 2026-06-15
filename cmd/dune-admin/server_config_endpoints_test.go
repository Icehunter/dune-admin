package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── GET /api/v1/servers/{id}/config ──────────────────────────────────────────

func TestHandleGetServerConfig_NotFound(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "1", Name: "One"})
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/999/config", nil)
	req.SetPathValue("id", "999")
	rr := httptest.NewRecorder()
	handleGetServerConfig(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleGetServerConfig_BadID(t *testing.T) {
	reg := newServerRegistry(nil)
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/ghost/config", nil)
	req.SetPathValue("id", "ghost")
	rr := httptest.NewRecorder()
	handleGetServerConfig(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for non-numeric id", rr.Code)
	}
}

func TestHandleGetServerConfig_MasksSecrets(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{
		ID:   "1",
		Name: "One",
		Cfg: ServerConfig{
			ID: 1, Name: "One",
			DBHost: "10.0.0.1", DBUser: "dune", DBPass: "topsecret",
			BrokerPass: "brokerpw", BrokerJWTSecret: "jwt", AmpAPIPass: "amppw",
		},
	})
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/1/config", nil)
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()
	handleGetServerConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var got ServerConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DBHost != "10.0.0.1" {
		t.Errorf("DBHost = %q, want 10.0.0.1 (non-secret preserved)", got.DBHost)
	}
	for name, v := range map[string]string{
		"DBPass":          got.DBPass,
		"BrokerPass":      got.BrokerPass,
		"BrokerJWTSecret": got.BrokerJWTSecret,
		"AmpAPIPass":      got.AmpAPIPass,
	} {
		if v != masked {
			t.Errorf("%s = %q, want masked", name, v)
		}
	}
}

// ── PUT /api/v1/servers/{id}/config ──────────────────────────────────────────

func TestHandleUpdateServerConfig_NotFound(t *testing.T) {
	reg := newServerRegistry(nil)
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	body, _ := json.Marshal(ServerConfig{Name: "ghost"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/servers/999/config", bytes.NewReader(body))
	req.SetPathValue("id", "999")
	rr := httptest.NewRecorder()
	handleUpdateServerConfig(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleUpdateServerConfig_PreservesMaskedSecretAndPersists(t *testing.T) {
	db := openSharedScopeDB(t)
	useTestServerStores(t, db)
	id1, _ := globalServersStore.insertServer(
		ServerConfig{Name: "One", Control: "local", DBPass: "oldsecret", DBName: "dune"}, 0)
	s1 := serverScope(id1)

	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{
		ID:   s1,
		Name: "One",
		Cfg:  ServerConfig{ID: id1, Name: "One", Control: "local", DBPass: "oldsecret", DBName: "dune"},
	})
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	origCfg := loadedConfig
	loadedConfig = appConfig{Servers: []ServerConfig{
		{ID: id1, Name: "One", Control: "local", DBPass: "oldsecret", DBName: "dune"},
	}}
	defer func() { loadedConfig = origCfg }()

	// Client sends masked password (unchanged) but a new DB name.
	body, _ := json.Marshal(ServerConfig{Name: "One", Control: "local", DBPass: masked, DBName: "newdb"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/servers/"+s1+"/config", bytes.NewReader(body))
	req.SetPathValue("id", s1)
	rr := httptest.NewRecorder()
	handleUpdateServerConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	// The DB row (source of truth) must hold the restored secret and new name.
	stored, ok, err := globalServersStore.getServer(id1)
	if err != nil || !ok {
		t.Fatalf("getServer: ok=%v err=%v", ok, err)
	}
	if stored.DBPass != "oldsecret" {
		t.Errorf("DBPass = %q, want oldsecret (masked restored)", stored.DBPass)
	}
	if stored.DBName != "newdb" {
		t.Errorf("DBName = %q, want newdb", stored.DBName)
	}

	// The in-memory mirror must match.
	var entry *ServerConfig
	for i := range loadedConfig.Servers {
		if loadedConfig.Servers[i].ID == id1 {
			entry = &loadedConfig.Servers[i]
		}
	}
	if entry == nil {
		t.Fatal("server missing from in-memory Servers mirror")
	}
	if entry.DBPass != "oldsecret" || entry.DBName != "newdb" {
		t.Errorf("mirror entry = %+v, want DBPass=oldsecret DBName=newdb", *entry)
	}
}
