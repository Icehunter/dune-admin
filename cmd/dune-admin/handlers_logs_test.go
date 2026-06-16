package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleListLogSources_NilControl_Returns503(t *testing.T) {
	orig := globalControl
	globalControl = nil
	t.Cleanup(func() { globalControl = orig })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	rr := httptest.NewRecorder()
	handleLogPods(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

func TestHandleListLogSources_CtxControlOverridesGlobal(t *testing.T) {
	orig := globalControl
	globalControl = nil
	t.Cleanup(func() { globalControl = orig })

	ctrl := &statusFakeControl{}
	reg := newServerRegistry(nil)
	sc := &ServerContext{ID: "s1", StoreScope: defaultServerID, Control: ctrl}
	reg.Register(sc)

	inner := http.HandlerFunc(handleLogPods)
	h := serverSelectorMiddleware(reg, inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	req.Header.Set("X-Dune-Server", "s1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code == http.StatusServiceUnavailable {
		t.Error("control from ctx should prevent the 503 guard")
	}
}
