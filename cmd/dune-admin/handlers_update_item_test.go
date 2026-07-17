package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleUpdateItem_Validation covers the input-validation paths shipped
// untested in v0.45.0 (#256): id parsing, body decoding, and the stack/quality
// bounds — all of which must reject BEFORE touching the DB (nil pool here, so
// reaching the cmd would 500 instead of 400).
func TestHandleUpdateItem_Validation(t *testing.T) {
	tests := []struct {
		name       string
		pathID     string
		body       string
		wantStatus int
		wantInBody string
	}{
		{
			name:   "non-numeric id",
			pathID: "abc", body: `{"stack_size":1,"quality":0}`,
			wantStatus: http.StatusBadRequest, wantInBody: "invalid id",
		},
		{
			name:   "bad json",
			pathID: "5", body: `{bad`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "zero stack size",
			pathID: "5", body: `{"stack_size":0,"quality":0}`,
			wantStatus: http.StatusBadRequest, wantInBody: "stack_size",
		},
		{
			name:   "negative stack size",
			pathID: "5", body: `{"stack_size":-3,"quality":0}`,
			wantStatus: http.StatusBadRequest, wantInBody: "stack_size",
		},
		{
			name:   "negative quality",
			pathID: "5", body: `{"stack_size":1,"quality":-1}`,
			wantStatus: http.StatusBadRequest, wantInBody: "quality",
		},
		{
			name:   "missing body fields default to zero stack and reject",
			pathID: "5", body: `{}`,
			wantStatus: http.StatusBadRequest, wantInBody: "stack_size",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/v1/players/item/"+tt.pathID, strings.NewReader(tt.body))
			req.SetPathValue("id", tt.pathID)
			rec := httptest.NewRecorder()
			handleUpdateItem(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("code = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantInBody != "" && !strings.Contains(rec.Body.String(), tt.wantInBody) {
				t.Fatalf("body %q missing %q", rec.Body.String(), tt.wantInBody)
			}
		})
	}
}

// TestHandleUpdateItem_NilDB verifies a valid request with no DB returns 500,
// proving validation passed and the failure is the connection, not the input.
func TestHandleUpdateItem_NilDB(t *testing.T) {
	// NOT parallel — reads globalDB package global (nil in tests).
	req := httptest.NewRequest(http.MethodPut, "/api/v1/players/item/5",
		strings.NewReader(`{"stack_size":10,"quality":3}`))
	req.SetPathValue("id", "5")
	rec := httptest.NewRecorder()
	handleUpdateItem(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500 (nil DB)", rec.Code)
	}
}
