package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoveredWebInterfaces_NilControlReturnsNil(t *testing.T) {
	t.Parallel()
	if got := discoveredWebInterfaces(context.Background(), nil, &localExecutor{}); got != nil {
		t.Errorf("nil control: got %v, want nil", got)
	}
}

func TestDiscoveredWebInterfaces_NilExecutorReturnsNil(t *testing.T) {
	t.Parallel()
	ctrl := &statusFakeControl{}
	if got := discoveredWebInterfaces(context.Background(), ctrl, nil); got != nil {
		t.Errorf("nil executor: got %v, want nil", got)
	}
}

func TestHandleGetWebInterfaces_NilControlNoDiscovered(t *testing.T) {
	orig := globalControl
	origExec := globalExecutor
	globalControl = nil
	globalExecutor = nil
	t.Cleanup(func() {
		globalControl = orig
		globalExecutor = origExec
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/web-interfaces", nil)
	rr := httptest.NewRecorder()
	handleGetWebInterfaces(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}
