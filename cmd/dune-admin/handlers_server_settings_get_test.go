package main

import (
	"testing"
)

func TestServerSettingsSchemaKeys(t *testing.T) {
	keys := serverSettingsSchemaKeys()
	if len(keys) != len(serverSettingsSchema) {
		t.Fatalf("expected %d keys, got %d", len(serverSettingsSchema), len(keys))
	}
	first := serverSettingsSchema[0]
	if !keys[first.Section+"|"+first.Key] {
		t.Fatalf("missing schema key %s|%s", first.Section, first.Key)
	}
}

func TestApplySettingLayers(t *testing.T) {
	s := ServerSetting{
		Section: "Sec",
		Key:     "Key",
		Type:    string(settingInt),
		Current: "0",
	}
	sources := []layerSource{
		{name: "defaultGame", ini: map[string]map[string]string{"Sec": {"Key": "1"}}},
		{name: "userGame", ini: map[string]map[string]string{"Sec": {"Key": "2"}}},
	}

	applySettingLayers(&s, sources)

	if s.Current != "2" || s.Source != "userGame" {
		t.Fatalf("unexpected current/source: %q / %q", s.Current, s.Source)
	}
	if !s.IsOverride {
		t.Fatal("expected IsOverride=true")
	}
	if len(s.Layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(s.Layers))
	}
}

func TestDiscoverUnknownSettings(t *testing.T) {
	schemaKeys := map[string]bool{"Sec|Known": true}
	sources := []layerSource{
		{
			name: "defaultGame",
			ini: map[string]map[string]string{
				"Sec": {"Known": "1", "+Array": "x", "CustomB": "true"},
				"A":   {"CustomA": "42"},
			},
		},
		{
			name: "userGame",
			ini: map[string]map[string]string{
				"Sec": {"CustomB": "false"},
			},
		},
	}

	discovered := discoverUnknownSettings(sources, schemaKeys)
	if len(discovered) != 2 {
		t.Fatalf("expected 2 discovered keys, got %d", len(discovered))
	}
	if discovered[0].section != "A" || discovered[0].key != "CustomA" {
		t.Fatalf("unexpected first discovered key: %+v", discovered[0])
	}
	if discovered[1].section != "Sec" || discovered[1].key != "CustomB" {
		t.Fatalf("unexpected second discovered key: %+v", discovered[1])
	}
}

func TestBuildDiscoveredSettings(t *testing.T) {
	discovered := []discoveredKey{{section: "Sec", key: "Custom"}}
	schemaKeys := map[string]bool{}
	sources := []layerSource{
		{name: "defaultGame", ini: map[string]map[string]string{"Sec": {"Custom": "1"}}},
		{name: "userGame", ini: map[string]map[string]string{"Sec": {"Custom": "True"}}},
	}

	settings := buildDiscoveredSettings(discovered, sources, schemaKeys)
	if len(settings) != 1 {
		t.Fatalf("expected 1 discovered setting, got %d", len(settings))
	}
	if settings[0].Type != string(settingInt) {
		t.Fatalf("expected inferred type int from first layer, got %q", settings[0].Type)
	}
	if settings[0].Current != "True" || settings[0].Source != "userGame" {
		t.Fatalf("unexpected current/source: %q / %q", settings[0].Current, settings[0].Source)
	}
	if !settings[0].IsOverride {
		t.Fatal("expected IsOverride=true for user layer")
	}
	if !schemaKeys["Sec|Custom"] {
		t.Fatal("expected discovered key to be added to schema key set")
	}
}

func TestBuildServerSettingsRawSections(t *testing.T) {
	schemaKeys := map[string]bool{"Sec|Known": true}
	defaultGame := "[Sec]\nKnown=1\nOther=2\n+Array=3\n"
	defaultEngine := "[Sec]\n-Array=4\n"
	raw := buildServerSettingsRawSections(defaultGame, defaultEngine, "", "", schemaKeys)

	if len(raw) != 2 {
		t.Fatalf("expected 2 raw sections, got %d", len(raw))
	}
	if raw[0].Source != "defaultGame" || len(raw[0].Lines) != 2 {
		t.Fatalf("unexpected defaultGame raw section: %+v", raw[0])
	}
	if raw[1].Source != "defaultEngine" || len(raw[1].Lines) != 1 {
		t.Fatalf("unexpected defaultEngine raw section: %+v", raw[1])
	}
	if raw[0].Lines[0].Key != "Other" || raw[0].Lines[0].Prefix != "" {
		t.Fatalf("unexpected first defaultGame raw line: %+v", raw[0].Lines[0])
	}
	if raw[0].Lines[1].Key != "Array" || raw[0].Lines[1].Prefix != "+" {
		t.Fatalf("unexpected second defaultGame raw line: %+v", raw[0].Lines[1])
	}
	if raw[1].Lines[0].Key != "Array" || raw[1].Lines[0].Prefix != "-" {
		t.Fatalf("unexpected defaultEngine raw line: %+v", raw[1].Lines[0])
	}
}
