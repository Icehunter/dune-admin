package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// defaultAmpAPIPort is the AMP instance ADS Web API port, reachable from inside
// the AMP container at http://127.0.0.1:8081/API/.
const defaultAmpAPIPort = 8081

// ampAPIClient talks to a CubeCoders AMP instance's Web API. Under the AMP
// control plane, gameplay/server settings are owned by AMP: it regenerates
// UserEngine.ini / UserGame.ini from its own config (GenericModule.kvp →
// App.AppSettings) on every start, so a direct INI edit gets clobbered. Writing
// through the AMP API persists cleanly and survives restarts.
//
// Requests are issued by building a curl command, wrapping it for in-container
// execution via wrap (ampControl.wrapInContainer), and running it through the
// host Executor. The AMP ADS port is not exposed on the host, but the executor
// already execs into the container for logs and rabbitmqctl, so the same path
// reaches the loopback API with no extra port plumbing.
type ampAPIClient struct {
	exec      Executor
	wrap      func(string) string // wraps an in-container shell command
	user      string
	pass      string
	port      int
	sessionID string // cached after the first successful login
}

func newAMPAPIClient(exec Executor, wrap func(string) string, user, pass string, port int) *ampAPIClient {
	return &ampAPIClient{exec: exec, wrap: wrap, user: user, pass: pass, port: port}
}

func (c *ampAPIClient) apiPort() int {
	if c.port == 0 {
		return defaultAmpAPIPort
	}
	return c.port
}

func (c *ampAPIClient) endpoint(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/API/%s", c.apiPort(), path)
}

// buildCurl returns an in-container shell command that POSTs payload as JSON to
// the named AMP API endpoint. The JSON body is base64-piped to curl so
// operator-supplied values (passwords, server names) never touch the shell
// command line — eliminating both quoting bugs and shell-injection risk.
func (c *ampAPIClient) buildCurl(path string, payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal %s payload: %w", path, err)
	}
	b64 := base64.StdEncoding.EncodeToString(body)
	return fmt.Sprintf(
		"echo %s | base64 -d | curl -s -m 20 -X POST "+
			"-H 'Content-Type: application/json' -H 'Accept: application/json' "+
			"--data-binary @- %s",
		b64, c.endpoint(path)), nil
}

// post runs an AMP API call and returns the trimmed response body. Executor
// failures are wrapped and surface curl's stderr for diagnosis.
func (c *ampAPIClient) post(path string, payload any) (string, error) {
	cmd, err := c.buildCurl(path, payload)
	if err != nil {
		return "", err
	}
	out, err := c.exec.Exec(c.wrap(cmd))
	if err != nil {
		return "", fmt.Errorf("amp api %s: %w (output: %s)", path, err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

// login authenticates against Core/Login and caches the session ID. AMP returns
// a LoginResult; success is gated on both the success flag and a non-empty
// sessionID.
func (c *ampAPIClient) login() (string, error) {
	resp, err := c.post("Core/Login", map[string]any{
		"username":   c.user,
		"password":   c.pass,
		"token":      "",
		"rememberMe": false,
	})
	if err != nil {
		return "", err
	}
	var result struct {
		Success      bool   `json:"success"`
		ResultReason string `json:"resultReason"`
		SessionID    string `json:"sessionID"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(resp)), &result); err != nil {
		return "", fmt.Errorf("amp api Core/Login: decode response: %w (output: %s)", err, resp)
	}
	if !result.Success || result.SessionID == "" {
		reason := result.ResultReason
		if reason == "" {
			reason = "login failed"
		}
		return "", fmt.Errorf("amp api login rejected: %s", reason)
	}
	c.sessionID = result.SessionID
	return c.sessionID, nil
}

// ensureSession returns the cached session ID, logging in on first use.
func (c *ampAPIClient) ensureSession() (string, error) {
	if c.sessionID != "" {
		return c.sessionID, nil
	}
	return c.login()
}

// isSessionError reports whether an AMP API error looks like a session
// rejection (expired, invalid, or unknown session ID). Used to trigger a
// one-shot re-login rather than surfacing a confusing auth error to the
// operator — AMP sessions can expire if the server is idle for a long time.
func isSessionError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "session")
}

// setConfig writes a single AMP config node (e.g.
// "Meta.GenericModule.ConsoleVariables.Dune.GlobalMiningOutputMultiplier").
// AMP persists it to GenericModule.kvp and regenerates the game INIs on the
// next start.
//
// If AMP rejects the call with a session error (expired or invalid session),
// setConfig clears the cached session ID, re-logs in once, and retries the
// write. This handles the case where the in-process session goes stale between
// a successful login and a subsequent SetConfig within the same batch.
func (c *ampAPIClient) setConfig(node, value string) error {
	sid, err := c.ensureSession()
	if err != nil {
		return err
	}
	resp, err := c.post("Core/SetConfig", map[string]any{
		"node":      node,
		"value":     value,
		"SESSIONID": sid,
	})
	if err != nil {
		return err
	}
	if err := parseActionResult("SetConfig", node, resp); err != nil {
		if !isSessionError(err) {
			return err
		}
		// Session expired — force re-login and retry once.
		c.sessionID = ""
		sid, err = c.login()
		if err != nil {
			return err
		}
		resp, err = c.post("Core/SetConfig", map[string]any{
			"node":      node,
			"value":     value,
			"SESSIONID": sid,
		})
		if err != nil {
			return err
		}
		return parseActionResult("SetConfig", node, resp)
	}
	return nil
}

// updateApplication triggers AMP's game-server update via Core/UpdateApplication
// on the instance — the SteamCMD app_update AMP runs from its dashboard "Update"
// button. AMP performs the update as a background task and returns a RunningTask
// object; this returns the raw response once the task is accepted. Like
// setConfig, a stale cached session triggers one re-login and retry.
func (c *ampAPIClient) updateApplication() (string, error) {
	resp, err := c.postUpdate()
	if err == nil || !isSessionError(err) {
		return resp, err
	}
	// Session expired — force re-login and retry once.
	c.sessionID = ""
	if _, lerr := c.login(); lerr != nil {
		return "", lerr
	}
	return c.postUpdate()
}

// postUpdate performs one Core/UpdateApplication call against the current
// session and interprets the response. An empty body (void return on some AMP
// builds), a bare {}, or a RunningTask object is success; a {"Status":false}
// ActionResult is surfaced as an error.
func (c *ampAPIClient) postUpdate() (string, error) {
	sid, err := c.ensureSession()
	if err != nil {
		return "", err
	}
	resp, err := c.post("Core/UpdateApplication", map[string]any{"SESSIONID": sid})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(resp) == "" {
		return resp, nil
	}
	if err := parseActionResult("UpdateApplication", "", resp); err != nil {
		return "", err
	}
	return resp, nil
}

// runningTaskCount returns how many tasks AMP reports running on the instance
// (Core/GetStatus → RunningTasks). The post-update watcher polls this to wait out
// a SteamCMD update before restarting the container. Re-logs in once on session
// expiry, like the other calls.
func (c *ampAPIClient) runningTaskCount() (int, error) {
	n, err := c.getStatusRunningTasks()
	if err == nil || !isSessionError(err) {
		return n, err
	}
	c.sessionID = ""
	if _, lerr := c.login(); lerr != nil {
		return 0, lerr
	}
	return c.getStatusRunningTasks()
}

// getStatusRunningTasks performs one Core/GetStatus call and returns the length
// of its RunningTasks array.
func (c *ampAPIClient) getStatusRunningTasks() (int, error) {
	sid, err := c.ensureSession()
	if err != nil {
		return 0, err
	}
	resp, err := c.post("Core/GetStatus", map[string]any{"SESSIONID": sid})
	if err != nil {
		return 0, err
	}
	var result struct {
		RunningTasks []json.RawMessage `json:"RunningTasks"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(resp)), &result); err != nil {
		return 0, fmt.Errorf("amp api GetStatus: decode response: %w (output: %s)", err, resp)
	}
	return len(result.RunningTasks), nil
}

// getConfig reads a single AMP config node's current value.
func (c *ampAPIClient) getConfig(node string) (string, error) {
	sid, err := c.ensureSession()
	if err != nil {
		return "", err
	}
	resp, err := c.post("Core/GetConfig", map[string]any{
		"node":      node,
		"SESSIONID": sid,
	})
	if err != nil {
		return "", err
	}
	var result struct {
		CurrentValue json.RawMessage `json:"CurrentValue"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(resp)), &result); err != nil {
		return "", fmt.Errorf("amp api GetConfig %s: decode response: %w (output: %s)", node, err, resp)
	}
	return jsonScalarToString(result.CurrentValue), nil
}

// parseActionResult interprets an AMP action response (SetConfig,
// UpdateApplication, ...), which is either an ActionResult object
// ({"Status":bool,"Reason":string}) or — on some AMP versions — a bare JSON
// bool. A missing Status is treated as success (older builds return {} when
// the write succeeds).
//
// action names the caller's operation for the error text (e.g. "SetConfig",
// "UpdateApplication"); node is an optional secondary identifier (the config
// node path for SetConfig) appended when non-empty. Every call site must pass
// its own action — reusing another caller's label produces a misleading error
// (e.g. an UpdateApplication rejection must not say "SetConfig").
func parseActionResult(action, node, resp string) error {
	label := action
	if node != "" {
		label = action + " " + node
	}
	trimmed := strings.TrimSpace(resp)
	switch trimmed {
	case "true":
		return nil
	case "false":
		return fmt.Errorf("amp api %s: rejected", label)
	}
	var result struct {
		Status *bool  `json:"Status"`
		Reason string `json:"Reason"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(trimmed)), &result); err != nil {
		return fmt.Errorf("amp api %s: decode response: %w (output: %s)", label, err, trimmed)
	}
	if result.Status != nil && !*result.Status {
		reason := result.Reason
		if reason == "" {
			reason = "rejected"
		}
		return fmt.Errorf("amp api %s: %s", label, reason)
	}
	return nil
}

// extractJSONObject returns the substring spanning the first '{' to the last
// '}', so a stray sudo banner or curl notice ahead of the JSON body doesn't
// break decoding. Returns s unchanged when no object braces are present (the
// caller's decode then fails with a clear error).
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end < start {
		return s
	}
	return s[start : end+1]
}

// jsonScalarToString renders a JSON scalar (string/number/bool/null) as a plain
// string: quoted strings are unquoted; numbers and bools are returned verbatim;
// null/empty become "".
func jsonScalarToString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	if len(s) >= 2 && s[0] == '"' {
		var unquoted string
		if err := json.Unmarshal(raw, &unquoted); err == nil {
			return unquoted
		}
	}
	return s
}
