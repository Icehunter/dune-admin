package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// botProxy forwards a request to the market bot API and returns the raw body.
// Returns (body, statusCode, error).
func botProxy(method, path string, body io.Reader) ([]byte, int, error) {
	if marketBotAddr == "" {
		return nil, 503, fmt.Errorf("market_bot_addr not configured")
	}
	req, err := http.NewRequestWithContext(context.Background(), method, marketBotAddr+path, body)
	if err != nil {
		return nil, 500, err
	}
	if marketBotToken != "" {
		req.Header.Set("Authorization", "Bearer "+marketBotToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req) // #nosec G704 -- marketBotAddr is admin-configured, not user-supplied
	if err != nil {
		return nil, 503, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

// handleMarketBotStatus proxies GET /status from the bot API and injects a
// "running" field (true when the bot responds, false when unreachable).
func handleMarketBotStatus(w http.ResponseWriter, r *http.Request) {
	data, _, err := botProxy(http.MethodGet, "/status", nil)
	if err != nil {
		// Bot is unreachable — return a minimal status rather than an error so
		// the frontend can show a "stopped" state instead of an error banner.
		jsonOK(w, map[string]any{"running": false, "error": err.Error()})
		return
	}
	var m map[string]any
	if json.Unmarshal(data, &m) == nil {
		m["running"] = true
		jsonOK(w, m)
		return
	}
	// Passthrough if JSON parsing fails.
	w.Header().Set("Content-Type", "application/json")
	w.Write(data) //nolint:errcheck
}

// handleMarketBotConfig proxies GET/PUT /config from/to the bot API.
// On a successful PUT the bot returns a short ack, not the updated config,
// so we follow up with a GET and return the canonical config to the client.
func handleMarketBotConfig(w http.ResponseWriter, r *http.Request) {
	var body io.Reader
	if r.Method == http.MethodPut {
		body = r.Body
	}
	data, code, err := botProxy(r.Method, "/config", body)
	if err != nil {
		jsonErr(w, err, code)
		return
	}
	if r.Method == http.MethodPut && code == http.StatusOK {
		if cfgData, _, cfgErr := botProxy(http.MethodGet, "/config", nil); cfgErr == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write(cfgData) //nolint:errcheck
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data) //nolint:errcheck
}

var botCmdAllowlist = map[string]bool{
	"start": true, "stop": true, "restart": true,
}

// handleMarketBotExec runs a lifecycle command (start/stop/restart) on the bot
// using the configured control plane.
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
	if globalExecutor == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	if marketBotContainer == "" {
		jsonErr(w, fmt.Errorf("market_bot_container not configured"), 503)
		return
	}

	out, err := execBotCommand(r.Context(), req.Cmd)
	if err != nil {
		jsonErr(w, fmt.Errorf("exec: %w — output: %s", err, out), 500)
		return
	}
	jsonOK(w, map[string]string{"output": out})
}

// execBotCommand runs start/stop/restart on the bot container/deployment.
func execBotCommand(ctx context.Context, cmd string) (string, error) {
	if globalControl == nil {
		return "", fmt.Errorf("not connected")
	}
	switch globalControl.Name() {
	case "kubectl":
		ns := marketBotNamespace
		if ns == "" {
			ns = "default"
		}
		switch cmd {
		case "start":
			return globalExecutor.Exec(fmt.Sprintf("sudo kubectl scale deployment/%s -n %s --replicas=1 2>&1", marketBotContainer, ns))
		case "stop":
			return globalExecutor.Exec(fmt.Sprintf("sudo kubectl scale deployment/%s -n %s --replicas=0 2>&1", marketBotContainer, ns))
		case "restart":
			return globalExecutor.Exec(fmt.Sprintf("sudo kubectl rollout restart deployment/%s -n %s 2>&1", marketBotContainer, ns))
		}
	case "docker":
		return globalExecutor.Exec(fmt.Sprintf("docker %s %s 2>&1", cmd, marketBotContainer))
	}
	return "", fmt.Errorf("lifecycle commands not supported for %s control plane", globalControl.Name())
}

// handleMarketBotLogsReady returns whether log streaming is available and why not if not.
// The WebSocket client calls this first so it can surface HTTP error bodies
// (which browsers don't expose on WS connection failure).
func handleMarketBotLogsReady(w http.ResponseWriter, r *http.Request) {
	if globalControl == nil || globalExecutor == nil {
		jsonOK(w, map[string]any{"ready": false, "reason": "not connected to server"})
		return
	}
	if marketBotContainer == "" {
		jsonOK(w, map[string]any{"ready": false, "reason": "market_bot_container not configured"})
		return
	}
	ns, name, err := botLogSource(r.Context())
	if err != nil {
		jsonOK(w, map[string]any{"ready": false, "reason": err.Error()})
		return
	}
	jsonOK(w, map[string]any{"ready": true, "namespace": ns, "name": name})
}

// handleMarketBotLogs streams bot container logs over WebSocket.
// It discovers the bot pod (kubectl) or uses the container name directly (docker).
func handleMarketBotLogs(w http.ResponseWriter, r *http.Request) {
	if globalControl == nil || globalExecutor == nil {
		http.Error(w, "not connected", http.StatusServiceUnavailable)
		return
	}
	if marketBotContainer == "" {
		http.Error(w, "market_bot_container not configured", http.StatusServiceUnavailable)
		return
	}

	ns, name, err := botLogSource(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetWriteDeadline(time.Time{})

	ch, cancel, err := globalControl.StreamLog(r.Context(), globalExecutor, ns, name)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error())) //nolint:errcheck
		return
	}
	defer cancel()

	for line := range ch {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
			return
		}
	}
}

// botLogSource returns the namespace and pod/container name for log streaming.
func botLogSource(ctx context.Context) (ns, name string, err error) {
	switch globalControl.Name() {
	case "kubectl":
		botNS := marketBotNamespace
		if botNS == "" {
			botNS = "default"
		}
		// Try label selector matching deployment name first (most accurate).
		labelSel := ""
		if marketBotContainer != "" {
			labelSel = fmt.Sprintf(" -l app=%s", marketBotContainer)
		}
		out, execErr := globalExecutor.Exec(fmt.Sprintf(
			"sudo kubectl get pods -n %s%s --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}' 2>/dev/null",
			botNS, labelSel))
		pod := strings.TrimSpace(strings.Trim(out, "'"))
		if execErr != nil || pod == "" {
			// Retry without Running filter — pod might be initialising.
			out, _ = globalExecutor.Exec(fmt.Sprintf(
				"sudo kubectl get pods -n %s%s -o jsonpath='{.items[0].metadata.name}' 2>/dev/null",
				botNS, labelSel))
			pod = strings.TrimSpace(strings.Trim(out, "'"))
		}
		if execErr != nil || pod == "" {
			// Last resort: any pod in namespace, no label filter.
			out, _ = globalExecutor.Exec(fmt.Sprintf(
				"sudo kubectl get pods -n %s -o jsonpath='{.items[0].metadata.name}' 2>/dev/null",
				botNS))
			pod = strings.TrimSpace(strings.Trim(out, "'"))
		}
		if pod == "" {
			return "", "", fmt.Errorf("no running pod found in namespace %s (deployment: %s)", botNS, marketBotContainer)
		}
		return botNS, pod, nil
	default:
		// docker/local: container name is used directly.
		return "", marketBotContainer, nil
	}
}
