package main

import (
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
)

// userServerCustomKeys are the UserServerCustomSettings.ini fields AMP's Dune
// template exposes (Dune Awakening 1.5). #336.
var userServerCustomKeys = []string{
	"CraftingTimeMultiplier", "CraftingCost", "BuildingCostMultiplier", "InventoryVolumeMultiplier",
	"WaterExtractionRate", "ResourceRespawnSpeed", "LootRespawnSpeed", "FuelBurnTimeMultiplier",
	"PlayerDamageToPlayer", "PlayerDamageToNPC", "PlayerDamageToVehicle", "PVPDamageStructures",
	"NPCHealth", "NPCDamageToPlayer", "NPCDamageToNPC", "NPCRespawnMultiplier",
	"PlayerShieldDamageAbsorptionMultiplier", "NPCShieldDamageAbsorptionMultiplier", "PlayerStaminaDrain",
	"GlobalXpMultiplier", "CombatXp", "GatheringXp", "MissionXp", "IntelPointsGainMultiplier",
	"ItemDurabilityDrainMultiplier", "bEnableItemMaxDurabilityLoss",
	"HeatBuildupRate", "ThirstMultiplier", "DropEquipmentOnDeath", "SandwormConsequences",
	"PlayerDeathLootRule", "bAllowDynamicBuildingDamage",
	"BuildingPieceLimitMultiplier", "bBuildingInfiniteStability", "BaseBackupToolTimeRestriction",
	"LandsraadContributionMultiplier", "LandsraadSpecializationXpMultiplier",
	"LandsraadFactionStandingMultiplier", "bLandsraadDisableDecreeRerollLimit",
}

func TestServerSettingsSchema_IncludesUserServerCustomSettings(t *testing.T) {
	t.Parallel()
	schema := buildServerSettingsSchemaMap()
	categories := map[string]bool{
		catCraftingHarvesting: true, catCombatProgression: true, catSurvival: true, catBuildingLandsraad: true,
	}
	for _, key := range userServerCustomKeys {
		def, ok := schema[secCustom+"|"+key]
		if !ok {
			t.Errorf("schema missing %s", key)
			continue
		}
		if def.FieldName != secCustom+"."+key {
			t.Errorf("%s FieldName = %q, want AMP node suffix %q", key, def.FieldName, secCustom+"."+key)
		}
		if !categories[def.Category] {
			t.Errorf("%s category = %q, want one of the UserServerCustomSettings categories", key, def.Category)
		}
		if def.Label == "" || def.Description == "" || def.Default == "" {
			t.Errorf("%s missing label/description/default: %+v", key, def)
		}
	}
}

func TestServerSettingsSchema_CustomTypes(t *testing.T) {
	t.Parallel()
	schema := buildServerSettingsSchemaMap()
	cases := []struct {
		key     string
		typ     settingType
		def     string
		options []string
	}{
		{"CraftingCost", settingFloat, "1.0", nil},
		{"BaseBackupToolTimeRestriction", settingFloat, "16.0", nil},
		{"bEnableItemMaxDurabilityLoss", settingBool, "True", nil},
		{"bBuildingInfiniteStability", settingBool, "False", nil},
		{"DropEquipmentOnDeath", settingEnum, "Default", []string{"Default", "All", "Backpack", "None"}},
		{"SandwormConsequences", settingEnum, "All", []string{"Default", "All", "Backpack", "None"}},
		{"PlayerDeathLootRule", settingEnum, "DependsOnSecurityZone",
			[]string{"DependsOnSecurityZone", "AlwaysAllowOtherPlayers", "NeverAllowOtherPlayers"}},
	}
	for _, tc := range cases {
		def := schema[secCustom+"|"+tc.key]
		if def.Type != tc.typ || def.Default != tc.def {
			t.Errorf("%s = (%s, %q), want (%s, %q)", tc.key, def.Type, def.Default, tc.typ, tc.def)
		}
		if !slices.Equal(def.Options, tc.options) {
			t.Errorf("%s options = %v, want %v", tc.key, def.Options, tc.options)
		}
	}
}

func TestBuildSchemaSettings_PropagatesOptions(t *testing.T) {
	t.Parallel()
	for _, s := range buildSchemaSettings(nil) {
		if s.Section == secCustom && s.Key == "PlayerDeathLootRule" {
			if len(s.Options) != 3 || s.Type != string(settingEnum) {
				t.Errorf("PlayerDeathLootRule = type %q options %v, want enum with 3 options", s.Type, s.Options)
			}
			return
		}
	}
	t.Fatal("PlayerDeathLootRule not built")
}

func TestNormalizeServerSettingsUpdates_Enum(t *testing.T) {
	t.Parallel()
	schema := buildServerSettingsSchemaMap()

	got, err := normalizeServerSettingsUpdates([]serverSettingUpdate{
		{Section: secCustom, Key: "DropEquipmentOnDeath", Value: "Backpack"},
		{Section: secCustom, Key: "bBuildingInfiniteStability", Value: "true"},
	}, schema)
	if err != nil {
		t.Fatalf("valid enum rejected: %v", err)
	}
	if v := got.updates[secCustom]["DropEquipmentOnDeath"]; v != "Backpack" {
		t.Errorf("enum value = %q, want Backpack", v)
	}
	if v := got.updates[secCustom]["bBuildingInfiniteStability"]; v != "True" {
		t.Errorf("bool value = %q, want True", v)
	}

	if _, err := normalizeServerSettingsUpdates([]serverSettingUpdate{
		{Section: secCustom, Key: "DropEquipmentOnDeath", Value: "Everything"},
	}, schema); err == nil {
		t.Error("invalid enum value accepted, want error")
	}
}

func TestSplitCustomSettingsUpdates(t *testing.T) {
	t.Parallel()
	updates := map[string]map[string]string{
		secCustom:      {"CraftingCost": "2.000000"},
		secBuilding:    {"m_MaxNumLandclaimSegments": "8"},
		secConsoleVars: {"Dune.GlobalMiningOutputMultiplier": "2.000000"},
	}
	custom, rest := splitCustomSettingsUpdates(updates)
	if custom["CraftingCost"] != "2.000000" {
		t.Errorf("custom = %v, want CraftingCost", custom)
	}
	if _, ok := custom["DifficultyLevel"]; !ok || custom["DifficultyLevel"] != "Custom" {
		t.Errorf("custom = %v, want DifficultyLevel=Custom so the game applies the values", custom)
	}
	if _, ok := rest[secCustom]; ok {
		t.Error("custom section left in the game/engine updates")
	}
	if len(rest) != 2 {
		t.Errorf("rest = %v, want building + console vars", rest)
	}

	custom, rest = splitCustomSettingsUpdates(map[string]map[string]string{secBuilding: {"K": "1"}})
	if custom != nil || len(rest) != 1 {
		t.Errorf("no custom updates: custom=%v rest=%v, want nil + untouched", custom, rest)
	}
}

// writeCaptureExecutor records WriteFile bodies by path; Exec returns "" so
// every INI read comes back empty.
type writeCaptureExecutor struct {
	fnExecutor
	mu     sync.Mutex
	writes map[string]string
}

func (w *writeCaptureExecutor) WriteFile(path string, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes[path] = string(b)
	return nil
}

func TestApplyServerSettingsToINI_WritesUserServerCustomSettings(t *testing.T) {
	t.Parallel()
	exec := &writeCaptureExecutor{
		fnExecutor: fnExecutor{fn: func(string) (string, error) { return "", nil }},
		writes:     map[string]string{},
	}
	status, err := applyServerSettingsToINI("/srv/state", map[string]map[string]string{
		secCustom:   {"CraftingCost": "2.000000"},
		secBuilding: {"m_MaxNumLandclaimSegments": "8"},
	}, nil, exec)
	if err != nil || status != 0 {
		t.Fatalf("applyServerSettingsToINI: status=%d err=%v", status, err)
	}
	body, ok := exec.writes["/srv/state/UserServerCustomSettings.ini"]
	if !ok {
		t.Fatalf("UserServerCustomSettings.ini not written; writes: %v", exec.writes)
	}
	for _, want := range []string{"[" + secCustom + "]", "CraftingCost=2.000000", "DifficultyLevel=Custom"} {
		if !strings.Contains(body, want) {
			t.Errorf("UserServerCustomSettings.ini missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(exec.writes["/srv/state/UserGame.ini"], secCustom) {
		t.Error("custom section must not be written to UserGame.ini")
	}
	if !strings.Contains(exec.writes["/srv/state/UserGame.ini"], "m_MaxNumLandclaimSegments=8") {
		t.Errorf("game setting not written to UserGame.ini: %v", exec.writes)
	}
}

func TestBuildLayerSources_IncludesUserCustom(t *testing.T) {
	t.Parallel()
	custom := map[string]map[string]string{secCustom: {"CraftingCost": "3.000000"}}
	layers := buildLayerSources(nil, nil, nil, nil, custom, nil)
	s := ServerSetting{Section: secCustom, Key: "CraftingCost", Type: "float"}
	applySettingLayers(&s, layers)
	if s.Current != "3.000000" || s.Source != "userCustom" {
		t.Errorf("setting = (%q, %q), want (3.000000, userCustom)", s.Current, s.Source)
	}
}

func TestSplitCustomSettingsUpdates_DifficultyLevel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		kvs  map[string]string
		want string // "" = DifficultyLevel must not be set
	}{
		{"set value forces Custom", map[string]string{"CraftingCost": "2.000000"}, "Custom"},
		{"delete only leaves difficulty alone", map[string]string{"CraftingCost": ""}, ""},
		{"explicit difficulty wins", map[string]string{"CraftingCost": "2.000000", "DifficultyLevel": "Hard"}, "Hard"},
		{"explicit difficulty clear wins", map[string]string{"DifficultyLevel": ""}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			custom, _ := splitCustomSettingsUpdates(map[string]map[string]string{secCustom: tc.kvs})
			got, ok := custom["DifficultyLevel"]
			if tc.want == "" {
				if ok && got != "" {
					t.Errorf("DifficultyLevel = %q, want untouched", got)
				}
				return
			}
			if got != tc.want {
				t.Errorf("DifficultyLevel = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeServerSettingsUpdates_EnumCaseInsensitive(t *testing.T) {
	t.Parallel()
	got, err := normalizeServerSettingsUpdates([]serverSettingUpdate{
		{Section: secCustom, Key: "DropEquipmentOnDeath", Value: "backpack"},
	}, buildServerSettingsSchemaMap())
	if err != nil {
		t.Fatalf("lower-case enum rejected: %v", err)
	}
	if v := got.updates[secCustom]["DropEquipmentOnDeath"]; v != "Backpack" {
		t.Errorf("enum value = %q, want canonical Backpack", v)
	}
}

// AMP regenerates UserServerCustomSettings.ini from its own config, so a write
// there under AMP is lost on restart. Non-curated custom keys must be refused.
func TestApplyServerSettingsToINI_AMPRejectsCustomFileWrites(t *testing.T) {
	t.Parallel()
	exec := &writeCaptureExecutor{
		fnExecutor: fnExecutor{fn: func(string) (string, error) { return "", nil }},
		writes:     map[string]string{},
	}
	status, err := applyServerSettingsToINI("/srv/state", map[string]map[string]string{
		secCustom: {"DifficultyLevel": "Hard"},
	}, ampSettingsControl(), exec)
	if err == nil || status != 400 {
		t.Fatalf("status=%d err=%v, want 400 error", status, err)
	}
	if len(exec.writes) != 0 {
		t.Errorf("nothing may be written on rejection; writes: %v", exec.writes)
	}
}
