package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"dune-admin/internal/marketbot"

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

// @Summary Get market bot running status and mode
// @Tags market-bot
// @Produce json
// @Success 200 {object} map[string]any
// @Router /api/v1/market-bot/status [get]
func handleMarketBotStatus(w http.ResponseWriter, r *http.Request) {
	bot := activeBot(r)
	if bot != nil {
		snap := bot.StatusSnapshot()
		m, _ := json.Marshal(snap)
		var out map[string]any
		_ = json.Unmarshal(m, &out)
		if out == nil {
			out = map[string]any{}
		}
		running := bot.Enabled()
		out["running"] = running
		out["enabled"] = running
		out["mode"] = "embedded"
		out["configured"] = true
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
		out["configured"] = true
		jsonOK(w, out)
		return
	}
	configured := botConfiguredFor(r)
	errMsg := "market bot not configured; enable it for this server or set market_bot_remote_url"
	if configured {
		errMsg = "market bot enabled but not running; check the server's DB connection and logs"
	}
	jsonOK(w, map[string]any{
		"running":    false,
		"enabled":    false,
		"mode":       "none",
		"configured": configured,
		"error":      errMsg,
	})
}

// ── config ────────────────────────────────────────────────────────────────────

// @Summary Get market bot configuration
// @Tags market-bot
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 503 {object} map[string]string
// @Router /api/v1/market-bot/config [get]

// @Summary Update market bot configuration
// @Tags market-bot
// @Accept json
// @Produce json
// @Param body body object true "Configuration patch"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/market-bot/config [put]
func handleMarketBotConfig(w http.ResponseWriter, r *http.Request) {
	if bot := activeBot(r); bot != nil {
		handleEmbeddedBotConfig(bot, w, r)
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

func handleEmbeddedBotConfig(bot *marketbot.Instance, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data, err := bot.ConfigJSON()
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
		if err := bot.ApplyConfig(patch); err != nil {
			jsonErr(w, err, 400)
			return
		}
		data, err := bot.ConfigJSON()
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

// @Summary Execute a lifecycle command on the market bot (start/stop/restart)
// @Tags market-bot
// @Accept json
// @Produce json
// @Param body body object true "Command: start, stop, or restart"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/market-bot/exec [post]
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

	if bot := activeBot(r); bot != nil {
		output := "ok"
		switch req.Cmd {
		case "start":
			bot.Resume()
			output = "resumed"
		case "stop":
			bot.Pause()
			output = "paused"
		case "restart":
			if err := bot.Restart(r.Context()); err != nil {
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

// @Summary Trigger market bot listing cleanup
// @Tags market-bot
// @Produce json
// @Success 200 {object} map[string]int64
// @Failure 503 {object} map[string]string
// @Router /api/v1/market-bot/cleanup [post]
func handleMarketBotCleanup(w http.ResponseWriter, r *http.Request) {
	if bot := activeBot(r); bot != nil {
		orders, items, err := bot.CleanupListings(r.Context())
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

// @Summary Check whether market bot log streaming is available
// @Tags market-bot
// @Produce json
// @Success 200 {object} map[string]any
// @Router /api/v1/market-bot/logs-ready [get]
func handleMarketBotLogsReady(w http.ResponseWriter, r *http.Request) {
	if activeBot(r) != nil {
		jsonOK(w, map[string]any{"ready": true, "mode": "embedded"})
		return
	}
	if remoteBotProxy != nil {
		jsonOK(w, map[string]any{"ready": true, "mode": "remote"})
		return
	}
	jsonOK(w, map[string]any{"ready": false, "mode": "none", "reason": "market bot not configured"})
}

// @Summary Stream market bot log output via WebSocket
// @Tags market-bot
// @Produce text/plain
// @Success 101 {string} string "Switching Protocols"
// @Failure 503 {object} map[string]string
// @Router /api/v1/market-bot/logs [get]
func handleMarketBotLogs(w http.ResponseWriter, r *http.Request) {
	if bot := activeBot(r); bot != nil {
		streamEmbeddedBotLogs(bot, w, r)
		return
	}
	if remoteBotProxy != nil {
		proxyBotLogsWS(w, r, remoteBotProxy)
		return
	}
	http.Error(w, "market bot not configured", http.StatusServiceUnavailable)
}

func streamEmbeddedBotLogs(bot *marketbot.Instance, w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetWriteDeadline(time.Time{})

	ch := bot.Sink.Subscribe()
	defer bot.Sink.Unsubscribe(ch)
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
		componentLog("market_bot").Error().Err(err).Msg("remote bot ws dial failed")
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
