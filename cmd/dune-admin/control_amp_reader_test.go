package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// newAmpReadExec routes Core/Login and Core/GetConfig, returning canned
// CurrentValue responses keyed by the requested node, and counts logins.
func newAmpReadExec(t *testing.T, loginOK bool, values map[string]string, logins *int) *fnExecutor {
	return &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "Core/Login"):
			*logins++
			if !loginOK {
				return `{"success":false,"resultReason":"bad creds"}`, nil
			}
			return `{"success":true,"sessionID":"sess"}`, nil
		case strings.Contains(cmd, "Core/GetConfig"):
			var p struct {
				Node string `json:"node"`
			}
			decodePipedPayload(t, cmd, &p)
			b, _ := json.Marshal(values[p.Node])
			return `{"CurrentValue":` + string(b) + `}`, nil
		default:
			t.Fatalf("unexpected AMP API endpoint in cmd: %q", cmd)
			return "", nil
		}
	}}
}

func TestAmpReadServerSettings_LoginOnceThenGetPerField(t *testing.T) {
	t.Parallel()
	logins := 0
	values := map[string]string{
		"Meta.GenericModule.ConsoleVariables.Dune.GlobalMiningOutputMultiplier": "5.000000",
		"Meta.GenericModule.WorldTitle":                                         "My Sietch",
	}
	exec := newAmpReadExec(t, true, values, &logins)

	got, err := ampSettingsControl().readServerSettings(context.Background(), exec, []string{
		"ConsoleVariables.Dune.GlobalMiningOutputMultiplier",
		"WorldTitle",
	})
	if err != nil {
		t.Fatalf("readServerSettings: %v", err)
	}
	if logins != 1 {
		t.Errorf("logins = %d, want 1 (session reused across reads)", logins)
	}
	if got["ConsoleVariables.Dune.GlobalMiningOutputMultiplier"] != "5.000000" {
		t.Errorf("mining = %q, want 5.000000 (got: %v)", got["ConsoleVariables.Dune.GlobalMiningOutputMultiplier"], got)
	}
	if got["WorldTitle"] != "My Sietch" {
		t.Errorf("title = %q, want My Sietch", got["WorldTitle"])
	}
}

func TestAmpReadServerSettings_EmptyFieldsIsNoOp(t *testing.T) {
	t.Parallel()
	logins := 0
	exec := newAmpReadExec(t, true, nil, &logins)
	got, err := ampSettingsControl().readServerSettings(context.Background(), exec, nil)
	if err != nil {
		t.Fatalf("readServerSettings: %v", err)
	}
	if len(got) != 0 || logins != 0 {
		t.Errorf("empty fields must not contact AMP: got=%v logins=%d", got, logins)
	}
}

func TestAmpReadServerSettings_MissingCredentialsErrors(t *testing.T) {
	t.Parallel()
	called := false
	exec := &fnExecutor{fn: func(string) (string, error) { called = true; return "", nil }}
	c := &ampControl{useContainer: true, container: "AMP_X", ampUser: "amp", containerRuntime: "docker"} // no api creds
	_, err := c.readServerSettings(context.Background(), exec, []string{"WorldTitle"})
	if err == nil {
		t.Fatal("expected error when AMP API credentials are not configured")
	}
	if called {
		t.Error("must not contact the AMP API without credentials")
	}
}

func TestAmpReadServerSettings_GetConfigFailurePropagates(t *testing.T) {
	t.Parallel()
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "Core/Login") {
			return `{"success":true,"sessionID":"s"}`, nil
		}
		return "not json", nil // GetConfig garbage → decode error
	}}
	_, err := ampSettingsControl().readServerSettings(context.Background(), exec, []string{"WorldTitle"})
	if err == nil {
		t.Fatal("expected a GetConfig decode failure to propagate")
	}
}

// Compile-time guard that ampControl satisfies the optional reader interface the
// settings GET handler routes on.
func TestAmpControl_ImplementsServerSettingsReader(t *testing.T) {
	t.Parallel()
	var _ serverSettingsReader = (*ampControl)(nil)
}
