package main

import "testing"

// TestFieldNameToSectionKey covers the FieldName shapes the curated schema uses:
// ConsoleVariables CVars (single [ConsoleVariables] section, dotted CVar name as
// key) and UPROPERTY fields under /Script/... and /DeteriorationSystem...
// (class-path section, trailing member as key).
func TestFieldNameToSectionKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		fieldName   string
		wantSection string
		wantKey     string
	}{
		{"ConsoleVariables.Dune.GlobalMiningOutputMultiplier", "ConsoleVariables", "Dune.GlobalMiningOutputMultiplier"},
		{"ConsoleVariables.Bgd.ServerDisplayName", "ConsoleVariables", "Bgd.ServerDisplayName"},
		{"ConsoleVariables.dw.VehicleDurabilityDamageMultiplier", "ConsoleVariables", "dw.VehicleDurabilityDamageMultiplier"},
		{"/Script/DuneSandbox.BuildingSettings.m_MaxNumLandclaimSegments", "/Script/DuneSandbox.BuildingSettings", "m_MaxNumLandclaimSegments"},
		{"/Script/DuneSandbox.PvpPveSettings.m_bShouldForceEnablePvpOnAllPartitions", "/Script/DuneSandbox.PvpPveSettings", "m_bShouldForceEnablePvpOnAllPartitions"},
		{"/DeteriorationSystem.ItemDeteriorationConstants.UpdateRateInSeconds", "/DeteriorationSystem.ItemDeteriorationConstants", "UpdateRateInSeconds"},
		{"WorldTitle", "WorldTitle", ""}, // ampOnly / no ini target
	}
	for _, tt := range tests {
		gotSec, gotKey := fieldNameToSectionKey(tt.fieldName)
		if gotSec != tt.wantSection || gotKey != tt.wantKey {
			t.Errorf("fieldNameToSectionKey(%q) = (%q,%q), want (%q,%q)",
				tt.fieldName, gotSec, gotKey, tt.wantSection, tt.wantKey)
		}
	}
}

// TestServerSettingsSchema_Consistency ensures every curated entry's
// (Section,Key) was derived from its FieldName, has the required metadata, and
// is unique. ampOnly settings (empty Key) are out of scope for this rework.
func TestServerSettingsSchema_Consistency(t *testing.T) {
	t.Parallel()
	seenField := map[string]bool{}
	seenSectionKey := map[string]bool{}
	for _, d := range serverSettingsSchema {
		if d.FieldName == "" {
			t.Errorf("schema entry %q has empty FieldName", d.Label)
			continue
		}
		wantSec, wantKey := fieldNameToSectionKey(d.FieldName)
		if d.Section != wantSec || d.Key != wantKey {
			t.Errorf("%s: (Section,Key) = (%q,%q), want (%q,%q) derived from FieldName",
				d.FieldName, d.Section, d.Key, wantSec, wantKey)
		}
		if d.Key == "" {
			t.Errorf("%s: empty Key — ampOnly settings are not in the curated ini schema", d.FieldName)
		}
		if d.Label == "" || d.Category == "" {
			t.Errorf("%s: missing Label or Category", d.FieldName)
		}
		if seenField[d.FieldName] {
			t.Errorf("duplicate FieldName %q", d.FieldName)
		}
		seenField[d.FieldName] = true
		sk := d.Section + "|" + d.Key
		if seenSectionKey[sk] {
			t.Errorf("duplicate Section|Key %q", sk)
		}
		seenSectionKey[sk] = true
	}
}

// TestServerSettingsSchema_NoFictionalKeys guards against regressing to the
// fictional m_Global*Multiplier keys that #122 proved are no-ops — they are
// absent from the real DefaultGame.ini, so writing them does nothing.
func TestServerSettingsSchema_NoFictionalKeys(t *testing.T) {
	t.Parallel()
	fictional := map[string]bool{
		"m_GlobalHealthMultiplier": true, "m_GlobalDamageToNpcsMultiplier": true,
		"m_GlobalDamageToPlayersMultiplier": true, "m_GlobalXPMultiplier": true,
		"m_GlobalProgressionSpeedMultiplier": true, "m_GlobalFameMultiplier": true,
		"m_GlobalHarvestAmountMultiplier": true, "m_GlobalHarvestHealthMultiplier": true,
		"m_GlobalBuildingDamageMultiplier": true, "m_InventoryWeightMultiplier": true,
	}
	for _, d := range serverSettingsSchema {
		if fictional[d.Key] {
			t.Errorf("fictional no-op key %q must not be in the curated schema (FieldName %q)", d.Key, d.FieldName)
		}
	}
}

// TestServerSettingsSchema_HasProvenMiningCVar locks in the #122 fix: the real
// mining-output CVar is present and decomposes to the correct INI target.
func TestServerSettingsSchema_HasProvenMiningCVar(t *testing.T) {
	t.Parallel()
	var found *settingDef
	for i := range serverSettingsSchema {
		if serverSettingsSchema[i].FieldName == "ConsoleVariables.Dune.GlobalMiningOutputMultiplier" {
			found = &serverSettingsSchema[i]
			break
		}
	}
	if found == nil {
		t.Fatal("proven mining-output CVar missing from schema")
	}
	if found.Section != "ConsoleVariables" || found.Key != "Dune.GlobalMiningOutputMultiplier" {
		t.Errorf("mining CVar decomposed to (%q,%q), want (ConsoleVariables, Dune.GlobalMiningOutputMultiplier)",
			found.Section, found.Key)
	}
	if found.Type != settingFloat {
		t.Errorf("mining CVar type = %q, want float", found.Type)
	}
}

// TestSplitServerSettingsUpdatesByFile_ConsoleVariablesAlwaysEngine verifies CVar
// updates route to UserEngine.ini even when DefaultEngine.ini is unreadable
// (empty map): [ConsoleVariables] is engine-scoped regardless of what the
// default files happen to declare.
func TestSplitServerSettingsUpdatesByFile_ConsoleVariablesAlwaysEngine(t *testing.T) {
	t.Parallel()
	emptyDefaultEngine := map[string]map[string]string{}
	updates := map[string]map[string]string{
		"ConsoleVariables": {"Dune.GlobalMiningOutputMultiplier": "3.0"},
		secBuilding:        {"m_MaxNumLandclaimSegments": "6"},
	}
	game, engine := splitServerSettingsUpdatesByFile(emptyDefaultEngine, updates)
	if _, ok := engine["ConsoleVariables"]; !ok {
		t.Error("ConsoleVariables must route to engine even with empty DefaultEngine.ini")
	}
	if _, ok := game[secBuilding]; !ok {
		t.Error("UPROPERTY section should route to game updates")
	}
}
