package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleDiagnosticsEnvironment(t *testing.T) {
	origCfg := loadedConfig
	loadedConfig = appConfig{Control: "docker"}
	t.Cleanup(func() { loadedConfig = origCfg })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/environment", nil)
	handleDiagnosticsEnvironment(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var env environmentSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.ControlPlane != "docker" {
		t.Errorf("ControlPlane = %q, want docker", env.ControlPlane)
	}
}

func TestHandleDiagnosticsReport(t *testing.T) {
	globalLogRing = newLogRing(10)
	_, _ = globalLogRing.WriteLevel(0, []byte("dialing 192.168.0.59:8080\n"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/report", nil)
	handleDiagnosticsReport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var out struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.Body, "192.168.0.59") {
		t.Fatalf("report leaked host: %s", out.Body)
	}
}

func TestHandleDiagnosticsBundle(t *testing.T) {
	globalLogRing = newLogRing(10)
	_, _ = globalLogRing.WriteLevel(0, []byte("ok\n"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/bundle", nil)
	handleDiagnosticsBundle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "diagnostics.zip") {
		t.Errorf("Content-Disposition = %q", cd)
	}
}
