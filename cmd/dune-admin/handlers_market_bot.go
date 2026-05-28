package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// ── remote bot proxy ─────────────────────────────────────────────────────────

// remoteBotClient proxies /api/v1/market-bot/* calls to a standalone market
// bot's HTTP API (internal/marketbot.APIServer). Used when market_bot_enabled
// is false but market_bot_remote_url is set.
type remoteBotClient struct {
	baseURL string // trailing slash stripped
	token   string
	client  *http.Client
}

func newRemoteBotClient(rawURL, token string) *remoteBotClient {
	return &remoteBotClient{
		baseURL: strings.TrimRight(rawURL, "/"),
		token:   token,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// do executes a request against the remote bot, copies the response status and
// body back to w, and returns true on success.
func (r *remoteBotClient) do(w http.ResponseWriter, method, path string, body io.Reader) bool {
	u := r.baseURL + path
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		jsonErr(w, fmt.Errorf("remote bot: build request: %w", err), http.StatusInternalServerError)
		return false
	}
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := r.client.Do(req)
	if err != nil {
		jsonErr(w, fmt.Errorf("remote bot unreachable: %w", err), http.StatusBadGateway)
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
	return resp.StatusCode < 300
}

// wsURL converts the base HTTP URL to a WebSocket URL.
func (r *remoteBotClient) wsURL(path string) string {
	return strings.NewReplacer("https://", "wss://", "http://", "ws://").
		Replace(r.baseURL) + path
}

// ── status ────────────────────────────────────────────────────────────────────

func handleMarketBotStatus(w http.ResponseWriter, r *http.Request) {
	if embeddedBot != nil {
		snap := embeddedBot.StatusSnapshot()
		m, _ := json.Marshal(snap)
		var out map[string]any
		_ = json.Unmarshal(m, &out)
		if out == nil {
			out = map[string]any{}
		}
		running := embeddedBot.Enabled()
		out["running"] = running
		out["enabled"] = running
		out["mode"] = "embedded"
		jsonOK(w, out)
		return
	}
	if remoteBotProxy != nil {
		// Fetch status from remote and augment with mode field.
		u := remoteBotProxy.baseURL + "/status"
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
		if err != nil {
			jsonErr(w, fmt.Errorf("remote bot: %w", err), http.StatusInternalServerError)
			return
		}
		if remoteBotProxy.token != "" {
			req.Header.Set("Authorization", "Bearer "+remoteBotProxy.token)
		}
		resp, err := remoteBotProxy.client.Do(req)
		if err != nil {
			jsonErr(w, fmt.Errorf("remote bot unreachable: %w", err), http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			jsonErr(w, fmt.Errorf("remote bot: decode: %w", err), http.StatusBadGateway)
			return
		}
		if out == nil {
			out = map[string]any{}
		}
		out["mode"] = "remote"
		out["running"] = true
		out["enabled"] = true
		jsonOK(w, out)
		return
	}
	jsonOK(w, map[string]any{
		"running": false,
		"enabled": false,
		"mode":    "none",
		"error":   "market bot not configured; set market_bot_enabled: true or market_bot_remote_url",
	})
}

// ── config ────────────────────────────────────────────────────────────────────

func handleMarketBotConfig(w http.ResponseWriter, r *http.Request) {
	if embeddedBot != nil {
		handleEmbeddedBotConfig(w, r)
		return
	}
	if remoteBotProxy != nil {
		switch r.Method {
		case http.MethodGet:
			remoteBotProxy.do(w, http.MethodGet, "/config", nil)
		case http.MethodPut:
			remoteBotProxy.do(w, http.MethodPut, "/config", r.Body)
		default:
			jsonErr(w, fmt.Errorf("method not allowed"), http.StatusMethodNotAllowed)
		}
		return
	}
	jsonErr(w, fmt.Errorf("market bot not configured"), http.StatusServiceUnavailable)
}

func handleEmbeddedBotConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data, err := embeddedBot.ConfigJSON()
		if err != nil {
			jsonErr(w, err, 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data) //nolint:errcheck
	case http.MethodPut:
		var patch map[string]json.RawMessage
		if err := decode(r, &patch); err != nil {
			jsonErr(w, err, 400)
			return
		}
		if err := embeddedBot.ApplyConfig(patch); err != nil {
			jsonErr(w, err, 400)
			return
		}
		data, err := embeddedBot.ConfigJSON()
		if err != nil {
			jsonErr(w, err, 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data) //nolint:errcheck
	default:
		jsonErr(w, fmt.Errorf("method not allowed"), http.StatusMethodNotAllowed)
	}
}

// ── lifecycle exec ────────────────────────────────────────────────────────────

var botCmdAllowlist = map[string]bool{
	"start": true, "stop": true, "restart": true,
}

func handleMarketBotExec(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cmd string `json:"cmd"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if !botCmdAllowlist[req.Cmd] {
		jsonErr(w, fmt.Errorf("unknown command %q", req.Cmd), 400)
		return
	}

	if embeddedBot != nil {
		output := "ok"
		switch req.Cmd {
		case "start":
			embeddedBot.Resume()
			output = "resumed"
		case "stop":
			embeddedBot.Pause()
			output = "paused"
		case "restart":
			if err := embeddedBot.Restart(r.Context()); err != nil {
				jsonErr(w, err, 500)
				return
			}
			output = "restarted"
		}
		jsonOK(w, map[string]string{"output": output})
		return
	}
	if remoteBotProxy != nil {
		body, _ := json.Marshal(map[string]string{"cmd": req.Cmd})
		remoteBotProxy.do(w, http.MethodPost, "/exec", strings.NewReader(string(body)))
		return
	}
	jsonErr(w, fmt.Errorf("market bot not configured"), http.StatusServiceUnavailable)
}

// ── cleanup ───────────────────────────────────────────────────────────────────

func handleMarketBotCleanup(w http.ResponseWriter, r *http.Request) {
	if embeddedBot != nil {
		orders, items, err := embeddedBot.CleanupListings(r.Context())
		if err != nil {
			jsonErr(w, err, 500)
			return
		}
		jsonOK(w, map[string]int64{
			"orders_deleted": orders,
			"items_deleted":  items,
		})
		return
	}
	if remoteBotProxy != nil {
		remoteBotProxy.do(w, http.MethodPost, "/cleanup", nil)
		return
	}
	jsonErr(w, fmt.Errorf("market bot not configured"), http.StatusServiceUnavailable)
}

// ── log streaming ─────────────────────────────────────────────────────────────

func handleMarketBotLogsReady(w http.ResponseWriter, _ *http.Request) {
	if embeddedBot != nil {
		jsonOK(w, map[string]any{"ready": true, "mode": "embedded"})
		return
	}
	if remoteBotProxy != nil {
		jsonOK(w, map[string]any{"ready": true, "mode": "remote"})
		return
	}
	jsonOK(w, map[string]any{"ready": false, "mode": "none", "reason": "market bot not configured"})
}

func handleMarketBotLogs(w http.ResponseWriter, r *http.Request) {
	if embeddedBot != nil {
		streamEmbeddedBotLogs(w, r)
		return
	}
	if remoteBotProxy != nil {
		proxyBotLogsWS(w, r, remoteBotProxy)
		return
	}
	http.Error(w, "market bot not configured", http.StatusServiceUnavailable)
}

func streamEmbeddedBotLogs(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetWriteDeadline(time.Time{})

	ch := embeddedBot.Sink.Subscribe()
	defer embeddedBot.Sink.Unsubscribe(ch)
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

// proxyBotLogsWS bridges the client WebSocket connection to the remote bot's
// /logs WebSocket endpoint, relaying text frames in both directions.
func proxyBotLogsWS(w http.ResponseWriter, r *http.Request, proxy *remoteBotClient) {
	clientConn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = clientConn.Close() }()

	hdr := http.Header{}
	if proxy.token != "" {
		hdr.Set("Authorization", "Bearer "+proxy.token)
	}
	remoteConn, _, err := websocket.DefaultDialer.DialContext(r.Context(), proxy.wsURL("/logs"), hdr)
	if err != nil {
		log.Printf("remote bot ws dial: %v", err)
		_ = clientConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "remote unavailable"))
		return
	}
	defer func() { _ = remoteConn.Close() }()

	bridgeWSConns(r.Context(), clientConn, remoteConn)
}

// bridgeWSConns relays frames between two WebSocket connections until either
// closes or the context is cancelled.
func bridgeWSConns(ctx context.Context, a, b *websocket.Conn) {
	done := make(chan struct{}, 2)
	relay := func(src, dst *websocket.Conn) {
		defer func() { done <- struct{}{} }()
		for {
			mt, msg, err := src.ReadMessage()
			if err != nil {
				return
			}
			if err := dst.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}
	go relay(b, a) // remote → client
	go relay(a, b) // client → remote
	select {
	case <-done:
	case <-ctx.Done():
	}
}
