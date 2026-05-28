package marketbot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) (*APIServer, *Config) {
	t.Helper()
	cfg := &Config{}
	cfg.config = defaultConfig()
	srv := newAPIServer(cfg, &Exchange{}, nil, "test-token")
	return srv, cfg
}

func TestHealthNoAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d", w.Code)
	}
	var body map[string]bool
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body["ok"] {
		t.Error("expected ok:true")
	}
}

func TestAuthRequired(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, path := range []string{"/status", "/config", "/report"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: want 401 got %d", path, w.Code)
		}
	}
}

func TestGetConfig(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/config", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := body["max_buys"]; !ok {
		t.Error("expected max_buys in response")
	}
	if _, ok := body["buy_interval"].(string); !ok {
		t.Errorf("expected buy_interval to be a string, got %T", body["buy_interval"])
	}
}

func TestPutConfig(t *testing.T) {
	srv, cfg := newTestServer(t)
	body := `{"max_buys": 25}`
	req := httptest.NewRequest("PUT", "/config", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	if cfg.Snapshot().MaxBuys != 25 {
		t.Error("config not updated")
	}
}

func TestPutConfigInvalid(t *testing.T) {
	srv, _ := newTestServer(t)
	body := `{"max_buys": -5}`
	req := httptest.NewRequest("PUT", "/config", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 got %d", w.Code)
	}
}

func TestConfigReload(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("POST", "/config/reload", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d", w.Code)
	}
}

func TestReport(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/report", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d: %s", w.Code, w.Body.String())
	}
}

func TestNoTokenRejectsAll(t *testing.T) {
	cfg := &Config{}
	cfg.config = defaultConfig()
	srv := newAPIServer(cfg, &Exchange{}, nil, "")
	for _, path := range []string{"/status", "/config", "/report"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s with no token: want 401 got %d", path, w.Code)
		}
	}
}

func TestExecStart(t *testing.T) {
	cfg := &Config{}
	cfg.config = defaultConfig()
	cfg.config.Enabled = false
	inst := &Instance{cfg: cfg}
	srv := newAPIServer(cfg, &Exchange{}, inst, "test-token")

	req := httptest.NewRequest("POST", "/exec", strings.NewReader(`{"cmd":"start"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("exec start: want 200 got %d: %s", w.Code, w.Body.String())
	}
	if !cfg.Snapshot().Enabled {
		t.Error("exec start: bot should be enabled after start")
	}
}

func TestExecStop(t *testing.T) {
	cfg := &Config{}
	cfg.config = defaultConfig()
	cfg.config.Enabled = true
	inst := &Instance{cfg: cfg}
	srv := newAPIServer(cfg, &Exchange{}, inst, "test-token")

	req := httptest.NewRequest("POST", "/exec", strings.NewReader(`{"cmd":"stop"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("exec stop: want 200 got %d: %s", w.Code, w.Body.String())
	}
	if cfg.Snapshot().Enabled {
		t.Error("exec stop: bot should be disabled after stop")
	}
}

func TestExecUnknownCmd(t *testing.T) {
	cfg := &Config{}
	cfg.config = defaultConfig()
	inst := &Instance{cfg: cfg}
	srv := newAPIServer(cfg, &Exchange{}, inst, "test-token")

	req := httptest.NewRequest("POST", "/exec", strings.NewReader(`{"cmd":"explode"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("exec unknown: want 400 got %d", w.Code)
	}
}

func TestExecRequiresInstance(t *testing.T) {
	srv, _ := newTestServer(t) // no instance

	req := httptest.NewRequest("POST", "/exec", strings.NewReader(`{"cmd":"start"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("exec without instance: want 503 got %d", w.Code)
	}
}

func TestLogsReady(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/logs-ready", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("logs-ready: want 200 got %d", w.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["ready"] != true {
		t.Errorf("logs-ready: want ready=true got %v", body["ready"])
	}
}
