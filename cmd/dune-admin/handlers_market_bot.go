package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// ── status ────────────────────────────────────────────────────────────────────

func handleMarketBotStatus(w http.ResponseWriter, r *http.Request) {
	if embeddedBot == nil {
		jsonOK(w, map[string]any{
			"running": false,
			"enabled": false,
			"mode":    "embedded",
			"error":   "embedded market bot is disabled; set market_bot_enabled: true and restart dune-admin",
		})
		return
	}
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
}

// ── config ────────────────────────────────────────────────────────────────────

func handleMarketBotConfig(w http.ResponseWriter, r *http.Request) {
	if embeddedBot == nil {
		jsonErr(w, fmt.Errorf("embedded market bot is disabled"), http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		data, err := embeddedBot.ConfigJSON()
		if err != nil {
			jsonErr(w, err, 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data) //nolint:errcheck
		return
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
		return
	default:
		jsonErr(w, fmt.Errorf("method not allowed"), http.StatusMethodNotAllowed)
		return
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

	if embeddedBot == nil {
		jsonErr(w, fmt.Errorf("embedded market bot is disabled"), http.StatusServiceUnavailable)
		return
	}
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
}

// ── log streaming ─────────────────────────────────────────────────────────────

func handleMarketBotLogsReady(w http.ResponseWriter, r *http.Request) {
	if embeddedBot == nil {
		jsonOK(w, map[string]any{"ready": false, "mode": "embedded", "reason": "embedded market bot is disabled"})
		return
	}
	jsonOK(w, map[string]any{"ready": true, "mode": "embedded"})
}

func handleMarketBotLogs(w http.ResponseWriter, r *http.Request) {
	if embeddedBot == nil {
		http.Error(w, "embedded market bot is disabled", http.StatusServiceUnavailable)
		return
	}
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
