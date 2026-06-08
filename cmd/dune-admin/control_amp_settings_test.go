package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ampSettingsExec routes Core/Login and Core/SetConfig, recording each
// SetConfig's decoded node→value and counting logins.
type ampSettingsCapture struct {
	logins  int
	setCmds int
	nodes   map[string]string
	loginOK bool
	setResp string
	setErr  error
}

func newAmpSettingsExec(t *testing.T, cap *ampSettingsCapture) *fnExecutor {
	cap.nodes = map[string]string{}
	return &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "Core/Login"):
			cap.logins++
			if !cap.loginOK {
				return `{"success":false,"resultReason":"bad creds"}`, nil
			}
			return `{"success":true,"sessionID":"sess"}`, nil
		case strings.Contains(cmd, "Core/SetConfig"):
			cap.setCmds++
			var p struct {
				Node  string `json:"node"`
				Value string `json:"value"`
			}
			decodePipedPayload(t, cmd, &p)
			cap.nodes[p.Node] = p.Value
			resp := cap.setResp
			if resp == "" {
				resp = `{"Status":true}`
			}
			return resp, cap.setErr
		default:
			t.Fatalf("unexpected AMP API endpoint in cmd: %q", cmd)
			return "", nil
		}
	}}
}

func ampSettingsControl() *ampControl {
	return &ampControl{
		useContainer:     true,
		container:        "AMP_X",
		ampUser:          "amp",
		containerRuntime: "docker",
		apiUser:          "admin",
		apiPass:          "pw",
	}
}

func TestAmpWriteServerSettings_LoginOnceThenSetConfigPerField(t *testing.T) {
	t.Parallel()
	cap := &ampSettingsCapture{loginOK: true}
	exec := newAmpSettingsExec(t, cap)

	err := ampSettingsControl().writeServerSettings(context.Background(), exec, map[string]string{
		"ConsoleVariables.Dune.GlobalMiningOutputMultiplier":             "3.000000",
		"/Script/DuneSandbox.BuildingSettings.m_MaxNumLandclaimSegments": "6",
	})
	if err != nil {
		t.Fatalf("writeServerSettings: %v", err)
	}
	if cap.logins != 1 {
		t.Errorf("logins = %d, want 1 (session must be reused across fields)", cap.logins)
	}
	if cap.setCmds != 2 {
		t.Errorf("SetConfig calls = %d, want 2", cap.setCmds)
	}
	// Node = Meta.GenericModule.<FieldName> verbatim (the proven AMP write path).
	if got := cap.nodes["Meta.GenericModule.ConsoleVariables.Dune.GlobalMiningOutputMultiplier"]; got != "3.000000" {
		t.Errorf("mining node value = %q, want 3.000000 (nodes: %v)", got, cap.nodes)
	}
	if got := cap.nodes["Meta.GenericModule./Script/DuneSandbox.BuildingSettings.m_MaxNumLandclaimSegments"]; got != "6" {
		t.Errorf("landclaim node value = %q, want 6", got)
	}
}

func TestAmpWriteServerSettings_WrapsAPICallForContainer(t *testing.T) {
	t.Parallel()
	var loginCmd string
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "Core/Login") {
			loginCmd = cmd
			return `{"success":true,"sessionID":"s"}`, nil
		}
		return `{"Status":true}`, nil
	}}
	err := ampSettingsControl().writeServerSettings(context.Background(), exec,
		map[string]string{"ConsoleVariables.Sandstorm.Enabled": "True"})
	if err != nil {
		t.Fatalf("writeServerSettings: %v", err)
	}
	if !strings.Contains(loginCmd, "docker exec AMP_X") {
		t.Errorf("AMP API call must be wrapped for in-container exec, got: %q", loginCmd)
	}
	if !strings.Contains(loginCmd, "http://127.0.0.1:8081/API/Core/Login") {
		t.Errorf("AMP API call must hit the loopback ADS API, got: %q", loginCmd)
	}
}

func TestAmpWriteServerSettings_EmptyUpdatesIsNoOp(t *testing.T) {
	t.Parallel()
	cap := &ampSettingsCapture{loginOK: true}
	exec := newAmpSettingsExec(t, cap)
	if err := ampSettingsControl().writeServerSettings(context.Background(), exec, map[string]string{}); err != nil {
		t.Fatalf("writeServerSettings: %v", err)
	}
	if cap.logins != 0 || cap.setCmds != 0 {
		t.Errorf("expected no API calls for empty updates, got logins=%d set=%d", cap.logins, cap.setCmds)
	}
}

func TestAmpWriteServerSettings_MissingCredentialsErrors(t *testing.T) {
	t.Parallel()
	called := false
	exec := &fnExecutor{fn: func(string) (string, error) { called = true; return "", nil }}
	c := &ampControl{useContainer: true, container: "AMP_X", ampUser: "amp", containerRuntime: "docker"} // no apiUser/apiPass
	err := c.writeServerSettings(context.Background(), exec,
		map[string]string{"ConsoleVariables.Sandstorm.Enabled": "True"})
	if err == nil {
		t.Fatal("expected error when AMP API credentials are not configured")
	}
	if called {
		t.Error("must not contact the AMP API without credentials")
	}
}

func TestAmpWriteServerSettings_SetConfigFailurePropagates(t *testing.T) {
	t.Parallel()
	cap := &ampSettingsCapture{loginOK: true, setResp: `{"Status":false,"Reason":"No such node."}`}
	exec := newAmpSettingsExec(t, cap)
	err := ampSettingsControl().writeServerSettings(context.Background(), exec,
		map[string]string{"ConsoleVariables.Bogus": "1"})
	if err == nil {
		t.Fatal("expected SetConfig failure to propagate")
	}
	if !strings.Contains(err.Error(), "No such node") {
		t.Errorf("error should surface AMP reason, got: %v", err)
	}
}

func TestAmpWriteServerSettings_LoginFailurePropagates(t *testing.T) {
	t.Parallel()
	cap := &ampSettingsCapture{loginOK: false}
	exec := newAmpSettingsExec(t, cap)
	err := ampSettingsControl().writeServerSettings(context.Background(), exec,
		map[string]string{"ConsoleVariables.Sandstorm.Enabled": "True"})
	if err == nil {
		t.Fatal("expected login failure to abort the write")
	}
	if cap.setCmds != 0 {
		t.Error("must not SetConfig when login fails")
	}
}

// TestAmpControl_ImplementsServerSettingsWriter is a compile-time guard that
// ampControl satisfies the optional interface the handler routes on.
func TestAmpControl_ImplementsServerSettingsWriter(t *testing.T) {
	t.Parallel()
	var _ serverSettingsWriter = (*ampControl)(nil)
	// Sanity: a transport error from the executor is wrapped, not swallowed.
	exec := &fnExecutor{fn: func(string) (string, error) { return "", errors.New("boom") }}
	err := ampSettingsControl().writeServerSettings(context.Background(), exec,
		map[string]string{"ConsoleVariables.Sandstorm.Enabled": "True"})
	if err == nil {
		t.Fatal("expected executor error to propagate")
	}
}
