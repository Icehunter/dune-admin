package main

// Server Settings — read/write tuning keys for the running Dune dedicated
// server via the AMP-blessed UserOverrides.ini append file. Direct mode only;
// SSH mode would need a different file path and mechanism.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
)

// Where AMP appends overrides to UserGame.ini at server start.
// This path is also baked into /etc/sudoers.d/dune-admin — keep them in sync.
const userOverridesPath = "/home/amp/.ampdata/instances/MehDune01/duneawakening/server/state/UserOverrides.ini"

const userOverridesHeader = `; ============================================================================
; UserOverrides.ini  -  managed by dune-admin (Server Settings tab)
;
; Anything in this file is appended to UserGame.ini on every server start,
; AFTER AMP has written its managed settings. Keys here win over AMP's.
;
; Hand-edits are preserved on next dune-admin save IF the section/key is one
; dune-admin doesn't know about. Known keys may be overwritten.
;
; To apply changes: restart the Dune instance via AMP UI or:
;   sudo ampinstmgr -q MehDune01 && sudo ampinstmgr -s MehDune01
; ============================================================================
`

type settingType string

const (
	settingFloat settingType = "float"
	settingInt   settingType = "int"
	settingBool  settingType = "bool"
)

type settingDef struct {
	Section     string      `json:"section"`
	Key         string      `json:"key"`
	Type        settingType `json:"type"`
	Default     any         `json:"default"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
	Category    string      `json:"category"`
}

// Curated MVP schema. Sections + keys + categories per Samples-Ideas/server-config-catalog.md.
// Order within a category drives UI render order.
var serverSettingsSchema = []settingDef{
	// ── Survival ───────────────────────────────────────────────────────────
	{"/Script/DuneSandbox.DuneGameMode", "m_GlobalHealthMultiplier", settingFloat, 1.0, "Global Health Multiplier", "Scales the health pool of all entities (players + NPCs)", "Survival"},
	{"/Script/DuneSandbox.DuneGameMode", "m_GlobalDamageToNpcsMultiplier", settingFloat, 1.0, "Damage to NPCs Multiplier", "Scales damage dealt to NPCs", "Survival"},
	{"/Script/DuneSandbox.DuneGameMode", "m_GlobalDamageToPlayersMultiplier", settingFloat, 1.0, "Damage to Players Multiplier", "Scales damage dealt to players (PvP)", "Survival"},
	{"/Script/DuneSandbox.DuneGameMode", "m_WaterConsumptionRate", settingFloat, 1.0, "Water Consumption Rate", "Baseline hydration depletion rate", "Survival"},
	{"/Script/DuneSandbox.DuneGameMode", "m_WaterConsumptionInStormMultiplier", settingFloat, 4.0, "Water Loss in Storm Multiplier", "Extra thirst during storms (default 4×)", "Survival"},
	{"/Script/DuneSandbox.DuneGameMode", "m_PlayerStartingWater", settingFloat, 100.0, "Starting Water", "Hydration value on spawn", "Survival"},
	{"/Script/DuneSandbox.DuneGameMode", "m_DefaultReconnectGracePeriodSeconds", settingInt, 300, "Reconnect Grace Period (s)", "How long a player's corpse persists after disconnect", "Survival"},
	{"/Script/DuneSandbox.DuneGameMode", "m_ItemDurabilityLossMultiplier", settingFloat, 1.0, "Item Durability Loss Multiplier", "Gear degradation rate", "Survival"},

	// ── Building ──────────────────────────────────────────────────────────
	{"/Script/DuneSandbox.DuneGameMode", "m_GlobalBuildingDamageMultiplier", settingFloat, 1.0, "Building Damage Multiplier", "Scales damage taken by player structures", "Building"},
	{"/Script/DuneSandbox.DuneGameMode", "m_BuildingDecayRateMultiplier", settingFloat, 1.0, "Building Decay Multiplier", "Speed of fortification deterioration", "Building"},
	{"/Script/DuneSandbox.DuneGameMode", "bEnableBuildingStability", settingBool, true, "Building Stability Enabled", "Whether structural integrity checks apply", "Building"},
	{"/Script/DuneSandbox.DuneGameMode", "m_InventoryWeightMultiplier", settingFloat, 1.0, "Inventory Weight Multiplier", "Scales carrying capacity", "Building"},

	// ── Progression ───────────────────────────────────────────────────────
	{"/Script/DuneSandbox.DuneGameMode", "m_GlobalXPMultiplier", settingFloat, 1.0, "XP Multiplier", "Scales experience gain", "Progression"},
	{"/Script/DuneSandbox.DuneGameMode", "m_GlobalProgressionSpeedMultiplier", settingFloat, 1.0, "Progression Speed Multiplier", "Scales journey + talent unlock velocity", "Progression"},
	{"/Script/DuneSandbox.DuneGameMode", "m_GlobalFameMultiplier", settingFloat, 1.0, "Fame Multiplier", "Scales reputation accumulation", "Progression"},

	// ── Harvesting ────────────────────────────────────────────────────────
	{"/Script/DuneSandbox.DuneGameMode", "m_GlobalHarvestAmountMultiplier", settingFloat, 1.0, "Harvest Amount Multiplier", "Scales yield per harvest strike", "Harvesting"},
	{"/Script/DuneSandbox.DuneGameMode", "m_GlobalHarvestHealthMultiplier", settingFloat, 1.0, "Harvest Health Multiplier", "Scales durability of resource nodes", "Harvesting"},

	// ── Storm Cycle ───────────────────────────────────────────────────────
	{"/Script/DuneSandbox.SandStormConfig", "m_StormCycleDuration", settingInt, 7200, "Storm Cycle Duration (s)", "Seconds between storm occurrences", "Storm"},
	{"/Script/DuneSandbox.SandStormConfig", "m_StormDuration", settingInt, 600, "Storm Active Duration (s)", "How long an active storm lasts", "Storm"},
	{"/Script/DuneSandbox.SandStormConfig", "m_StormWarningDuration", settingInt, 120, "Storm Warning Duration (s)", "Pre-storm alert window", "Storm"},
	{"/Script/DuneSandbox.SandStormConfig", "m_CycleDurationInDays", settingInt, 7, "Deep Desert Reset Cycle (days)", "How often the Deep Desert resets", "Storm"},

	// ── Guilds & Economy ─────────────────────────────────────────────────
	{"/Script/DuneSandbox.DuneGameMode", "m_MaxGuildMembersAllowed", settingInt, 32, "Max Guild Members", "Ceiling on guild size", "Guilds"},
	{"/Script/DuneSandbox.DuneGameMode", "m_MaxGuildsAllowed", settingInt, 3, "Max Guilds Per Player", "Concurrent guild memberships allowed per player", "Guilds"},
	{"/Script/DuneSandbox.DuneGameMode", "m_GuildCreationCost", settingInt, 1000, "Guild Creation Cost (Solari)", "Cost in Solari to form a guild", "Guilds"},
	{"/Script/DuneSandbox.DuneGameMode", "m_MaxPermissionsPerActor", settingInt, 20, "Max Permissions Per Actor", "Permission slots per structure or container", "Guilds"},

	// ── Inventory ─────────────────────────────────────────────────────────
	{"/Script/DuneSandbox.InventorySystemSettings", "PlayerInventoryStartingSize", settingInt, 40, "Starting Inventory Slots", "Item slot count on first spawn", "Inventory"},
	{"/Script/DuneSandbox.InventorySystemSettings", "PlayerInventoryStartingVolumeCapacity", settingFloat, 225.0, "Starting Inventory Volume", "Bulk capacity on first spawn", "Inventory"},

	// ── Item Durability (existing Survival category) ─────────────────────
	{"/DeteriorationSystem.ItemDeteriorationConstants", "UpdateRateInSeconds", settingFloat, 1.0, "Deterioration Tick Rate (s)", "Seconds between deterioration ticks (0 disables it)", "Survival"},

	// ── Building (additions) ─────────────────────────────────────────────
	{"/Script/DuneSandbox.BuildingSettings", "m_MaxNumLandclaimSegments", settingInt, 3, "Max Landclaim Segments", "How many landclaim tiles a player can own (also needs client-side match)", "Building"},
	{"/Script/DuneSandbox.BuildingSettings", "m_BuildingBlueprintMaxExtensions", settingInt, 3, "Max Blueprint Extensions", "How many times a blueprint can be extended", "Building"},
	{"/Script/DuneSandbox.BuildingSettings", "m_BaseBackupMaxExtensions", settingInt, 3, "Max Base Backup Extensions", "How many extensions a base backup allows", "Building"},
	{"/Script/DuneSandbox.BuildingSettings", "m_bBuildingRestrictionLimitsEnabled", settingBool, true, "Building Restriction Limits Enabled", "Whether per-claim build limits apply", "Building"},

	// ── Storm (toggle additions) ──────────────────────────────────────────
	{"/Script/DuneSandbox.SandStormConfig", "m_bCoriolisAutoSpawnEnabled", settingBool, true, "Coriolis Auto-Spawn", "Whether the Coriolis storm auto-spawns on its cycle", "Storm"},
	{"/Script/DuneSandbox.SandStormConfig", "m_bIsDbWipeEnabled", settingBool, true, "DB Wipe on Coriolis Cycle", "Whether the database wipes on each Coriolis cycle (Deep Desert reset)", "Storm"},

	// ── Sandworm ──────────────────────────────────────────────────────────
	{"/Script/DuneSandbox.SandwormSettings", "WormDetectionDistance", settingFloat, 5000.0, "Worm Detection Distance", "Sensory radius — how far away a worm can detect a player", "Sandworm"},
	{"/Script/DuneSandbox.SandwormSettings", "m_MinWormSpawnInternal", settingFloat, 300.0, "Min Worm Spawn Interval (s)", "Minimum seconds between worm spawns (typo in upstream key name preserved)", "Sandworm"},
	{"/Script/DuneSandbox.SandwormSettings", "m_MinDistanceBetweenSandworms", settingFloat, 3000.0, "Min Distance Between Worms", "Spacing requirement for multiple worms", "Sandworm"},
	{"/Script/DuneSandbox.SandwormSettings", "m_SandwormQuicksandSpeedModifier", settingFloat, 0.5, "Quicksand Speed Modifier", "Movement speed multiplier when on a worm's quicksand", "Sandworm"},
	{"/Script/DuneSandbox.SandwormSettings", "m_GiantWormMinimumPlayersOnSpiceField", settingInt, 1, "Giant Worm Min Players", "Minimum players on a spice field before giant worm can appear", "Sandworm"},

	// ── PvP & Security ────────────────────────────────────────────────────
	{"/Script/DuneSandbox.DuneGameMode", "bPvPEnabled", settingBool, false, "PvP Enabled", "Server-wide PvP master toggle (per-partition rules in PvpPveSettings)", "PvP & Security"},
	{"/Script/DuneSandbox.DuneGameMode", "bServerPVE", settingBool, true, "Server PvE Mode", "Whether server enforces PvE ruleset", "PvP & Security"},
	{"/Script/DuneSandbox.PvpPveSettings", "m_bShouldForceEnablePvpOnAllPartitions", settingBool, false, "Force PvP On All Partitions", "Override per-partition PvP settings — force PvP everywhere", "PvP & Security"},
	{"/Script/DuneSandbox.SecurityZonesSubsystem", "m_bAreSecurityZonesEnabled", settingBool, true, "Security Zones Enabled", "Whether security zones apply (disabling allows PvP and ability use everywhere)", "PvP & Security"},

	// ── Spice Harvesting ──────────────────────────────────────────────────
	{"/Script/DuneSandbox.SpiceHarvestingSystem", "m_PrimeRateInSeconds", settingFloat, 30.0, "Spice Prime Rate (s)", "Seconds for a spice field to prime before harvesting", "Spice"},
	{"/Script/DuneSandbox.SpiceHarvestingSystem", "m_NodeValueToSpiceResourceRatio", settingFloat, 10.0, "Spice Node Value Ratio", "Conversion ratio from node value to spice yield", "Spice"},
	{"/Script/DuneSandbox.SpiceHarvestingSystem", "m_bSpawningActive", settingBool, true, "Spice Spawning Active", "Whether spice fields spawn at all", "Spice"},
	{"/Script/DuneSandbox.SpiceHarvestingSystem", "m_bPlayerMustWitnessBloom", settingBool, false, "Player Must Witness Bloom", "Whether spice blooms only happen when a player can see them", "Spice"},

	// ── Taxation ──────────────────────────────────────────────────────────
	{"/Script/DuneSandbox.TaxationSettings", "m_bTaxationEnabled", settingBool, true, "Taxation Enabled", "Whether the landclaim tax system runs", "Taxation"},
	{"/Script/DuneSandbox.TaxationSettings", "m_TaxationCycleLengthSeconds", settingInt, 86400, "Tax Cycle Length (s)", "Seconds in one tax cycle (default 86400 = 1 day)", "Taxation"},
	{"/Script/DuneSandbox.TaxationSettings", "m_SpicePerHour", settingInt, 100, "Base Spice Per Hour", "Base spice tax rate per hour per landclaim", "Taxation"},
}

func findSettingDef(section, key string) *settingDef {
	for i := range serverSettingsSchema {
		if serverSettingsSchema[i].Section == section && serverSettingsSchema[i].Key == key {
			return &serverSettingsSchema[i]
		}
	}
	return nil
}

// ── Read / parse ────────────────────────────────────────────────────────────

func readUserOverrides() (map[string]map[string]string, error) {
	out, err := exec.Command("sudo", "-u", "amp", "/usr/bin/cat", userOverridesPath).Output()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", userOverridesPath, err)
	}
	return parseINI(string(out)), nil
}

// parseINI handles the subset of UE INI we care about: [Section] headers + key=value
// lines. Comments (lines starting with ;) and blank lines are skipped. Array
// keys (+key=val) are not part of the MVP schema and aren't preserved.
func parseINI(content string) map[string]map[string]string {
	sections := map[string]map[string]string{}
	var current string
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = line[1 : len(line)-1]
			if sections[current] == nil {
				sections[current] = map[string]string{}
			}
			continue
		}
		if eq := strings.Index(line, "="); eq > 0 && current != "" {
			key := strings.TrimSpace(line[:eq])
			val := strings.TrimSpace(line[eq+1:])
			sections[current][key] = val
		}
	}
	return sections
}

// ── Write ───────────────────────────────────────────────────────────────────

func writeUserOverrides(values map[string]map[string]string) error {
	var sb strings.Builder
	sb.WriteString(userOverridesHeader)

	// Stable section order (schema order, then anything else alphabetically)
	written := map[string]bool{}
	for _, def := range serverSettingsSchema {
		if written[def.Section] {
			continue
		}
		written[def.Section] = true
		if kvs, ok := values[def.Section]; ok && len(kvs) > 0 {
			writeSection(&sb, def.Section, kvs)
		}
	}
	for section, kvs := range values {
		if written[section] || len(kvs) == 0 {
			continue
		}
		writeSection(&sb, section, kvs)
	}

	cmd := exec.Command("sudo", "-u", "amp", "/usr/bin/tee", userOverridesPath)
	cmd.Stdin = strings.NewReader(sb.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("write %s: %w (%s)", userOverridesPath, err, out)
	}
	return nil
}

func writeSection(sb *strings.Builder, section string, kvs map[string]string) {
	sb.WriteString("\n[")
	sb.WriteString(section)
	sb.WriteString("]\n")
	// Write known keys first in schema order
	written := map[string]bool{}
	for _, def := range serverSettingsSchema {
		if def.Section != section {
			continue
		}
		if v, ok := kvs[def.Key]; ok {
			sb.WriteString(def.Key)
			sb.WriteString("=")
			sb.WriteString(v)
			sb.WriteString("\n")
			written[def.Key] = true
		}
	}
	// Preserve any unknown keys (hand-edits we don't recognize)
	for k, v := range kvs {
		if written[k] {
			continue
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
		sb.WriteString("\n")
	}
}

// ── Value normalization ─────────────────────────────────────────────────────

// normalizeValue formats a user-supplied string into the UE INI value
// representation for the given setting type. Returns the formatted value or
// an error if the input doesn't parse.
func normalizeValue(t settingType, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	switch t {
	case settingBool:
		switch strings.ToLower(raw) {
		case "true", "1", "yes":
			return "True", nil
		case "false", "0", "no":
			return "False", nil
		}
		return "", fmt.Errorf("invalid bool %q", raw)
	case settingInt:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid int %q", raw)
		}
		return strconv.FormatInt(n, 10), nil
	case settingFloat:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return "", fmt.Errorf("invalid float %q", raw)
		}
		return strconv.FormatFloat(f, 'f', 6, 64), nil
	}
	return "", fmt.Errorf("unknown setting type %q", t)
}

// ── HTTP handlers ───────────────────────────────────────────────────────────

type serverSettingItem struct {
	settingDef
	Current      string `json:"current"`       // raw INI value if overridden, empty otherwise
	IsOverridden bool   `json:"is_overridden"` // whether the user has set it explicitly
}

func handleGetServerSettings(w http.ResponseWriter, _ *http.Request) {
	if connectionMode != "direct" {
		jsonErr(w, fmt.Errorf("server-settings only available in direct mode"), http.StatusNotImplemented)
		return
	}

	overrides, err := readUserOverrides()
	if err != nil {
		jsonErr(w, err, http.StatusInternalServerError)
		return
	}

	items := make([]serverSettingItem, 0, len(serverSettingsSchema))
	for _, def := range serverSettingsSchema {
		current := overrides[def.Section][def.Key]
		items = append(items, serverSettingItem{
			settingDef:   def,
			Current:      current,
			IsOverridden: current != "",
		})
	}
	jsonOK(w, items)
}

func handleUpdateServerSettings(w http.ResponseWriter, r *http.Request) {
	if connectionMode != "direct" {
		jsonErr(w, fmt.Errorf("server-settings only available in direct mode"), http.StatusNotImplemented)
		return
	}

	var req struct {
		Updates []struct {
			Section string `json:"section"`
			Key     string `json:"key"`
			Value   string `json:"value"` // empty string ⇒ unset (revert to default)
		} `json:"updates"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	overrides, err := readUserOverrides()
	if err != nil {
		jsonErr(w, err, http.StatusInternalServerError)
		return
	}

	applied := 0
	cleared := 0
	for _, u := range req.Updates {
		def := findSettingDef(u.Section, u.Key)
		if def == nil {
			jsonErr(w, fmt.Errorf("unknown setting: %s / %s", u.Section, u.Key), http.StatusBadRequest)
			return
		}
		if u.Value == "" {
			if overrides[u.Section] != nil {
				delete(overrides[u.Section], u.Key)
				cleared++
			}
			continue
		}
		formatted, err := normalizeValue(def.Type, u.Value)
		if err != nil {
			jsonErr(w, fmt.Errorf("%s/%s: %w", u.Section, u.Key, err), http.StatusBadRequest)
			return
		}
		if overrides[u.Section] == nil {
			overrides[u.Section] = map[string]string{}
		}
		overrides[u.Section][u.Key] = formatted
		applied++
	}

	if err := writeUserOverrides(overrides); err != nil {
		jsonErr(w, err, http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]any{
		"ok":      fmt.Sprintf("Saved (%d set, %d cleared). Restart Dune instance via AMP UI to apply.", applied, cleared),
		"applied": applied,
		"cleared": cleared,
	})
}
