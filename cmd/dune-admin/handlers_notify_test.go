package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleNotify_NilExecutor_Returns503(t *testing.T) {
	orig := globalExecutor
	globalExecutor = nil
	t.Cleanup(func() { globalExecutor = orig })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notify",
		strings.NewReader(`{"content":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handleNotify(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

func TestHandleNotify_ExecutorFromCtx_Overrides503(t *testing.T) {
	// When a ServerContext with a non-nil Executor is stashed in the request
	// context, the handler must use that instead of globalExecutor. We expect
	// it NOT to return 503 (it will attempt AMQP publish and fail, returning 500).
	orig := globalExecutor
	globalExecutor = nil
	t.Cleanup(func() { globalExecutor = orig })

	exec := &localExecutor{}
	reg := newServerRegistry(nil)
	sc := &ServerContext{ID: "s1", StoreScope: defaultServerID, Executor: exec}
	reg.Register(sc)

	inner := http.HandlerFunc(handleNotify)
	h := serverSelectorMiddleware(reg, inner)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notify",
		strings.NewReader(`{"content":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dune-Server", "s1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code == http.StatusServiceUnavailable {
		t.Error("executor from ctx should prevent the 503 guard")
	}
}

func TestHandleNotify_MissingContent_Returns400(t *testing.T) {
	orig := globalExecutor
	globalExecutor = &localExecutor{}
	t.Cleanup(func() { globalExecutor = orig })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notify",
		strings.NewReader(`{"routing_key":"#"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handleNotify(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}
