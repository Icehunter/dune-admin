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
