package main

import (
	"context"
	"encoding/json"
	"errors"
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
	out          string
	err          error
}

func (p *partitionRestartControl) Name() string { return "kubectl" }

func (p *partitionRestartControl) RestartPartition(_ context.Context, _ Executor, partition int) (string, error) {
	p.gotPartition = partition
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

	rr := postRestartPartition(t, `{"partition":3}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if ctrl.gotPartition != 3 {
		t.Fatalf("gotPartition = %d, want 3", ctrl.gotPartition)
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
