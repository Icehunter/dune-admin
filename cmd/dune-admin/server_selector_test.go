package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// probeHandler records the ServerContext stashed in the request context.
type probeHandler struct {
	gotCtx *ServerContext
	called bool
}

func (p *probeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.called = true
	p.gotCtx = serverFromCtx(r)
	w.WriteHeader(http.StatusOK)
}

// ── serverSelectorMiddleware ────────────────────────────────────────────────

func TestServerSelectorMiddleware_KnownHeader(t *testing.T) {
	t.Parallel()
	reg := newServerRegistry(nil)
	sc := &ServerContext{ID: "srv-a", Name: "A", StoreScope: 1}
	reg.Register(sc)

	probe := &probeHandler{}
	h := serverSelectorMiddleware(reg, probe)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Dune-Server", "srv-a")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !probe.called {
		t.Fatal("inner handler not called")
	}
	if probe.gotCtx == nil || probe.gotCtx.ID != "srv-a" {
		t.Errorf("serverFromCtx ID = %v, want srv-a", probe.gotCtx)
	}
}

func TestServerSelectorMiddleware_UnknownHeader_Returns404(t *testing.T) {
	t.Parallel()
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "srv-a", StoreScope: 1})

	probe := &probeHandler{}
	h := serverSelectorMiddleware(reg, probe)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Dune-Server", "does-not-exist")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	if probe.called {
		t.Error("inner handler must not be called when server not found")
	}
}

func TestServerSelectorMiddleware_NoHeader_FallsBackToActive(t *testing.T) {
	t.Parallel()
	reg := newServerRegistry(nil)
	scA := &ServerContext{ID: "srv-a", StoreScope: 1}
	scB := &ServerContext{ID: "srv-b", StoreScope: 1}
	reg.Register(scA) // first registered → active
	reg.Register(scB)

	probe := &probeHandler{}
	h := serverSelectorMiddleware(reg, probe)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !probe.called {
		t.Fatal("inner handler not called")
	}
	if probe.gotCtx == nil || probe.gotCtx.ID != "srv-a" {
		t.Errorf("serverFromCtx ID = %v, want srv-a (the active server)", probe.gotCtx)
	}
}

func TestServerSelectorMiddleware_EmptyRegistry_ServesWithNilCtx(t *testing.T) {
	t.Parallel()
	reg := newServerRegistry(nil)

	probe := &probeHandler{}
	h := serverSelectorMiddleware(reg, probe)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !probe.called {
		t.Fatal("inner handler not called")
	}
	// serverFromCtx returns nil for an empty registry — handlers guard globalDB.
	if probe.gotCtx != nil {
		t.Errorf("serverFromCtx = %v, want nil for empty registry", probe.gotCtx)
	}
}

// ── context accessors ───────────────────────────────────────────────────────

func TestControlFromCtx_ReturnsControlFromStashedContext(t *testing.T) {
	t.Parallel()
	ctrl := &statusFakeControl{}
	reg := newServerRegistry(nil)
	sc := &ServerContext{ID: "srv-c", StoreScope: 1, Control: ctrl}
	reg.Register(sc)

	var gotCtrl ControlPlane
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCtrl = controlFromCtx(r)
		w.WriteHeader(http.StatusOK)
	})
	h := serverSelectorMiddleware(reg, inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Dune-Server", "srv-c")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if gotCtrl != ctrl {
		t.Errorf("controlFromCtx = %v, want %v", gotCtrl, ctrl)
	}
}

func TestControlFromCtx_NoCtx_FallsBackToGlobal(t *testing.T) {
	orig := globalControl
	t.Cleanup(func() { globalControl = orig })
	globalControl = &statusFakeControl{}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := controlFromCtx(req); got != globalControl {
		t.Errorf("controlFromCtx with no stash should return globalControl, got %v", got)
	}
}

func TestExecutorFromCtx_ReturnsExecutorFromStashedContext(t *testing.T) {
	t.Parallel()
	exec := &localExecutor{}
	reg := newServerRegistry(nil)
	sc := &ServerContext{ID: "srv-e", StoreScope: 1, Executor: exec}
	reg.Register(sc)

	var gotExec Executor
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotExec = executorFromCtx(r)
		w.WriteHeader(http.StatusOK)
	})
	h := serverSelectorMiddleware(reg, inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Dune-Server", "srv-e")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if gotExec != exec {
		t.Errorf("executorFromCtx = %v, want %v", gotExec, exec)
	}
}

func TestExecutorFromCtx_NoCtx_FallsBackToGlobal(t *testing.T) {
	orig := globalExecutor
	t.Cleanup(func() { globalExecutor = orig })
	globalExecutor = &localExecutor{}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := executorFromCtx(req); got != globalExecutor {
		t.Errorf("executorFromCtx with no stash should return globalExecutor, got %v", got)
	}
}

func TestDBFromCtx_ReturnsPoolFromStashedContext(t *testing.T) {
	t.Parallel()
	reg := newServerRegistry(nil)
	sc := &ServerContext{ID: "srv-a", StoreScope: 1, DB: nil} // nil pool is valid
	reg.Register(sc)

	var gotPool *pgxpool.Pool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPool = dbFromCtx(r)
		w.WriteHeader(http.StatusOK)
	})
	h := serverSelectorMiddleware(reg, inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Dune-Server", "srv-a")
	h.ServeHTTP(httptest.NewRecorder(), req)

	// sc.DB is nil; dbFromCtx must return a nil pool.
	if gotPool != nil {
		t.Errorf("dbFromCtx = %v, want nil (sc.DB is nil)", gotPool)
	}
}

func TestStoreScopeFromCtx_ReturnsServerStoreScope(t *testing.T) {
	t.Parallel()
	reg := newServerRegistry(nil)
	sc := &ServerContext{ID: "srv-x", StoreScope: 7}
	reg.Register(sc)

	var gotScope int
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotScope = storeScopeFromCtx(r)
		w.WriteHeader(http.StatusOK)
	})
	h := serverSelectorMiddleware(reg, inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Dune-Server", "srv-x")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if gotScope != 7 {
		t.Errorf("storeScopeFromCtx = %d, want 7", gotScope)
	}
}

func TestStoreScopeFromCtx_NoCtx_ReturnsDefault(t *testing.T) {
	t.Parallel()
	// Call accessor on a bare request (no middleware stash) — must return the
	// default server id so legacy handlers that call storeScopeFromCtx without the
	// middleware are safe.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := storeScopeFromCtx(req); got != defaultServerID {
		t.Errorf("storeScopeFromCtx with no stash = %d, want %d", got, defaultServerID)
	}
}

// ── GET /api/v1/servers ─────────────────────────────────────────────────────

func TestHandleListServers_SingleServer(t *testing.T) {
	origReg := globalRegistry
	t.Cleanup(func() { globalRegistry = origReg })

	globalRegistry = newServerRegistry(nil)
	globalRegistry.Register(&ServerContext{ID: "default", Name: "Default", StoreScope: 1})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers", nil)
	rr := httptest.NewRecorder()
	handleListServers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if body == "" || body == "null" {
		t.Error("expected non-empty JSON array")
	}
}

func TestHandleListServers_MultipleServers(t *testing.T) {
	origReg := globalRegistry
	t.Cleanup(func() { globalRegistry = origReg })

	globalRegistry = newServerRegistry(nil)
	globalRegistry.Register(&ServerContext{ID: "alpha", Name: "Alpha", StoreScope: 1})
	globalRegistry.Register(&ServerContext{ID: "beta", Name: "Beta", StoreScope: 1})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers", nil)
	rr := httptest.NewRecorder()
	handleListServers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

// ── PUT /api/v1/servers/active ──────────────────────────────────────────────

func TestHandleSetActiveServer_Valid(t *testing.T) {
	origReg := globalRegistry
	t.Cleanup(func() { globalRegistry = origReg })

	globalRegistry = newServerRegistry(nil)
	globalRegistry.Register(&ServerContext{ID: "1", Name: "Alpha", StoreScope: 1})
	globalRegistry.Register(&ServerContext{ID: "2", Name: "Beta", StoreScope: 1})

	body := `{"id":2}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/servers/active",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handleSetActiveServer(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rr.Code, rr.Body.String())
	}
	if globalRegistry.ActiveID() != "2" {
		t.Errorf("ActiveID = %q, want %q", globalRegistry.ActiveID(), "2")
	}
}

func TestHandleSetActiveServer_UnknownID_Returns404(t *testing.T) {
	origReg := globalRegistry
	t.Cleanup(func() { globalRegistry = origReg })

	globalRegistry = newServerRegistry(nil)
	globalRegistry.Register(&ServerContext{ID: "1", Name: "Alpha", StoreScope: 1})

	body := `{"id":999}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/servers/active",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handleSetActiveServer(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleSetActiveServer_MissingID_Returns400(t *testing.T) {
	origReg := globalRegistry
	t.Cleanup(func() { globalRegistry = origReg })

	globalRegistry = newServerRegistry(nil)

	body := `{}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/servers/active",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handleSetActiveServer(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}
