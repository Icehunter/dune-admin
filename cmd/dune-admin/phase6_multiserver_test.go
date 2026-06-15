package main

// Phase 6 — Red tests for multi-server config loading and per-server reconnect handler.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ── Config loading ────────────────────────────────────────────────────────────

func TestAppConfig_ServersYAML(t *testing.T) {
	const raw = `
servers:
  - id: "s1"
    name: "Server One"
    db_host: "db1.local"
    db_port: 5432
    control: "local"
  - id: "s2"
    name: "Server Two"
    db_host: "db2.local"
    db_port: 5432
    control: "docker"
default_server: "s2"
`
	var cfg appConfig
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("got %d servers, want 2", len(cfg.Servers))
	}
	if cfg.Servers[0].ID != "s1" {
		t.Errorf("servers[0].ID = %q, want s1", cfg.Servers[0].ID)
	}
	if cfg.Servers[1].ID != "s2" {
		t.Errorf("servers[1].ID = %q, want s2", cfg.Servers[1].ID)
	}
	if cfg.Servers[1].Control != "docker" {
		t.Errorf("servers[1].Control = %q, want docker", cfg.Servers[1].Control)
	}
	if cfg.DefaultServer != "s2" {
		t.Errorf("DefaultServer = %q, want s2", cfg.DefaultServer)
	}
}

func TestAppConfig_ServersEmpty_LegacyPath(t *testing.T) {
	const raw = `
db_host: "db.local"
db_port: 5432
control: "local"
`
	var cfg appConfig
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cfg.Servers) != 0 {
		t.Errorf("got %d servers, want 0 (legacy flat config)", len(cfg.Servers))
	}
	if cfg.DefaultServer != "" {
		t.Errorf("DefaultServer = %q, want empty for legacy config", cfg.DefaultServer)
	}
}

// ── Per-server reconnect handler ──────────────────────────────────────────────

func TestHandleReconnectServer_NotFound(t *testing.T) {
	// Registry with one server; request targets a non-existent ID.
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "s1", Name: "S1"})
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers/does-not-exist/reconnect", nil)
	req.SetPathValue("id", "does-not-exist")
	rr := httptest.NewRecorder()
	handleReconnectServer(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestHandleListServers_MultiServer(t *testing.T) {
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "s1", Name: "Alpha"})
	reg.Register(&ServerContext{ID: "s2", Name: "Beta"})
	_ = reg.SetActive("s2")
	orig := globalRegistry
	globalRegistry = reg
	defer func() { globalRegistry = orig }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers", nil)
	rr := httptest.NewRecorder()
	handleListServers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var items []serverListItem
	if err := json.NewDecoder(rr.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	// active flag must be on s2
	var foundActive bool
	for _, it := range items {
		if it.ID == "s2" && it.Active {
			foundActive = true
		}
	}
	if !foundActive {
		t.Error("expected s2 to be marked active")
	}
}

// ── CORS allows X-Dune-Server ─────────────────────────────────────────────────

func TestCORSAllowsXDuneServerHeader(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := corsMiddleware(inner)
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/players", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Headers", "X-Dune-Server")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	allowed := rr.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(strings.ToLower(allowed), "x-dune-server") {
		t.Errorf("X-Dune-Server not in Access-Control-Allow-Headers: %q", allowed)
	}
}
