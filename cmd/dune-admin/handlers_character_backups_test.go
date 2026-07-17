package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestCharacterBackupsStoreForCtx_HonoursRequestScope pins the property the
// delete-with-backup path relies on: the store used for a backup must be
// scoped to the request's server, not the process-default. cmdDeleteCharacter
// previously reached for the unscoped global here, which filed the safety
// backup under server 1 and hid it from the actual server's list/restore.
func TestCharacterBackupsStoreForCtx_HonoursRequestScope(t *testing.T) {
	setupCharacterBackupsStore(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/players/1/backup", nil)
	req = req.WithContext(context.WithValue(req.Context(), serverContextKey,
		&ServerContext{ID: "s2", StoreScope: 2}))

	store := characterBackupsStoreForCtx(req)
	if store == nil {
		t.Fatal("expected a scoped store")
	}
	if store.serverID != 2 {
		t.Fatalf("store.serverID = %d, want 2 (request scope)", store.serverID)
	}
}

// setupCharacterBackupsStore sets globalCharacterBackupsStore to a fresh
// in-memory store and restores nil on cleanup. NOT parallel — mutates package
// global.
func setupCharacterBackupsStore(t *testing.T) *characterBackupsStore {
	t.Helper()
	s := testCharacterBackupsStore(t)
	globalCharacterBackupsStore = s
	t.Cleanup(func() { globalCharacterBackupsStore = nil })
	return s
}

// ── handleBackupCharacter ────────────────────────────────────────────────────

func TestHandleBackupCharacter_InputValidation(t *testing.T) {
	tests := []struct {
		name       string
		pathID     string
		rawBody    []byte
		wantStatus int
	}{
		{"invalid id", "abc", []byte(`{}`), http.StatusBadRequest},
		{"bad json", "42", []byte(`{bad`), http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/players/"+tt.pathID+"/backup", bytes.NewReader(tt.rawBody))
			req.SetPathValue("id", tt.pathID)
			rec := httptest.NewRecorder()
			handleBackupCharacter(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("want %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestHandleBackupCharacter_NilDB verifies a valid request with no DB returns 500.
func TestHandleBackupCharacter_NilDB(t *testing.T) {
	// NOT parallel — reads globalDB package global (nil in tests).
	body, _ := json.Marshal(map[string]any{"character_name": "Paul", "reason": "manual"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/players/42/backup", bytes.NewReader(body))
	req.SetPathValue("id", "42")
	rec := httptest.NewRecorder()
	handleBackupCharacter(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 (nil DB), got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// ── handleListCharacterBackups ───────────────────────────────────────────────

func TestHandleListCharacterBackups_InvalidID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/players/abc/backups", nil)
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()
	handleListCharacterBackups(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandleListCharacterBackups_NilStore(t *testing.T) {
	globalCharacterBackupsStore = nil
	req := httptest.NewRequest(http.MethodGet, "/api/v1/players/42/backups", nil)
	req.SetPathValue("id", "42")
	rec := httptest.NewRecorder()
	handleListCharacterBackups(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandleListCharacterBackups_ReturnsAccountScopedRows(t *testing.T) {
	setupCharacterBackupsStore(t)
	if _, err := globalCharacterBackupsStore.create(characterBackup{AccountID: 42, Action: "manual", FilePath: "/a"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := globalCharacterBackupsStore.create(characterBackup{AccountID: 99, Action: "manual", FilePath: "/b"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/players/42/backups", nil)
	req.SetPathValue("id", "42")
	rec := httptest.NewRecorder()
	handleListCharacterBackups(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var got []characterBackup
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].AccountID != 42 {
		t.Fatalf("got %+v, want exactly one row for account 42", got)
	}
}

// ── handleRestoreCharacterBackup ─────────────────────────────────────────────

func TestHandleRestoreCharacterBackup_InputValidation(t *testing.T) {
	tests := []struct {
		name       string
		pathID     string
		rawBody    []byte
		wantStatus int
	}{
		{"invalid id", "abc", []byte(`{"confirm":true}`), http.StatusBadRequest},
		{"missing confirm", "1", []byte(`{}`), http.StatusBadRequest},
		{"confirm false", "1", []byte(`{"confirm":false}`), http.StatusBadRequest},
		{"bad json", "1", []byte(`{bad`), http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/character-backups/"+tt.pathID+"/restore", bytes.NewReader(tt.rawBody))
			req.SetPathValue("id", tt.pathID)
			rec := httptest.NewRecorder()
			handleRestoreCharacterBackup(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("want %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleRestoreCharacterBackup_NilStore(t *testing.T) {
	globalCharacterBackupsStore = nil
	body, _ := json.Marshal(map[string]any{"confirm": true})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/character-backups/1/restore", bytes.NewReader(body))
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handleRestoreCharacterBackup(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandleRestoreCharacterBackup_NilDB(t *testing.T) {
	setupCharacterBackupsStore(t)
	created, err := globalCharacterBackupsStore.create(characterBackup{AccountID: 42, Action: "manual", FilePath: "/a"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	body, _ := json.Marshal(map[string]any{"confirm": true})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/character-backups/%d/restore", created.ID), bytes.NewReader(body))
	req.SetPathValue("id", fmt.Sprintf("%d", created.ID))
	rec := httptest.NewRecorder()
	handleRestoreCharacterBackup(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 (nil DB), got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// ── handleDownloadCharacterBackup ────────────────────────────────────────────

func TestHandleDownloadCharacterBackup_InvalidID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/character-backups/abc/download", nil)
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()
	handleDownloadCharacterBackup(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandleDownloadCharacterBackup_NilStore(t *testing.T) {
	globalCharacterBackupsStore = nil
	req := httptest.NewRequest(http.MethodGet, "/api/v1/character-backups/1/download", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handleDownloadCharacterBackup(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandleDownloadCharacterBackup_NotFound(t *testing.T) {
	setupCharacterBackupsStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/character-backups/999/download", nil)
	req.SetPathValue("id", "999")
	rec := httptest.NewRecorder()
	handleDownloadCharacterBackup(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandleDownloadCharacterBackup_ServesFileContents(t *testing.T) {
	setupCharacterBackupsStore(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "42.json")
	if err := os.WriteFile(path, []byte(`{"_patches_checksum":"abc"}`), 0o640); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	created, err := globalCharacterBackupsStore.create(characterBackup{AccountID: 42, Action: "manual", FilePath: path})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/character-backups/%d/download", created.ID), nil)
	req.SetPathValue("id", fmt.Sprintf("%d", created.ID))
	rec := httptest.NewRecorder()
	handleDownloadCharacterBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"_patches_checksum":"abc"}` {
		t.Fatalf("body = %q, want the file contents", rec.Body.String())
	}
	if rec.Header().Get("Content-Disposition") == "" {
		t.Error("expected a Content-Disposition attachment header")
	}
}

// ── handleDeleteCharacterBackup ──────────────────────────────────────────────

func TestHandleDeleteCharacterBackup_InvalidID(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/character-backups/abc", nil)
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()
	handleDeleteCharacterBackup(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteCharacterBackup_NilStore(t *testing.T) {
	globalCharacterBackupsStore = nil
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/character-backups/1", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	handleDeleteCharacterBackup(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteCharacterBackup_NotFound(t *testing.T) {
	setupCharacterBackupsStore(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/character-backups/999", nil)
	req.SetPathValue("id", "999")
	rec := httptest.NewRecorder()
	handleDeleteCharacterBackup(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteCharacterBackup_RemovesRowAndFile(t *testing.T) {
	setupCharacterBackupsStore(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "42.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o640); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	created, err := globalCharacterBackupsStore.create(characterBackup{AccountID: 42, Action: "manual", FilePath: path})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/character-backups/%d", created.ID), nil)
	req.SetPathValue("id", fmt.Sprintf("%d", created.ID))
	rec := httptest.NewRecorder()
	handleDeleteCharacterBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if _, err := globalCharacterBackupsStore.get(created.ID); err != errNotFound {
		t.Fatalf("get after delete = %v, want errNotFound", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected backup file to be removed, stat err = %v", err)
	}
}
