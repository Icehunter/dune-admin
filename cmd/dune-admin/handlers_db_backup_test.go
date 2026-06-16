package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleDBBackupRestore_RequiresConfirm verifies the destructive restore
// endpoint rejects a request without confirm=true before doing anything else.
func TestHandleDBBackupRestore_RequiresConfirm(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/db-backups/restore",
		strings.NewReader(`{"file":"dune-x.dump","confirm":false}`))
	handleDBBackupRestore(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("restore without confirm: code = %d, want 400", rec.Code)
	}
}

// TestHandleDBBackupCreate_NoControl verifies a 503 when no control plane is
// connected (globals nil).
func TestHandleDBBackupCreate_NoControl(t *testing.T) {
	prevC, prevE := globalControl, globalExecutor
	t.Cleanup(func() { globalControl, globalExecutor = prevC, prevE })
	globalControl, globalExecutor = nil, nil

	rec := httptest.NewRecorder()
	handleDBBackupCreate(rec, httptest.NewRequest(http.MethodPost, "/api/v1/db-backups", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("create with no control: code = %d, want 503", rec.Code)
	}
}

// TestHandleDBBackupDownload_BadName verifies path-traversal / bad names are rejected.
func TestHandleDBBackupDownload_BadName(t *testing.T) {
	rec := httptest.NewRecorder()
	handleDBBackupDownload(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/db-backups/download?file=../../etc/passwd", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("download traversal: code = %d, want 400", rec.Code)
	}
}

// TestDbBackupProviderOrErr_CtxControlOverridesGlobal verifies that a
// ServerContext stashed in the request context satisfies the control guard even
// when globalControl is nil.
func TestDbBackupProviderOrErr_CtxControlOverridesGlobal(t *testing.T) {
	prevC, prevE := globalControl, globalExecutor
	globalControl, globalExecutor = nil, nil
	t.Cleanup(func() { globalControl, globalExecutor = prevC, prevE })

	ctrl := &dbProviderControl{}
	exec := &fnExecutor{fn: func(string) (string, error) { return "", nil }}
	reg := newServerRegistry(nil)
	sc := &ServerContext{ID: "s1", StoreScope: defaultServerID, Control: ctrl, Executor: exec}
	reg.Register(sc)

	var capturedProv dbBackupProvider
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prov, ok := dbBackupProviderOrErr(w, r)
		if !ok {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		capturedProv = prov
		w.WriteHeader(http.StatusOK)
	})
	h := serverSelectorMiddleware(reg, inner)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/db-backups", nil)
	req.Header.Set("X-Dune-Server", "s1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if capturedProv == nil {
		t.Fatal("expected a dbBackupProvider to be resolved from ctx control")
	}
}

// TestGameServersRunning_ExplicitCtrl verifies the function uses the passed ctrl
// rather than globalControl.
func TestGameServersRunning_ExplicitCtrl(t *testing.T) {
	prevC, prevE := globalControl, globalExecutor
	globalControl, globalExecutor = nil, nil
	t.Cleanup(func() { globalControl, globalExecutor = prevC, prevE })

	ctrl := &recordingControl{}
	exec := &fnExecutor{fn: func(string) (string, error) { return "", nil }}

	running, err := gameServersRunning(t.Context(), ctrl, exec)
	if err != nil {
		t.Fatalf("gameServersRunning: %v", err)
	}
	if running {
		t.Error("expected false — recordingControl returns empty status")
	}
}
