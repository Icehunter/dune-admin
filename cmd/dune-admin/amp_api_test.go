package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// identityWrap is a no-op container wrapper so tests inspect the exact curl
// command the AMP API client builds, without the sudo/exec envelope.
func identityWrap(s string) string { return s }

// decodePipedPayload extracts the base64 blob from an `echo <b64> | base64 -d |
// curl ...` command and unmarshals the decoded JSON into out. The API client
// base64-pipes request bodies so operator-supplied values (passwords, names)
// never need shell escaping; tests assert on the decoded payload rather than on
// brittle string formatting.
func decodePipedPayload(t *testing.T, cmd string, out any) {
	t.Helper()
	// The payload rides as `echo <b64> | base64 -d | curl …`. Locate that segment
	// whether the command is bare (identity wrap) or wrapped for in-container
	// exec (`sudo … sh -c 'echo <b64> | …'`). The base64 token has no spaces or
	// quotes, so the field after "echo " is the payload in both forms.
	const marker = "echo "
	i := strings.Index(cmd, marker)
	if i < 0 {
		t.Fatalf("command has no `echo <payload>` segment: %q", cmd)
	}
	b64 := strings.Fields(cmd[i+len(marker):])[0]
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("payload is not valid base64 (%v) in cmd: %q", err, cmd)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decoded payload is not valid JSON (%v): %s", err, raw)
	}
}

// ── login ───────────────────────────────────────────────────────────────────

func TestAMPAPILogin_BuildsRequestAndReturnsSession(t *testing.T) {
	t.Parallel()
	var gotCmd string
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		gotCmd = cmd
		return `{"success":true,"resultReason":"","sessionID":"abc-123"}`, nil
	}}
	c := newAMPAPIClient(exec, identityWrap, "admin", "s3cr3t!", 0)

	sid, err := c.login()
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sid != "abc-123" {
		t.Errorf("sessionID = %q, want abc-123", sid)
	}

	// Endpoint + default port 8081 when port is 0.
	if !strings.Contains(gotCmd, "http://127.0.0.1:8081/API/Core/Login") {
		t.Errorf("missing Login endpoint with default port in cmd: %q", gotCmd)
	}
	// JSON is base64-piped, not inlined, and posted as the request body.
	for _, want := range []string{"base64 -d", "--data-binary @-", "-H 'Content-Type: application/json'", "-H 'Accept: application/json'"} {
		if !strings.Contains(gotCmd, want) {
			t.Errorf("cmd missing %q: %q", want, gotCmd)
		}
	}
	// Operator credentials, including the special-char password, ride in the
	// decoded payload — never on the shell command line.
	if strings.Contains(gotCmd, "s3cr3t!") {
		t.Errorf("password leaked onto the command line: %q", gotCmd)
	}
	var payload struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		Token      string `json:"token"`
		RememberMe bool   `json:"rememberMe"`
	}
	decodePipedPayload(t, gotCmd, &payload)
	if payload.Username != "admin" || payload.Password != "s3cr3t!" {
		t.Errorf("login payload creds = %+v, want admin/s3cr3t!", payload)
	}
	if payload.Token != "" || payload.RememberMe {
		t.Errorf("login payload token/rememberMe = %q/%v, want empty/false", payload.Token, payload.RememberMe)
	}
}

func TestAMPAPILogin_HonoursConfiguredPort(t *testing.T) {
	t.Parallel()
	var gotCmd string
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		gotCmd = cmd
		return `{"success":true,"sessionID":"x"}`, nil
	}}
	c := newAMPAPIClient(exec, identityWrap, "u", "p", 9999)
	if _, err := c.login(); err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.Contains(gotCmd, "http://127.0.0.1:9999/API/Core/Login") {
		t.Errorf("expected configured port 9999 in endpoint: %q", gotCmd)
	}
}

func TestAMPAPILogin_FailedAuthIsError(t *testing.T) {
	t.Parallel()
	exec := &fnExecutor{fn: func(string) (string, error) {
		return `{"success":false,"resultReason":"Invalid username or password.","sessionID":""}`, nil
	}}
	c := newAMPAPIClient(exec, identityWrap, "admin", "wrong", 8081)
	_, err := c.login()
	if err == nil {
		t.Fatal("expected error on failed auth")
	}
	if !strings.Contains(err.Error(), "Invalid username or password") {
		t.Errorf("error should surface the AMP reason, got: %v", err)
	}
}

func TestAMPAPILogin_ExecErrorIsWrapped(t *testing.T) {
	t.Parallel()
	exec := &fnExecutor{fn: func(string) (string, error) {
		return "curl: (7) Failed to connect", errors.New("exit status 7")
	}}
	c := newAMPAPIClient(exec, identityWrap, "admin", "pw", 8081)
	if _, err := c.login(); err == nil {
		t.Fatal("expected error when exec fails")
	}
}

func TestAMPAPILogin_GarbageResponseIsError(t *testing.T) {
	t.Parallel()
	exec := &fnExecutor{fn: func(string) (string, error) { return "not json at all", nil }}
	c := newAMPAPIClient(exec, identityWrap, "admin", "pw", 8081)
	if _, err := c.login(); err == nil {
		t.Fatal("expected error on non-JSON response")
	}
}

// ── setConfig ────────────────────────────────────────────────────────────────

func TestAMPAPISetConfig_LogsInThenSetsAndReusesSession(t *testing.T) {
	t.Parallel()
	var loginCalls, setCalls int
	var setCmd string
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "Core/Login"):
			loginCalls++
			return `{"success":true,"sessionID":"sess-9"}`, nil
		case strings.Contains(cmd, "Core/SetConfig"):
			setCalls++
			setCmd = cmd
			return `{"Status":true,"Reason":""}`, nil
		default:
			t.Fatalf("unexpected endpoint in cmd: %q", cmd)
			return "", nil
		}
	}}
	c := newAMPAPIClient(exec, identityWrap, "admin", "pw", 8081)

	node := "Meta.GenericModule.ConsoleVariables.Dune.GlobalMiningOutputMultiplier"
	if err := c.setConfig(node, "3.0"); err != nil {
		t.Fatalf("first setConfig: %v", err)
	}
	if err := c.setConfig("Meta.GenericModule.WorldTitle", "My Sietch's Server"); err != nil {
		t.Fatalf("second setConfig: %v", err)
	}

	if loginCalls != 1 {
		t.Errorf("login called %d times, want 1 (session must be cached)", loginCalls)
	}
	if setCalls != 2 {
		t.Errorf("setConfig issued %d POSTs, want 2", setCalls)
	}
	if !strings.Contains(setCmd, "/API/Core/SetConfig") {
		t.Errorf("missing SetConfig endpoint: %q", setCmd)
	}
	var payload struct {
		Node      string `json:"node"`
		Value     string `json:"value"`
		SessionID string `json:"SESSIONID"`
	}
	decodePipedPayload(t, setCmd, &payload)
	if payload.Node != "Meta.GenericModule.WorldTitle" {
		t.Errorf("node = %q, want Meta.GenericModule.WorldTitle", payload.Node)
	}
	if payload.Value != "My Sietch's Server" {
		t.Errorf("value = %q, want the quote-containing title verbatim", payload.Value)
	}
	if payload.SessionID != "sess-9" {
		t.Errorf("SESSIONID = %q, want sess-9", payload.SessionID)
	}
}

func TestAMPAPISetConfig_StatusFalseIsError(t *testing.T) {
	t.Parallel()
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "Core/Login") {
			return `{"success":true,"sessionID":"s"}`, nil
		}
		return `{"Status":false,"Reason":"No such node."}`, nil
	}}
	c := newAMPAPIClient(exec, identityWrap, "admin", "pw", 8081)
	err := c.setConfig("Meta.GenericModule.Nope", "1")
	if err == nil {
		t.Fatal("expected error when Status is false")
	}
	if !strings.Contains(err.Error(), "No such node") {
		t.Errorf("error should surface AMP reason, got: %v", err)
	}
}

func TestAMPAPISetConfig_AcceptsBareBoolResult(t *testing.T) {
	t.Parallel()
	// Some AMP versions return a bare `true` from SetConfig rather than an
	// ActionResult object.
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "Core/Login") {
			return `{"success":true,"sessionID":"s"}`, nil
		}
		return `true`, nil
	}}
	c := newAMPAPIClient(exec, identityWrap, "admin", "pw", 8081)
	if err := c.setConfig("Meta.GenericModule.X", "1"); err != nil {
		t.Errorf("bare true should be success, got: %v", err)
	}
}

func TestAMPAPISetConfig_LoginFailureAborts(t *testing.T) {
	t.Parallel()
	setReached := false
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "Core/Login") {
			return `{"success":false,"resultReason":"locked"}`, nil
		}
		setReached = true
		return `{"Status":true}`, nil
	}}
	c := newAMPAPIClient(exec, identityWrap, "admin", "pw", 8081)
	if err := c.setConfig("Meta.GenericModule.X", "1"); err == nil {
		t.Fatal("expected error when login fails")
	}
	if setReached {
		t.Error("setConfig must not POST when login fails")
	}
}

// ── getConfig ────────────────────────────────────────────────────────────────

func TestAMPAPIGetConfig_ReturnsCurrentValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		resp string
		want string
	}{
		{"string value", `{"CurrentValue":"3.000000","Node":"x"}`, "3.000000"},
		{"numeric value", `{"CurrentValue":42}`, "42"},
		{"bool value", `{"CurrentValue":true}`, "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var getCmd string
			exec := &fnExecutor{fn: func(cmd string) (string, error) {
				if strings.Contains(cmd, "Core/Login") {
					return `{"success":true,"sessionID":"s"}`, nil
				}
				getCmd = cmd
				return tt.resp, nil
			}}
			c := newAMPAPIClient(exec, identityWrap, "admin", "pw", 8081)
			got, err := c.getConfig("Meta.GenericModule.X")
			if err != nil {
				t.Fatalf("getConfig: %v", err)
			}
			if got != tt.want {
				t.Errorf("CurrentValue = %q, want %q", got, tt.want)
			}
			if !strings.Contains(getCmd, "/API/Core/GetConfig") {
				t.Errorf("missing GetConfig endpoint: %q", getCmd)
			}
			var payload struct {
				Node      string `json:"node"`
				SessionID string `json:"SESSIONID"`
			}
			decodePipedPayload(t, getCmd, &payload)
			if payload.Node != "Meta.GenericModule.X" || payload.SessionID != "s" {
				t.Errorf("getConfig payload = %+v, want node X + session s", payload)
			}
		})
	}
}
