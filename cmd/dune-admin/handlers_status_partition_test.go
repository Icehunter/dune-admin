package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The Battlegroup tab decided whether to render the per-map restart action by
// comparing the control-plane NAME against a hardcoded "kubectl". docker grew
// RestartPartition in #311, but the button stayed hidden — the reporter
// verified every other part of the fix and then noted the single-map restart
// was still missing on their install.
//
// The backend already knows the answer: handleBGRestartPartition type-asserts
// the active plane for partitionRestarter. Reporting that assertion is what
// stops the frontend from having to re-derive it, and is why adding a plane
// won't reopen this bug.
func TestHandleStatus_ReportsPartitionRestartCapability(t *testing.T) {
	// Not parallel: handleStatus reads globalControl + loadedConfig globals.
	prevCtrl, prevCfg, prevReg := globalControl, loadedConfig, globalRegistry
	t.Cleanup(func() { globalControl = prevCtrl; loadedConfig = prevCfg; globalRegistry = prevReg })
	loadedConfig = appConfig{}
	globalRegistry = newServerRegistry(nil)

	statusField := func(t *testing.T) bool {
		t.Helper()
		rec := httptest.NewRecorder()
		handleStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status code = %d, want 200", rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		got, ok := body["supports_partition_restart"]
		if !ok {
			t.Fatal("status payload is missing supports_partition_restart")
		}
		b, ok := got.(bool)
		if !ok {
			t.Fatalf("supports_partition_restart = %T, want bool", got)
		}
		return b
	}

	cases := []struct {
		name string
		ctrl ControlPlane
		want bool
	}{
		// docker restarts the one container serving the partition (#311).
		{"docker", &dockerControl{}, true},
		// kubectl uses Funcom's ServerRestart CRD.
		{"kubectl", &kubectlControl{}, true},
		// AMP runs every partition inside one container: no narrower unit.
		{"amp", &ampControl{}, false},
		{"local", &localControl{}, false},
		// Before connect there is no plane to ask.
		{"disconnected", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			globalControl = tc.ctrl
			if got := statusField(t); got != tc.want {
				t.Fatalf("supports_partition_restart = %v, want %v", got, tc.want)
			}
		})
	}
}

// The capability reported to the UI must be the same assertion the restart
// handler enforces. If these ever drift, the button either 404s or hides a
// working feature — which is exactly the bug this replaced.
func TestPartitionRestartCapability_MatchesHandlerAssertion(t *testing.T) {
	t.Parallel()
	for _, ctrl := range []ControlPlane{
		&dockerControl{}, &kubectlControl{}, &ampControl{}, &localControl{},
	} {
		_, handlerAllows := ctrl.(partitionRestarter)
		if got := supportsPartitionRestart(ctrl); got != handlerAllows {
			t.Fatalf("%s: supportsPartitionRestart = %v, handler assertion = %v",
				ctrl.Name(), got, handlerAllows)
		}
	}
}
