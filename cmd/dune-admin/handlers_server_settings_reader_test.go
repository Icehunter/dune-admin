package main

import "testing"

func TestOverlayAMPSettings(t *testing.T) {
	t.Parallel()
	settings := []ServerSetting{
		{
			Key: "GlobalMiningOutputMultiplier", Default: "1.0", Current: "1.0",
			FieldName: "ConsoleVariables.Dune.GlobalMiningOutputMultiplier",
			Source:    "defaultEngine",
			Layers:    []SettingLayer{{Source: "defaultEngine", Value: "1.0"}},
		},
		{Key: "ServerName", Default: "", Current: "OldName", FieldName: "WorldTitle"},
		{Key: "SomeIniThing", Default: "x", Current: "x"}, // no FieldName — untouched
	}
	amp := map[string]string{
		"ConsoleVariables.Dune.GlobalMiningOutputMultiplier": "5.0",
		"WorldTitle":    "My Server",
		"NotInSettings": "ignored",
	}
	out := overlayAMPSettings(settings, amp)

	// Mining: AMP value wins, overridden (5.0 != 1.0), amp source + top layer.
	if out[0].Current != "5.0" || !out[0].IsOverride || out[0].Source != ampSettingsSource {
		t.Fatalf("mining overlay = %+v", out[0])
	}
	last := out[0].Layers[len(out[0].Layers)-1]
	if last != (SettingLayer{Source: ampSettingsSource, Value: "5.0"}) {
		t.Fatalf("mining top layer = %+v, want amp/5.0", last)
	}
	// ServerName: default "" so "My Server" is an override.
	if out[1].Current != "My Server" || !out[1].IsOverride || out[1].Source != ampSettingsSource {
		t.Fatalf("servername overlay = %+v", out[1])
	}
	// Non-curated (no FieldName): untouched.
	if out[2].Current != "x" || out[2].Source == ampSettingsSource {
		t.Fatalf("non-curated should be untouched: %+v", out[2])
	}
}

// When AMP holds the schema default, the setting is current=default and NOT
// marked overridden — and gets no amp layer, so the "modified" filter is honest.
func TestOverlayAMPSettings_DefaultValueNotOverridden(t *testing.T) {
	t.Parallel()
	settings := []ServerSetting{{
		Default: "1.0", Current: "9.0", // stale INI value
		FieldName: "X",
		Source:    "userEngine",
		Layers:    []SettingLayer{{Source: "userEngine", Value: "9.0"}},
	}}
	out := overlayAMPSettings(settings, map[string]string{"X": "1.0"})
	if out[0].Current != "1.0" {
		t.Fatalf("current = %q, want AMP's authoritative 1.0", out[0].Current)
	}
	if out[0].IsOverride {
		t.Fatal("value == default must not be flagged overridden")
	}
	for _, l := range out[0].Layers {
		if l.Source == ampSettingsSource {
			t.Fatalf("no amp layer expected when value == default: %+v", out[0].Layers)
		}
	}
}

func TestOverlayAMPSettings_EmptyMapIsNoOp(t *testing.T) {
	t.Parallel()
	settings := []ServerSetting{{Current: "a", Source: "userGame", FieldName: "X"}}
	out := overlayAMPSettings(settings, nil)
	if out[0].Current != "a" || out[0].Source != "userGame" {
		t.Fatalf("empty amp map must be a no-op: %+v", out[0])
	}
}

// AMP returns "" (null/unset) for fields it has never explicitly stored.
// The overlay must not clobber the INI-derived value in that case.
func TestOverlayAMPSettings_EmptyAMPValueKeepsINIValue(t *testing.T) {
	t.Parallel()
	settings := []ServerSetting{
		{
			Default: "True", Current: "True", FieldName: "SandwormEnabled",
			Source: "userEngine",
			Layers: []SettingLayer{{Source: "userEngine", Value: "True"}},
		},
		{
			Default: "1.0", Current: "2.0", FieldName: "MiningRate",
			Source: "userGame",
			Layers: []SettingLayer{{Source: "userGame", Value: "2.0"}},
		},
	}
	// AMP returns empty string for both — means "not configured in AMP".
	amp := map[string]string{
		"SandwormEnabled": "",
		"MiningRate":      "",
	}
	out := overlayAMPSettings(settings, amp)

	if out[0].Current != "True" || out[0].Source != "userEngine" || out[0].IsOverride {
		t.Fatalf("bool: empty AMP value must not override INI value: %+v", out[0])
	}
	if out[1].Current != "2.0" || out[1].Source != "userGame" {
		t.Fatalf("float: empty AMP value must not override INI value: %+v", out[1])
	}
	for _, s := range out {
		for _, l := range s.Layers {
			if l.Source == ampSettingsSource {
				t.Fatalf("no amp layer expected when AMP value is empty: %+v", s.Layers)
			}
		}
	}
}

func TestCuratedFieldNamesFrom(t *testing.T) {
	t.Parallel()
	settings := []ServerSetting{
		{FieldName: "A"},
		{FieldName: ""}, // discovered/non-curated
		{FieldName: "B"},
	}
	got := curatedFieldNamesFrom(settings)
	if len(got) != 2 {
		t.Fatalf("want 2 curated field names, got %v", got)
	}
	seen := map[string]bool{}
	for _, f := range got {
		seen[f] = true
	}
	if !seen["A"] || !seen["B"] {
		t.Fatalf("missing expected field names: %v", got)
	}
}
