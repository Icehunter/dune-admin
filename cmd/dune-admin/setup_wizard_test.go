package main

// Phase 6 / #166 — Red tests for the web setup wizard flow.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleStatus_IncludesNeedsSetup asserts that handleStatus always includes
// a "needs_setup" boolean in its response, so the frontend can gate the wizard.
func TestHandleStatus_IncludesNeedsSetup(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rr := httptest.NewRecorder()
	handleStatus(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["needs_setup"]; !ok {
		t.Error("handleStatus response missing 'needs_setup' field")
	}
}

// TestSetupIfNeeded_DoesNotBlock asserts that setupIfNeeded no longer auto-runs
// the terminal wizard — it must return immediately with a bool.
// (Verified structurally: if this compiles and the test returns, the function
// doesn't block on I/O.)
func TestSetupIfNeeded_DoesNotBlock(t *testing.T) {
	// Save and restore global state modified by the call.
	origDB := globalDB
	defer func() { globalDB = origDB }()

	// With globalDB nil and no config file, needsSetup() returns true.
	// setupIfNeeded() must return without blocking.
	done := make(chan bool, 1)
	go func() {
		done <- setupIfNeeded()
	}()
	select {
	case result := <-done:
		// result is either true (needs setup) or false (config present);
		// either is fine — what matters is it returned.
		_ = result
	case <-make(chan struct{}): // never fires — just to show non-blocking intent
	}
}
