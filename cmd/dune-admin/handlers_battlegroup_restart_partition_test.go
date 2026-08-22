package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// partitionRestartControl is a fake ControlPlane implementing partitionRestarter,
// standing in for kubectl. It records the partition it was asked to restart.
type partitionRestartControl struct {
	stubControlPlane
	gotPartition int
	gotMap       string
	out          string
	err          error
}

func (p *partitionRestartControl) Name() string { return "kubectl" }

func (p *partitionRestartControl) RestartPartition(_ context.Context, _ Executor, target restartTarget) (string, error) {
	p.gotPartition = target.Partition
	p.gotMap = target.Map
	return p.out, p.err
}

// nonPartitionRestartControl is a fake ControlPlane that does NOT implement
// partitionRestarter, standing in for AMP/docker/local.
type nonPartitionRestartControl struct {
	stubControlPlane
}

func (n *nonPartitionRestartControl) Name() string { return "amp" }

func saveRestartPartitionGlobals(t *testing.T) {
	t.Helper()
	prevC, prevE := globalControl, globalExecutor
	t.Cleanup(func() { globalControl, globalExecutor = prevC, prevE })
	globalExecutor = &fnExecutor{fn: func(string) (string, error) { return "", nil }}
}

func postRestartPartition(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/battlegroup/restart-partition", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handleBGRestartPartition(rr, req)
	return rr
}

func TestHandleBGRestartPartition_NotConnected(t *testing.T) {
	saveRestartPartitionGlobals(t)
	globalControl = nil

	rr := postRestartPartition(t, `{"partition":1}`)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleBGRestartPartition_UnsupportedControlPlane(t *testing.T) {
	saveRestartPartitionGlobals(t)
	globalControl = &nonPartitionRestartControl{}

	rr := postRestartPartition(t, `{"partition":1}`)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotImplemented)
	}
}

func TestHandleBGRestartPartition_BadBody(t *testing.T) {
	saveRestartPartitionGlobals(t)
	globalControl = &partitionRestartControl{}

	rr := postRestartPartition(t, `not-json`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleBGRestartPartition_NegativePartition(t *testing.T) {
	saveRestartPartitionGlobals(t)
	ctrl := &partitionRestartControl{}
	globalControl = ctrl

	rr := postRestartPartition(t, `{"partition":-1}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if ctrl.gotPartition != 0 {
		t.Fatal("RestartPartition must not be called for an invalid partition")
	}
}

func TestHandleBGRestartPartition_Success(t *testing.T) {
	saveRestartPartitionGlobals(t)
	ctrl := &partitionRestartControl{out: "serverrestart.igw.funcom.com/dune-admin-restart-abc created"}
	globalControl = ctrl

	rr := postRestartPartition(t, `{"partition":3,"map":"overmap"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if ctrl.gotPartition != 3 {
		t.Fatalf("gotPartition = %d, want 3", ctrl.gotPartition)
	}
	// The map has to reach the plane: it is what disambiguates rows that all
	// report the same partition index (see restartTarget).
	if ctrl.gotMap != "overmap" {
		t.Fatalf("gotMap = %q, want %q", ctrl.gotMap, "overmap")
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(resp["output"], "created") {
		t.Errorf("output = %q, want it to contain the apply result", resp["output"])
	}
}

func TestHandleBGRestartPartition_ControlPlaneError(t *testing.T) {
	saveRestartPartitionGlobals(t)
	ctrl := &partitionRestartControl{err: errors.New("no ServerSet found for partition 5")}
	globalControl = ctrl

	rr := postRestartPartition(t, `{"partition":5}`)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// A restart target the operator can correct — a map that matches no container,
// or a partition several containers claim — is not a server fault. Returning
// 500 for all of them leaves the UI unable to tell "your click was stale,
// refresh" from "the restart itself broke".
func TestHandleBGRestartPartition_TargetErrorsMapToClientStatus(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"unknown target", fmt.Errorf("no container found for map %q: %w", "arrakeen", errRestartTargetUnknown), http.StatusNotFound},
		{"ambiguous target", fmt.Errorf("3 containers report partition 0: %w", errRestartTargetAmbiguous), http.StatusConflict},
		{"genuine failure", errors.New("docker daemon unreachable"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			saveRestartPartitionGlobals(t)
			globalControl = &partitionRestartControl{err: tc.err}

			rr := postRestartPartition(t, `{"partition":0,"map":"arrakeen"}`)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rr.Code, tc.wantStatus, rr.Body.String())
			}
		})
	}
}
