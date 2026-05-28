package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newRemoteFakeServer creates a fake remote bot HTTP server and returns a
// remoteBotClient pointed at it, plus a cleanup function.
func newRemoteFakeServer(t *testing.T, mux *http.ServeMux) (*remoteBotClient, func()) {
	t.Helper()
	ts := httptest.NewServer(mux)
	client := newRemoteBotClient(ts.URL, "fake-token")
	// Override the HTTP client to use the test server's client so redirects work.
	client.client = ts.Client()
	return client, ts.Close
}

func TestHandleMarketBotStatus_NeitherConfigured(t *testing.T) {
	origBot := embeddedBot
	origProxy := remoteBotProxy
	embeddedBot = nil
	remoteBotProxy = nil
	defer func() { embeddedBot = origBot; remoteBotProxy = origProxy }()

	req := httptest.NewRequest("GET", "/api/v1/market-bot/status", nil)
	w := httptest.NewRecorder()
	handleMarketBotStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d", w.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["running"] != false {
		t.Errorf("running should be false, got %v", body["running"])
	}
	if body["mode"] != "none" {
		t.Errorf("mode should be 'none', got %v", body["mode"])
	}
}

func TestHandleMarketBotStatus_RemoteProxy(t *testing.T) {
	origBot := embeddedBot
	origProxy := remoteBotProxy
	embeddedBot = nil
	defer func() { embeddedBot = origBot; remoteBotProxy = origProxy }()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fake-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"uptime":"1m","listing_count":42}`)
	})

	proxy, cleanup := newRemoteFakeServer(t, mux)
	defer cleanup()
	remoteBotProxy = proxy

	req := httptest.NewRequest("GET", "/api/v1/market-bot/status", nil)
	w := httptest.NewRecorder()
	handleMarketBotStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["mode"] != "remote" {
		t.Errorf("mode should be 'remote', got %v", body["mode"])
	}
	if body["running"] != true {
		t.Errorf("running should be true for reachable remote, got %v", body["running"])
	}
	if body["listing_count"].(float64) != 42 {
		t.Errorf("listing_count should be 42, got %v", body["listing_count"])
	}
}

func TestHandleMarketBotConfig_RemoteGet(t *testing.T) {
	origBot := embeddedBot
	origProxy := remoteBotProxy
	embeddedBot = nil
	defer func() { embeddedBot = origBot; remoteBotProxy = origProxy }()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"max_buys":99,"enabled":true}`)
	})

	proxy, cleanup := newRemoteFakeServer(t, mux)
	defer cleanup()
	remoteBotProxy = proxy

	req := httptest.NewRequest("GET", "/api/v1/market-bot/config", nil)
	w := httptest.NewRecorder()
	handleMarketBotConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"max_buys":99`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestHandleMarketBotConfig_RemotePut(t *testing.T) {
	origBot := embeddedBot
	origProxy := remoteBotProxy
	embeddedBot = nil
	defer func() { embeddedBot = origBot; remoteBotProxy = origProxy }()

	var receivedBody string
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /config", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})

	proxy, cleanup := newRemoteFakeServer(t, mux)
	defer cleanup()
	remoteBotProxy = proxy

	req := httptest.NewRequest("PUT", "/api/v1/market-bot/config",
		strings.NewReader(`{"max_buys":7}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleMarketBotConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(receivedBody, `"max_buys":7`) {
		t.Errorf("remote did not receive correct body: %s", receivedBody)
	}
}

func TestHandleMarketBotConfig_NeitherConfigured(t *testing.T) {
	origBot := embeddedBot
	origProxy := remoteBotProxy
	embeddedBot = nil
	remoteBotProxy = nil
	defer func() { embeddedBot = origBot; remoteBotProxy = origProxy }()

	req := httptest.NewRequest("GET", "/api/v1/market-bot/config", nil)
	w := httptest.NewRecorder()
	handleMarketBotConfig(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503 got %d", w.Code)
	}
}

func TestHandleMarketBotExec_Remote(t *testing.T) {
	origBot := embeddedBot
	origProxy := remoteBotProxy
	embeddedBot = nil
	defer func() { embeddedBot = origBot; remoteBotProxy = origProxy }()

	var receivedCmd string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /exec", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		receivedCmd = body["cmd"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"output":"resumed"}`)
	})

	proxy, cleanup := newRemoteFakeServer(t, mux)
	defer cleanup()
	remoteBotProxy = proxy

	req := httptest.NewRequest("POST", "/api/v1/market-bot/exec",
		strings.NewReader(`{"cmd":"start"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleMarketBotExec(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	if receivedCmd != "start" {
		t.Errorf("remote received cmd=%q want 'start'", receivedCmd)
	}
}

func TestHandleMarketBotExec_UnknownCmd(t *testing.T) {
	origBot := embeddedBot
	origProxy := remoteBotProxy
	embeddedBot = nil
	remoteBotProxy = nil
	defer func() { embeddedBot = origBot; remoteBotProxy = origProxy }()

	req := httptest.NewRequest("POST", "/api/v1/market-bot/exec",
		strings.NewReader(`{"cmd":"nuke"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleMarketBotExec(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("unknown cmd: want 400 got %d", w.Code)
	}
}

func TestHandleMarketBotCleanup_Remote(t *testing.T) {
	origBot := embeddedBot
	origProxy := remoteBotProxy
	embeddedBot = nil
	defer func() { embeddedBot = origBot; remoteBotProxy = origProxy }()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /cleanup", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"orders_deleted":5,"items_deleted":10}`)
	})

	proxy, cleanup := newRemoteFakeServer(t, mux)
	defer cleanup()
	remoteBotProxy = proxy

	req := httptest.NewRequest("POST", "/api/v1/market-bot/cleanup", nil)
	w := httptest.NewRecorder()
	handleMarketBotCleanup(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"orders_deleted":5`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestHandleMarketBotLogsReady_Remote(t *testing.T) {
	origBot := embeddedBot
	origProxy := remoteBotProxy
	embeddedBot = nil
	defer func() { embeddedBot = origBot; remoteBotProxy = origProxy }()

	remoteBotProxy = newRemoteBotClient("http://irrelevant", "tok")

	req := httptest.NewRequest("GET", "/api/v1/market-bot/logs-ready", nil)
	w := httptest.NewRecorder()
	handleMarketBotLogsReady(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d", w.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["ready"] != true {
		t.Errorf("ready should be true, got %v", body["ready"])
	}
	if body["mode"] != "remote" {
		t.Errorf("mode should be 'remote', got %v", body["mode"])
	}
}

func TestHandleMarketBotLogsReady_NeitherConfigured(t *testing.T) {
	origBot := embeddedBot
	origProxy := remoteBotProxy
	embeddedBot = nil
	remoteBotProxy = nil
	defer func() { embeddedBot = origBot; remoteBotProxy = origProxy }()

	req := httptest.NewRequest("GET", "/api/v1/market-bot/logs-ready", nil)
	w := httptest.NewRecorder()
	handleMarketBotLogsReady(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d", w.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["ready"] != false {
		t.Errorf("ready should be false when nothing configured, got %v", body["ready"])
	}
}

func TestHandleMarketBotStatus_RemoteUnreachable(t *testing.T) {
	origBot := embeddedBot
	origProxy := remoteBotProxy
	embeddedBot = nil
	defer func() { embeddedBot = origBot; remoteBotProxy = origProxy }()

	remoteBotProxy = newRemoteBotClient("http://127.0.0.1:19999", "tok")

	req := httptest.NewRequest("GET", "/api/v1/market-bot/status", nil)
	w := httptest.NewRecorder()
	handleMarketBotStatus(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("unreachable remote: want 502 got %d", w.Code)
	}
}
