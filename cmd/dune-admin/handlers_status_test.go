package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleStatus_IncludesDirectorAndListen verifies the status endpoint
// surfaces the configured director_url and listen_addr so the Server Health
// page can render the Web Interfaces card and port chips without guessing.
func TestHandleStatus_IncludesDirectorAndListen(t *testing.T) {
	// Not parallel: handleStatus reads the loadedConfig package global + registry.
	prev := loadedConfig
	prevReg := globalRegistry
	t.Cleanup(func() { loadedConfig = prev; globalRegistry = prevReg })
	// director_url is per-server (active server's ServerConfig) after the remodel;
	// listen_addr stays global.
	loadedConfig = appConfig{ListenAddr: ":9090"}
	globalRegistry = newServerRegistry(nil)
	globalRegistry.Register(&ServerContext{ID: "1", Cfg: ServerConfig{ID: 1, DirectorURL: "http://127.0.0.1:11717"}})
	if err := globalRegistry.SetActive("1"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	rec := httptest.NewRecorder()
	handleStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["director_url"] != "http://127.0.0.1:11717" {
		t.Fatalf("director_url = %v, want configured value", body["director_url"])
	}
	if body["listen_addr"] != ":9090" {
		t.Fatalf("listen_addr = %v, want configured value", body["listen_addr"])
	}
}
