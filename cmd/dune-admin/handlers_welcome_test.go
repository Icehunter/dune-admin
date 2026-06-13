package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// setupWelcomeStore wires a fresh in-memory store into welcomeStoreDB and
// restores nil on cleanup. NOT parallel — mutates a package global.
func setupWelcomeStore(t *testing.T) *welcomeStore {
	t.Helper()
	s := openMemWelcomeStore(t)
	welcomeStoreDB = s
	t.Cleanup(func() { welcomeStoreDB = nil })
	return s
}

// TestHandleOverrideWelcomeGrant_Validation covers the request-validation and
// runtime-lookup branches of the override handler that do not reach the DB.
func TestHandleOverrideWelcomeGrant_Validation(t *testing.T) {
	setupWelcomeStore(t)
	setWelcomeRuntime(buildWelcomeRuntime(true, []string{"v1"}, 30,
		[]welcomePackage{{Version: "v1", Items: []welcomePackageItem{{Template: "PlantFiber", Qty: 1}}}},
		welcomeMessageOptions{}))
	t.Cleanup(func() { setWelcomeRuntime(welcomePackageRuntime{}) })

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"bad json", `{`, http.StatusBadRequest},
		{"missing account", `{"package_version":"v1"}`, http.StatusBadRequest},
		{"missing version", `{"account_id":5}`, http.StatusBadRequest},
		{"unknown package", `{"account_id":5,"package_version":"nope"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/welcome-package/override", bytes.NewReader([]byte(tt.body)))
			rec := httptest.NewRecorder()
			handleOverrideWelcomeGrant(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

// TestHandleOverrideWelcomeGrant_NoDB returns 503 when no DB is connected even
// for an otherwise-valid request (globalDB is nil in unit tests).
func TestHandleOverrideWelcomeGrant_NoDB(t *testing.T) {
	setupWelcomeStore(t)
	setWelcomeRuntime(buildWelcomeRuntime(true, []string{"v1"}, 30,
		[]welcomePackage{{Version: "v1", Items: []welcomePackageItem{{Template: "PlantFiber", Qty: 1}}}},
		welcomeMessageOptions{}))
	t.Cleanup(func() { setWelcomeRuntime(welcomePackageRuntime{}) })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/welcome-package/override",
		bytes.NewReader([]byte(`{"account_id":5,"package_version":"v1"}`)))
	rec := httptest.NewRecorder()
	handleOverrideWelcomeGrant(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHandlePutWelcomeConfig_MessageFieldsPersisted verifies that the welcome
// message fields survive a PUT → GET round-trip via the SQLite store.
// This is a regression test for the bug where the handler built the
// welcomeConfigRow without WelcomeMessage* fields, so they were lost on refresh.
func TestHandlePutWelcomeConfig_MessageFieldsPersisted(t *testing.T) {
	setupWelcomeStore(t)

	payload := welcomeConfigResponse{
		Enabled:                    false,
		ScanIntervalSecs:           30,
		ActiveVersion:              "",
		Packages:                   []welcomePackage{},
		WelcomeMessageEnabled:      true,
		WelcomeMessage:             "Welcome to the server! Enjoy your starter pack.",
		WelcomeWhisperSourcePlayer: "fls-id-abc123",
	}
	body, _ := json.Marshal(payload)

	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/welcome-package/config", bytes.NewReader(body))
	putRec := httptest.NewRecorder()
	handlePutWelcomeConfig(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT: want 200, got %d: %s", putRec.Code, putRec.Body.String())
	}

	// Simulate a UI refresh: GET re-reads the store.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/welcome-package/config", nil)
	getRec := httptest.NewRecorder()
	handleGetWelcomeConfig(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET: want 200, got %d: %s", getRec.Code, getRec.Body.String())
	}

	var got welcomeConfigResponse
	if err := json.NewDecoder(getRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if !got.WelcomeMessageEnabled {
		t.Error("WelcomeMessageEnabled: want true, got false after refresh")
	}
	if got.WelcomeMessage != payload.WelcomeMessage {
		t.Errorf("WelcomeMessage: want %q, got %q", payload.WelcomeMessage, got.WelcomeMessage)
	}
	if got.WelcomeWhisperSourcePlayer != payload.WelcomeWhisperSourcePlayer {
		t.Errorf("WelcomeWhisperSourcePlayer: want %q, got %q", payload.WelcomeWhisperSourcePlayer, got.WelcomeWhisperSourcePlayer)
	}
}

// TestHandlePutWelcomeConfig_MultipleActiveVersions verifies that multiple active
// versions survive a PUT → GET round-trip and that active_version (compat field)
// is set to the first element.
func TestHandlePutWelcomeConfig_MultipleActiveVersions(t *testing.T) {
	setupWelcomeStore(t)

	payload := welcomeConfigResponse{
		Enabled:          false,
		ScanIntervalSecs: 30,
		ActiveVersions:   []string{"v1", "v2"},
		ActiveVersion:    "v1",
		Packages: []welcomePackage{
			{Version: "v1", Items: []welcomePackageItem{}},
			{Version: "v2", Items: []welcomePackageItem{}},
		},
	}
	body, _ := json.Marshal(payload)

	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/welcome-package/config", bytes.NewReader(body))
	putRec := httptest.NewRecorder()
	handlePutWelcomeConfig(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT: want 200, got %d: %s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/welcome-package/config", nil)
	getRec := httptest.NewRecorder()
	handleGetWelcomeConfig(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET: want 200, got %d: %s", getRec.Code, getRec.Body.String())
	}

	var got welcomeConfigResponse
	if err := json.NewDecoder(getRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.ActiveVersions) != 2 || got.ActiveVersions[0] != "v1" || got.ActiveVersions[1] != "v2" {
		t.Fatalf("ActiveVersions: want [v1 v2], got %v", got.ActiveVersions)
	}
	if got.ActiveVersion != "v1" {
		t.Fatalf("ActiveVersion compat: want v1, got %q", got.ActiveVersion)
	}
}

// TestHandlePutWelcomeConfig_CompatSingleActiveVersion verifies that a PUT with
// only the legacy active_version field (no active_versions) is promoted to a
// single-element slice on GET.
func TestHandlePutWelcomeConfig_CompatSingleActiveVersion(t *testing.T) {
	setupWelcomeStore(t)

	payload := welcomeConfigResponse{
		Enabled:          false,
		ScanIntervalSecs: 30,
		ActiveVersion:    "v1",
		// ActiveVersions intentionally omitted (zero value = nil)
		Packages: []welcomePackage{
			{Version: "v1", Items: []welcomePackageItem{}},
		},
	}
	body, _ := json.Marshal(payload)

	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/welcome-package/config", bytes.NewReader(body))
	putRec := httptest.NewRecorder()
	handlePutWelcomeConfig(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT: want 200, got %d: %s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/welcome-package/config", nil)
	getRec := httptest.NewRecorder()
	handleGetWelcomeConfig(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET: want 200, got %d: %s", getRec.Code, getRec.Body.String())
	}

	var got welcomeConfigResponse
	if err := json.NewDecoder(getRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.ActiveVersions) != 1 || got.ActiveVersions[0] != "v1" {
		t.Fatalf("compat: want [v1], got %v", got.ActiveVersions)
	}
}

func TestBuildWelcomeRuntime(t *testing.T) {
	t.Parallel()
	pkgs := []welcomePackage{{Version: "v1"}, {Version: "v2"}}
	tests := []struct {
		name         string
		enabled      bool
		active       []string
		scanSecs     int
		packages     []welcomePackage
		wantActive   string
		wantInterval time.Duration
	}{
		{"defaults active to first package", true, nil, 0, pkgs, "v1", welcomeDefaultScanInterval},
		{"unknown active falls back to first", true, []string{"vX"}, 0, pkgs, "v1", welcomeDefaultScanInterval},
		{"explicit active respected", true, []string{"v2"}, 120, pkgs, "v2", 120 * time.Second},
		{"interval below floor is clamped", false, []string{"v1"}, 1, pkgs, "v1", welcomeDefaultScanInterval},
		{"min interval honored", true, []string{"v2"}, 5, pkgs, "v2", 5 * time.Second},
		{"no packages → empty active", true, nil, 60, nil, "", 60 * time.Second},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := buildWelcomeRuntime(tt.enabled, tt.active, tt.scanSecs, tt.packages, welcomeMessageOptions{})
			if rt.enabled != tt.enabled {
				t.Fatalf("enabled: want %v, got %v", tt.enabled, rt.enabled)
			}
			firstActive := ""
			if len(rt.activeVersions) > 0 {
				firstActive = rt.activeVersions[0]
			}
			if firstActive != tt.wantActive {
				t.Fatalf("activeVersion: want %q, got %q", tt.wantActive, firstActive)
			}
			if rt.interval != tt.wantInterval {
				t.Fatalf("interval: want %v, got %v", tt.wantInterval, rt.interval)
			}
		})
	}
}

func TestWelcomeRuntimeActive(t *testing.T) {
	t.Parallel()
	rt := buildWelcomeRuntime(true, []string{"v2"}, 30, []welcomePackage{
		{Version: "v1", Items: []welcomePackageItem{{Template: "A", Qty: 1}}},
		{Version: "v2", Items: []welcomePackageItem{{Template: "B", Qty: 2}}},
	}, welcomeMessageOptions{})
	p, ok := rt.active()
	if !ok {
		t.Fatal("expected an active package")
	}
	if p.Version != "v2" || len(p.Items) != 1 || p.Items[0].Template != "B" {
		t.Fatalf("active package wrong: %+v", p)
	}

	empty := buildWelcomeRuntime(true, nil, 30, nil, welcomeMessageOptions{})
	if _, ok := empty.active(); ok {
		t.Fatal("expected no active package when library is empty")
	}
}
