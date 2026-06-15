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
	reg.Register(&ServerContext{ID: "s1", Name: "One"})
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/ghost/config", nil)
	req.SetPathValue("id", "ghost")
	rr := httptest.NewRecorder()
	handleGetServerConfig(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleGetServerConfig_MasksSecrets(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{
		ID:   "s1",
		Name: "One",
		Cfg: ServerConfig{
			ID: "s1", Name: "One",
			DBHost: "10.0.0.1", DBUser: "dune", DBPass: "topsecret",
			BrokerPass: "brokerpw", BrokerJWTSecret: "jwt", AmpAPIPass: "amppw",
		},
	})
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/s1/config", nil)
	req.SetPathValue("id", "s1")
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

	body, _ := json.Marshal(ServerConfig{ID: "ghost"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/servers/ghost/config", bytes.NewReader(body))
	req.SetPathValue("id", "ghost")
	rr := httptest.NewRecorder()
	handleUpdateServerConfig(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleUpdateServerConfig_PreservesMaskedSecretAndPersists(t *testing.T) {
	t.Setenv("DUNE_ADMIN_CONFIG_DIR", t.TempDir())

	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{
		ID:   "s1",
		Name: "One",
		Cfg:  ServerConfig{ID: "s1", Name: "One", Control: "local", DBPass: "oldsecret", DBName: "dune"},
	})
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	origCfg := loadedConfig
	loadedConfig = appConfig{Servers: []ServerConfig{
		{ID: "s1", Name: "One", Control: "local", DBPass: "oldsecret", DBName: "dune"},
	}}
	defer func() { loadedConfig = origCfg }()

	// Client sends masked password (unchanged) but a new DB name.
	body, _ := json.Marshal(ServerConfig{ID: "s1", Name: "One", Control: "local", DBPass: masked, DBName: "newdb"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/servers/s1/config", bytes.NewReader(body))
	req.SetPathValue("id", "s1")
	rr := httptest.NewRecorder()
	handleUpdateServerConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	var entry *ServerConfig
	for i := range loadedConfig.Servers {
		if loadedConfig.Servers[i].ID == "s1" {
			entry = &loadedConfig.Servers[i]
		}
	}
	if entry == nil {
		t.Fatal("s1 missing from persisted Servers")
	}
	if entry.DBPass != "oldsecret" {
		t.Errorf("DBPass = %q, want oldsecret (masked restored)", entry.DBPass)
	}
	if entry.DBName != "newdb" {
		t.Errorf("DBName = %q, want newdb", entry.DBName)
	}
}

func TestHandleUpdateServerConfig_DefaultWritesFlatConfig(t *testing.T) {
	t.Setenv("DUNE_ADMIN_CONFIG_DIR", t.TempDir())

	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{
		ID:   "default",
		Name: "Default",
		Cfg:  ServerConfig{ID: "default", Name: "Default", Control: "local", DBName: "dune"},
	})
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	origCfg := loadedConfig
	origDBName := dbName
	loadedConfig = appConfig{} // legacy flat, no Servers
	defer func() { loadedConfig = origCfg; dbName = origDBName }()

	body, _ := json.Marshal(ServerConfig{ID: "default", Name: "Default", Control: "local", DBName: "flatdb"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/servers/default/config", bytes.NewReader(body))
	req.SetPathValue("id", "default")
	rr := httptest.NewRecorder()
	handleUpdateServerConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if len(loadedConfig.Servers) != 0 {
		t.Errorf("Servers should stay empty for legacy default, got %d", len(loadedConfig.Servers))
	}
	if loadedConfig.DBName != "flatdb" {
		t.Errorf("loadedConfig.DBName = %q, want flatdb (flat field written)", loadedConfig.DBName)
	}
	if dbName != "flatdb" {
		t.Errorf("dbName global = %q, want flatdb (applyConfig ran)", dbName)
	}
}
