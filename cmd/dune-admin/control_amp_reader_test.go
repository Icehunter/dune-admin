package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// newAmpReadExec routes Core/Login and Core/GetConfigs, returning a canned
// SettingSpec array (Node + CurrentValue) for the requested nodes, and counts
// logins.
func newAmpReadExec(t *testing.T, loginOK bool, values map[string]string, logins *int) *fnExecutor {
	return &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "Core/Login"):
			*logins++
			if !loginOK {
				return `{"success":false,"resultReason":"bad creds"}`, nil
			}
			return `{"success":true,"sessionID":"sess"}`, nil
		case strings.Contains(cmd, "Core/GetConfigs"):
			return ampGetConfigsResponse(t, cmd, values), nil
		default:
			t.Fatalf("unexpected AMP API endpoint in cmd: %q", cmd)
			return "", nil
		}
	}}
}

func TestAmpReadServerSettings_LoginOnceThenBatchRead(t *testing.T) {
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

// ampGetConfigsResponse decodes a Core/GetConfigs payload and answers it the
// way AMP does: a JSON array of SettingSpec objects in request order.
func ampGetConfigsResponse(t *testing.T, cmd string, values map[string]string) string {
	t.Helper()
	var p struct {
		Nodes []string `json:"nodes"`
	}
	decodePipedPayload(t, cmd, &p)
	specs := make([]map[string]any, 0, len(p.Nodes))
	for _, n := range p.Nodes {
		specs = append(specs, map[string]any{"Node": n, "CurrentValue": values[n]})
	}
	b, err := json.Marshal(specs)
	if err != nil {
		t.Fatalf("marshal specs: %v", err)
	}
	return string(b)
}

// Reading the whole curated schema one GetConfig at a time costs one remote
// exec per field (~60), so reads go out as a single Core/GetConfigs call.
func TestAmpReadServerSettings_BatchesIntoOneGetConfigs(t *testing.T) {
	t.Parallel()
	getConfigCalls := 0
	values := map[string]string{
		"Meta.GenericModule.WorldTitle": "My Sietch",
		"Meta.GenericModule./Script/DuneSandbox.UserServerCustomSettings.CraftingCost":               "2.000000",
		"Meta.GenericModule./Script/DuneSandbox.UserServerCustomSettings.bBuildingInfiniteStability": "True",
	}
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "Core/Login"):
			return `{"success":true,"sessionID":"sess"}`, nil
		case strings.Contains(cmd, "Core/GetConfig"):
			getConfigCalls++
			if !strings.Contains(cmd, "Core/GetConfigs") {
				t.Errorf("per-field Core/GetConfig call, want one batched Core/GetConfigs: %q", cmd)
			}
			return ampGetConfigsResponse(t, cmd, values), nil
		}
		t.Fatalf("unexpected cmd: %q", cmd)
		return "", nil
	}}
	got, err := ampSettingsControl().readServerSettings(context.Background(), exec, []string{
		"WorldTitle",
		"/Script/DuneSandbox.UserServerCustomSettings.CraftingCost",
		"/Script/DuneSandbox.UserServerCustomSettings.bBuildingInfiniteStability",
	})
	if err != nil {
		t.Fatalf("readServerSettings: %v", err)
	}
	if getConfigCalls != 1 {
		t.Errorf("GetConfig calls = %d, want 1", getConfigCalls)
	}
	if got["/Script/DuneSandbox.UserServerCustomSettings.CraftingCost"] != "2.000000" || got["WorldTitle"] != "My Sietch" ||
		got["/Script/DuneSandbox.UserServerCustomSettings.bBuildingInfiniteStability"] != "True" {
		t.Errorf("values = %v", got)
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

// An AMP instance on a Dune template older than 1.5 has no
// UserServerCustomSettings nodes, and GetConfigs fails the whole batch on any
// unknown node. The older curated settings must still read back.
func TestAmpReadServerSettings_RetriesWithoutCustomNodesOnBatchFailure(t *testing.T) {
	t.Parallel()
	calls := 0
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "Core/Login"):
			return `{"success":true,"sessionID":"sess"}`, nil
		case strings.Contains(cmd, "Core/GetConfigs"):
			calls++
			var p struct {
				Nodes []string `json:"nodes"`
			}
			decodePipedPayload(t, cmd, &p)
			for _, n := range p.Nodes {
				if strings.Contains(n, "UserServerCustomSettings") {
					return `{"Title":"ArgumentException","Message":"No such node"}`, nil
				}
			}
			return ampGetConfigsResponse(t, cmd, map[string]string{"Meta.GenericModule.WorldTitle": "Sietch"}), nil
		}
		t.Fatalf("unexpected cmd: %q", cmd)
		return "", nil
	}}
	got, err := ampSettingsControl().readServerSettings(context.Background(), exec, []string{
		"WorldTitle", "/Script/DuneSandbox.UserServerCustomSettings.CraftingCost",
	})
	if err != nil {
		t.Fatalf("readServerSettings: %v", err)
	}
	if got["WorldTitle"] != "Sietch" || calls != 2 {
		t.Errorf("got=%v calls=%d, want WorldTitle read back on the retry", got, calls)
	}
	if _, ok := got["/Script/DuneSandbox.UserServerCustomSettings.CraftingCost"]; ok {
		t.Error("custom node must be absent after the fallback")
	}
}
