package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withNilGlobalDB runs fn with globalDB forced nil, restoring it afterwards.
func withNilGlobalDB(t *testing.T, fn func()) {
	t.Helper()
	orig := globalDB
	t.Cleanup(func() { globalDB = orig })
	globalDB = nil
	fn()
}

func TestHandleGetIntel_BadID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/players/not-a-number/intel", nil)
	r.SetPathValue("id", "not-a-number")
	w := httptest.NewRecorder()
	handleGetIntel(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleGetIntel_NotConnected(t *testing.T) {
	withNilGlobalDB(t, func() {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/players/42/intel", nil)
		r.SetPathValue("id", "42")
		w := httptest.NewRecorder()
		handleGetIntel(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

func TestHandleSetIntel_Validation(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing player_id", `{"amount": 100}`},
		{"zero player_id", `{"player_id": 0, "amount": 100}`},
		{"negative amount", `{"player_id": 42, "amount": -1}`},
		{"amount above cap", `{"player_id": 42, "amount": 2780}`},
		{"malformed json", `{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/players/set-intel", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			handleSetIntel(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", w.Code)
			}
		})
	}
}

func TestHandleSetIntel_NotConnected(t *testing.T) {
	withNilGlobalDB(t, func() {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/players/set-intel",
			strings.NewReader(`{"player_id": 42, "amount": 100}`))
		w := httptest.NewRecorder()
		handleSetIntel(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

func TestHandleIntelAudit_NotConnected(t *testing.T) {
	withNilGlobalDB(t, func() {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/players/intel-audit", nil)
		w := httptest.NewRecorder()
		handleIntelAudit(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}
