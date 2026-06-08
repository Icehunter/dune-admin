package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestResolveFieldUpdates maps normalized (section→key→value) updates to AMP
// FieldName→value: known keys resolve to their FieldName, an empty value
// resolves to the schema default (AMP has no "unset"), and (section,key) pairs
// outside the curated schema are reported as unknown.
func TestResolveFieldUpdates(t *testing.T) {
	t.Parallel()
	updates := map[string]map[string]string{
		secConsoleVars:         {"Dune.GlobalMiningOutputMultiplier": "3.000000"},
		secBuilding:            {"m_MaxNumLandclaimSegments": ""}, // clear → revert to default
		"/Script/Fake.Section": {"m_unknown": "1"},                // not curated → unknown
	}
	fields, unknown := resolveFieldUpdates(updates)

	if got := fields["ConsoleVariables.Dune.GlobalMiningOutputMultiplier"]; got != "3.000000" {
		t.Errorf("mining FieldName value = %q, want 3.000000", got)
	}
	// Cleared landclaim resolves to the curated default (6).
	if got := fields["/Script/DuneSandbox.BuildingSettings.m_MaxNumLandclaimSegments"]; got != "6" {
		t.Errorf("cleared landclaim value = %q, want default 6", got)
	}
	if len(unknown) != 1 || unknown[0] != "/Script/Fake.Section|m_unknown" {
		t.Errorf("unknown = %v, want [/Script/Fake.Section|m_unknown]", unknown)
	}
	if _, ok := fields["m_unknown"]; ok {
		t.Error("unknown key must not appear in field updates")
	}
}

// TestHandleUpdateServerSettings_AMPRoutesToAPI verifies that when the control
// plane is AMP (a serverSettingsWriter), settings are written through the AMP
// API rather than the INI files.
func TestHandleUpdateServerSettings_AMPRoutesToAPI(t *testing.T) {
	origControl, origExec := globalControl, globalExecutor
	t.Cleanup(func() { globalControl, globalExecutor = origControl, origExec })

	cap := &ampSettingsCapture{loginOK: true}
	exec := newAmpSettingsExec(t, cap)
	globalExecutor = exec
	globalControl = ampSettingsControl()

	body := `{"updates":[{"section":"ConsoleVariables","key":"Dune.GlobalMiningOutputMultiplier","value":"3.0"}]}`
	req := httptest.NewRequest("PUT", "/api/v1/server-settings", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleUpdateServerSettings(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if cap.setCmds != 1 {
		t.Errorf("expected 1 SetConfig via AMP API, got %d", cap.setCmds)
	}
	// Float values are normalized before the AMP write.
	if got := cap.nodes["Meta.GenericModule.ConsoleVariables.Dune.GlobalMiningOutputMultiplier"]; got != "3.000000" {
		t.Errorf("AMP node value = %q, want normalized 3.000000", got)
	}
}

// TestHandleUpdateServerSettings_AMPRejectsUncuratedKey verifies that a key with
// no AMP node (not in the curated schema) is rejected with 400 under AMP rather
// than silently written to an INI that AMP would clobber.
func TestHandleUpdateServerSettings_AMPRejectsUncuratedKey(t *testing.T) {
	origControl, origExec := globalControl, globalExecutor
	t.Cleanup(func() { globalControl, globalExecutor = origControl, origExec })

	cap := &ampSettingsCapture{loginOK: true}
	exec := newAmpSettingsExec(t, cap)
	globalExecutor = exec
	globalControl = ampSettingsControl()

	body := `{"updates":[{"section":"/Script/Fake.Section","key":"m_unknown","value":"1"}]}`
	req := httptest.NewRequest("PUT", "/api/v1/server-settings", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleUpdateServerSettings(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 for uncurated key under AMP; body=%s", rec.Code, rec.Body.String())
	}
	if cap.setCmds != 0 {
		t.Error("must not write anything when an uncurated key is present")
	}
}
