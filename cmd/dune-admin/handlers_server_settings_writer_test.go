package main

import (
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSplitCuratedFromINI partitions updates into curated AMP settings
// (FieldName→value, clears resolved to schema default) and everything else
// (section→key→value for the INI path). Non-curated keys must NOT be dropped —
// they route to the INI files so operators can still set custom settings under
// AMP.
func TestSplitCuratedFromINI(t *testing.T) {
	t.Parallel()
	updates := map[string]map[string]string{
		secConsoleVars:                         {"Dune.GlobalMiningOutputMultiplier": "3.000000"},
		secBuilding:                            {"m_MaxNumLandclaimSegments": ""},                  // curated clear → default
		"/Script/DuneSandbox.SandwormSettings": {"ThreatScale": "2.0", "InitialThreatRate": "0.5"}, // non-curated → INI
	}
	fields, ini := splitCuratedFromINI(updates)

	if got := fields["ConsoleVariables.Dune.GlobalMiningOutputMultiplier"]; got != "3.000000" {
		t.Errorf("mining FieldName value = %q, want 3.000000", got)
	}
	if got := fields["/Script/DuneSandbox.BuildingSettings.m_MaxNumLandclaimSegments"]; got != "6" {
		t.Errorf("cleared landclaim = %q, want default 6", got)
	}
	if len(fields) != 2 {
		t.Errorf("expected 2 curated fields, got %d: %v", len(fields), fields)
	}
	sw := ini["/Script/DuneSandbox.SandwormSettings"]
	if sw == nil || sw["ThreatScale"] != "2.0" || sw["InitialThreatRate"] != "0.5" {
		t.Errorf("non-curated sandworm settings must route to INI updates, got %v", ini)
	}
	if _, leaked := ini[secConsoleVars]; leaked {
		t.Error("curated CVar must not appear in INI updates")
	}
}

// fullRecExec is an Executor that routes Exec through a func and records whether
// WriteFile was called — enough to drive the handler's INI write path in tests.
type fullRecExec struct {
	execFn func(string) (string, error)
	writes int
}

func (e *fullRecExec) Exec(cmd string) (string, error)              { return e.execFn(cmd) }
func (e *fullRecExec) Stream(string) (<-chan string, func(), error) { return nil, func() {}, nil }
func (e *fullRecExec) PipeToWriter(string, io.Writer) error         { return nil }
func (e *fullRecExec) WriteFile(string, io.Reader) error            { e.writes++; return nil }
func (e *fullRecExec) Dial(network, addr string) (net.Conn, error)  { return nil, nil }
func (e *fullRecExec) Close()                                       {}
func (e *fullRecExec) Type() string                                 { return "local" }

// TestHandleUpdateServerSettings_AMPRoutesCuratedToAPI verifies a curated
// setting is written through the AMP API (and no INI write happens for it).
func TestHandleUpdateServerSettings_AMPRoutesCuratedToAPI(t *testing.T) {
	origControl, origExec := globalControl, globalExecutor
	t.Cleanup(func() { globalControl, globalExecutor = origControl, origExec })

	cap := &ampSettingsCapture{loginOK: true}
	exec := newAmpSettingsExec(t, cap)
	globalExecutor = exec
	globalControl = ampSettingsControl()

	body := `{"updates":[{"section":"ConsoleVariables","key":"Dune.GlobalMiningOutputMultiplier","value":"3.0"}]}`
	req := httptest.NewRequest("PUT", "/api/v1/server-settings", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handleUpdateServerSettings(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if cap.setCmds != 1 {
		t.Errorf("expected 1 SetConfig via AMP API, got %d", cap.setCmds)
	}
	if got := cap.nodes["Meta.GenericModule.ConsoleVariables.Dune.GlobalMiningOutputMultiplier"]; got != "3.000000" {
		t.Errorf("AMP node value = %q, want normalized 3.000000", got)
	}
}

// TestHandleUpdateServerSettings_AMPNonCuratedGoesToINI verifies a setting with
// no AMP node (not in the curated schema) is written to the INI files under AMP
// rather than rejected — the fix for the "not configurable on the amp control
// plane" regression. The AMP API must NOT be contacted for it.
func TestHandleUpdateServerSettings_AMPNonCuratedGoesToINI(t *testing.T) {
	origControl, origExec := globalControl, globalExecutor
	t.Cleanup(func() { globalControl, globalExecutor = origControl, origExec })

	sawAPI := false
	exec := &fullRecExec{execFn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "Core/Login") || strings.Contains(cmd, "Core/SetConfig") {
			sawAPI = true
		}
		return "", nil // empty INI reads + "no" probes
	}}
	globalExecutor = exec
	// iniDir set so DiscoverIniDir resolves without an instance.
	globalControl = &ampControl{
		useContainer: true, container: "AMP_X", ampUser: "amp", containerRuntime: "docker",
		apiUser: "admin", apiPass: "pw", iniDir: "/srv/state",
	}

	body := `{"updates":[{"section":"/Script/DuneSandbox.SandwormSettings","key":"ThreatScale","value":"2.0"}]}`
	req := httptest.NewRequest("PUT", "/api/v1/server-settings", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handleUpdateServerSettings(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (non-curated routes to INI, not 400); body=%s", rec.Code, rec.Body.String())
	}
	if sawAPI {
		t.Error("non-curated setting must NOT hit the AMP API")
	}
	if exec.writes == 0 {
		t.Error("non-curated setting should be written to an INI file")
	}
}
