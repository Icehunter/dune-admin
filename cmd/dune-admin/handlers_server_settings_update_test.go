package main

import "testing"

func TestBuildServerSettingsSchemaMap(t *testing.T) {
	t.Parallel()

	schemaMap := buildServerSettingsSchemaMap()
	if len(schemaMap) != len(serverSettingsSchema) {
		t.Fatalf("expected %d schema entries, got %d", len(serverSettingsSchema), len(schemaMap))
	}
	if _, ok := schemaMap[secBuilding+"|m_bBuildingRestrictionLimitsEnabled"]; !ok {
		t.Fatalf("expected known schema key for building restriction limits")
	}
}

func TestNormalizeServerSettingsUpdates(t *testing.T) {
	t.Parallel()

	schemaMap := buildServerSettingsSchemaMap()
	normalized, err := normalizeServerSettingsUpdates([]serverSettingUpdate{
		{Section: secBuilding, Key: "m_bBuildingRestrictionLimitsEnabled", Value: "true"},
		{Section: secBuilding, Key: "CustomKey", Value: "raw-value"},
		{Section: secBuilding, Key: "AnotherKey", Value: ""},
	}, schemaMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized.applied != 2 || normalized.cleared != 1 {
		t.Fatalf("unexpected counters: applied=%d cleared=%d", normalized.applied, normalized.cleared)
	}
	if got := normalized.updates[secBuilding]["m_bBuildingRestrictionLimitsEnabled"]; got != "True" {
		t.Fatalf("expected normalized bool value True, got %q", got)
	}
	if got := normalized.updates[secBuilding]["CustomKey"]; got != "raw-value" {
		t.Fatalf("expected custom key passthrough, got %q", got)
	}
	if got := normalized.updates[secBuilding]["AnotherKey"]; got != "" {
		t.Fatalf("expected clear marker empty string, got %q", got)
	}
}

func TestNormalizeServerSettingsUpdates_InvalidKnownValue(t *testing.T) {
	t.Parallel()

	schemaMap := buildServerSettingsSchemaMap()
	_, err := normalizeServerSettingsUpdates([]serverSettingUpdate{
		{Section: secBuilding, Key: "m_bBuildingRestrictionLimitsEnabled", Value: "not-bool"},
	}, schemaMap)
	if err == nil {
		t.Fatalf("expected normalization error for invalid bool")
	}
}

func TestSplitServerSettingsUpdatesByFile(t *testing.T) {
	t.Parallel()

	defaultEngine := map[string]map[string]string{
		"/Script/Engine.EngineSection": {"K": "V"},
	}
	updates := map[string]map[string]string{
		"/Script/Engine.EngineSection": {"A": "1"},
		secBuilding:                    {"B": "2"},
	}
	game, engine := splitServerSettingsUpdatesByFile(defaultEngine, updates)
	if _, ok := engine["/Script/Engine.EngineSection"]; !ok {
		t.Fatalf("expected engine section to route to engine updates")
	}
	if _, ok := game[secBuilding]; !ok {
		t.Fatalf("expected non-engine section to route to game updates")
	}
}

func TestBuildUpdatedINIContent_NoUpdates(t *testing.T) {
	t.Parallel()

	body, err := buildUpdatedINIContent("ignored.ini", map[string]map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != "" {
		t.Fatalf("expected empty body when no updates, got %q", body)
	}
}
