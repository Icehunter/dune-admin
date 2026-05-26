package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type settingType string

const (
	settingFloat  settingType = "float"
	settingInt    settingType = "int"
	settingBool   settingType = "bool"
	settingString settingType = "string"
)

type settingDef struct {
	Section     string
	Key         string
	Type        settingType
	Default     string
	Label       string
	Description string
	Category    string
}

type ServerSetting struct {
	Section     string         `json:"section"`
	Key         string         `json:"key"`
	Type        string         `json:"type"`
	Default     string         `json:"default"`
	Label       string         `json:"label"`
	Description string         `json:"description"`
	Category    string         `json:"category"`
	Current     string         `json:"current"`
	IsOverride  bool           `json:"is_overridden"`
	Source      string         `json:"source"` // "userOverrides"|"userGame"|"userEngine"|"defaultGame"|"defaultEngine"|""
	Layers      []SettingLayer `json:"layers"` // ordered low→high priority; empty when setting is unconfigured
}

// SettingLayer records one file's contribution to a setting's value,
// ordered low → high priority in the Layers slice.
type SettingLayer struct {
	Source string `json:"source"` // "defaultEngine"|"defaultGame"|"userEngine"|"userGame"|"userOverrides"
	Value  string `json:"value"`
}

type serverSettingUpdate struct {
	Section string `json:"section"`
	Key     string `json:"key"`
	Value   string `json:"value"` // empty = clear/revert to default
}

// RawLine is a single line from an INI file that doesn't match any schema entry
// or is an array operation (+/-). Returned alongside typed ServerSettings so the
// UI can display everything that's actually in the files.
type RawLine struct {
	Prefix string `json:"prefix"` // "", "+", or "-"
	Key    string `json:"key"`
	Value  string `json:"value"`
}

// RawSection groups non-schema / array lines by their INI section and source file.
type RawSection struct {
	Section string    `json:"section"`
	Source  string    `json:"source"` // "userGame"|"userOverrides"|"userEngine"|"defaultGame"|"defaultEngine"
	Lines   []RawLine `json:"lines"`
}

// ── Schema ────────────────────────────────────────────────────────────────────

const (
	secGame        = "/Script/DuneSandbox.DuneGameMode"
	secStorm       = "/Script/DuneSandbox.SandStormConfig"
	secBuilding    = "/Script/DuneSandbox.BuildingSettings"
	secInventory   = "/Script/DuneSandbox.InventorySystemSettings"
	secPvP         = "/Script/DuneSandbox.PvpPveSettings"
	secSecurity    = "/Script/DuneSandbox.SecurityZonesSubsystem"
	secSpice       = "/Script/DuneSandbox.SpiceHarvestingSystem"
	secTaxation    = "/Script/DuneSandbox.TaxationSettings"
	secSandworm    = "/Script/DuneSandbox.SandwormSettings"
	secDurab       = "/DeteriorationSystem.ItemDeteriorationConstants"
	secGuilds      = "/Script/DuneSandbox.GuildSettings"
	secOnlineState = "/Script/DuneSandbox.PlayerOnlineStateSettings"
)

var serverSettingsSchema = []settingDef{
	// Survival
	{secGame, "m_GlobalHealthMultiplier", settingFloat, "1.0", "Global Health Multiplier", "Scales the health pool of all entities (players + NPCs)", "Survival"},
	{secGame, "m_GlobalDamageToNpcsMultiplier", settingFloat, "1.0", "Damage to NPCs Multiplier", "Scales damage dealt to NPCs", "Survival"},
	{secGame, "m_GlobalDamageToPlayersMultiplier", settingFloat, "1.0", "Damage to Players Multiplier", "Scales damage dealt to players", "Survival"},
	{secGame, "m_WaterConsumptionRate", settingFloat, "1.0", "Water Consumption Rate", "How quickly players consume water", "Survival"},
	{secGame, "m_WaterConsumptionInStormMultiplier", settingFloat, "2.0", "Water Consumption in Storm Multiplier", "Additional water drain during sandstorms", "Survival"},
	{secGame, "m_PlayerStartingWater", settingFloat, "100.0", "Player Starting Water", "Water amount when a player spawns", "Survival"},
	{secOnlineState, "m_DefaultReconnectGracePeriodSeconds", settingInt, "300", "Reconnect Grace Period (s)", "Seconds a player's corpse persists after disconnect", "Survival"},
	{secDurab, "m_ItemDurabilityLossMultiplier", settingFloat, "1.0", "Item Durability Loss Multiplier", "Scales durability loss for all items", "Survival"},

	// Progression
	{secGame, "m_GlobalXPMultiplier", settingFloat, "1.0", "XP Multiplier", "Scales XP gained from all sources", "Progression"},
	{secGame, "m_GlobalProgressionSpeedMultiplier", settingFloat, "1.0", "Progression Speed Multiplier", "Scales overall progression speed", "Progression"},
	{secGame, "m_GlobalFameMultiplier", settingFloat, "1.0", "Fame Multiplier", "Scales fame gained from all sources", "Progression"},

	// Harvesting
	{secGame, "m_GlobalHarvestAmountMultiplier", settingFloat, "1.0", "Harvest Amount Multiplier", "Scales resource yield from harvesting", "Harvesting"},
	{secGame, "m_GlobalHarvestHealthMultiplier", settingFloat, "1.0", "Harvest Health Multiplier", "Scales node health (how long nodes last)", "Harvesting"},

	// Building
	{secBuilding, "m_GlobalBuildingDamageMultiplier", settingFloat, "1.0", "Building Damage Multiplier", "Scales damage dealt to player buildings", "Building"},
	{secBuilding, "m_BuildingDecayRateMultiplier", settingFloat, "1.0", "Building Decay Rate Multiplier", "Scales how fast buildings decay", "Building"},
	{secBuilding, "bEnableBuildingStability", settingBool, "True", "Enable Building Stability", "Whether structural stability rules apply", "Building"},
	{secBuilding, "m_MaxNumLandclaimSegments", settingInt, "100", "Max Landclaim Segments", "Maximum territory claim segments per guild", "Building"},
	{secBuilding, "m_BuildingBlueprintMaxExtensions", settingInt, "5", "Blueprint Max Extensions", "Maximum blueprint extension slots", "Building"},
	{secBuilding, "m_BaseBackupExtensions", settingInt, "2", "Base Backup Extensions", "Default backup extension slots per base", "Building"},

	// Inventory
	{secInventory, "PlayerInventoryStartingSize", settingInt, "40", "Starting Inventory Slots", "Number of inventory slots at spawn", "Inventory"},
	{secInventory, "PlayerInventoryStartingVolumeCapacity", settingFloat, "225.0", "Starting Inventory Volume", "Volume capacity of starting inventory", "Inventory"},
	{secGame, "m_InventoryWeightMultiplier", settingFloat, "1.0", "Inventory Weight Multiplier", "Scales item weight across all inventories", "Inventory"},

	// Guilds & Economy
	{secGuilds, "m_MaxGuildMembersAllowed", settingInt, "32", "Max Guild Members", "Maximum players per guild", "Guilds & Economy"},
	{secGuilds, "m_MaxGuildsAllowed", settingInt, "3", "Max Guilds per Player", "How many guilds a player may belong to", "Guilds & Economy"},
	{secGuilds, "m_GuildCreationCost", settingInt, "1000", "Guild Creation Cost (Solari)", "Solari required to create a guild", "Guilds & Economy"},
	{secGuilds, "m_MaxPermissionsPerActor", settingInt, "20", "Max Permissions per Actor", "Max permission rules per actor/structure", "Guilds & Economy"},

	// Storm Cycle
	{secStorm, "m_StormCycleDuration", settingInt, "3600", "Storm Cycle Duration (s)", "Total duration of one storm cycle", "Storm Cycle"},
	{secStorm, "m_StormDuration", settingInt, "900", "Storm Duration (s)", "How long each active storm lasts", "Storm Cycle"},
	{secStorm, "m_StormWarningDuration", settingInt, "300", "Storm Warning Duration (s)", "Warning period before a storm hits", "Storm Cycle"},
	{secStorm, "m_CycleDurationInDays", settingInt, "7", "Coriolis Cycle (days)", "In-game days between Coriolis storm events", "Storm Cycle"},
	{secGame, "m_bCoriolisAutoSpawnEnabled", settingBool, "True", "Coriolis Auto-Spawn", "Whether Coriolis storms spawn automatically", "Storm Cycle"},
	{secGame, "m_bIsDbWipeEnabled", settingBool, "False", "Database Wipe on Season End", "Wipe the database when the season ends", "Storm Cycle"},

	// PvP & Security
	{secPvP, "bPvPEnabled", settingBool, "False", "PvP Enabled", "Allow player-vs-player combat globally", "PvP & Security"},
	{secPvP, "bServerPVE", settingBool, "True", "Server PvE Mode", "Enables PvE protection globally", "PvP & Security"},
	{secPvP, "m_bShouldForceEnablePvpOnAllPartitions", settingBool, "False", "Force PvP on All Partitions", "Override per-partition PvP settings", "PvP & Security"},
	{secSecurity, "m_bAreSecurityZonesEnabled", settingBool, "True", "Security Zones Enabled", "Whether base security zones are enforced", "PvP & Security"},

	// Spice
	{secSpice, "m_PrimeRateInSeconds", settingFloat, "30.0", "Spice Prime Rate (s)", "Seconds between spice node priming ticks", "Spice"},
	{secSpice, "m_NodeValueToSpiceResourceRatio", settingFloat, "10.0", "Node Value to Spice Ratio", "Converts node value into harvestable spice", "Spice"},
	{secSpice, "m_bSpawningActive", settingBool, "True", "Spice Spawning Active", "Whether spice nodes spawn at all", "Spice"},
	{secSpice, "m_bPlayerMustWitnessBloom", settingBool, "True", "Player Must Witness Bloom", "Player must be present for bloom to count", "Spice"},

	// Taxation
	{secTaxation, "m_bTaxationEnabled", settingBool, "True", "Taxation Enabled", "Whether the taxation system is active", "Taxation"},
	{secTaxation, "m_TaxationCycleLengthSeconds", settingInt, "86400", "Taxation Cycle (s)", "Seconds between taxation collection cycles", "Taxation"},
	{secTaxation, "m_SpicePerHour", settingInt, "100", "Spice Yield per Hour", "Base spice generated per hour per field", "Taxation"},

	// Sandworm
	{secSandworm, "WormDetectionDistance", settingFloat, "5000.0", "Worm Detection Distance", "Distance at which worms detect players", "Sandworm"},
	{secSandworm, "m_MinWormSpawnInternal", settingFloat, "300.0", "Min Spawn Interval (s)", "Minimum seconds between worm spawns", "Sandworm"},
	{secSandworm, "m_MinDistanceBetweenSandworms", settingFloat, "10000.0", "Min Distance Between Worms", "Minimum world units separating active worms", "Sandworm"},
	{secSandworm, "m_SandwormQuicksandSpeedModifier", settingFloat, "0.5", "Quicksand Speed Modifier", "Movement speed multiplier in quicksand", "Sandworm"},
	{secSandworm, "m_GiantWormMinimumPlayersOnSpiceField", settingInt, "1", "Giant Worm Min Players", "Players required on field to trigger giant worm", "Sandworm"},
}

// ── INI helpers ───────────────────────────────────────────────────────────────

// parseINI returns map[section]map[key]value, ignoring comments and blank lines.
func parseINI(content string) map[string]map[string]string {
	sections := map[string]map[string]string{}
	var cur string
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			cur = line[1 : len(line)-1]
			if sections[cur] == nil {
				sections[cur] = map[string]string{}
			}
			continue
		}
		if eq := strings.Index(line, "="); eq > 0 && cur != "" {
			sections[cur][strings.TrimSpace(line[:eq])] = strings.TrimSpace(line[eq+1:])
		}
	}
	return sections
}

// parseINILines parses content and returns all non-schema / array lines grouped
// into RawSections. schemaKeys is a set of "section|key" strings to skip for
// plain (non-prefixed) lines.
func parseINILines(content, source string, schemaKeys map[string]bool) []RawSection {
	secMap := map[string]int{} // section name → index in result
	var result []RawSection
	curSec := ""

	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			curSec = line[1 : len(line)-1]
			if _, ok := secMap[curSec]; !ok {
				secMap[curSec] = len(result)
				result = append(result, RawSection{Section: curSec, Source: source})
			}
			continue
		}
		if curSec == "" {
			continue
		}
		prefix, rest := "", line
		if len(line) > 0 && (line[0] == '+' || line[0] == '-') {
			prefix = string(line[0])
			rest = line[1:]
		}
		eq := strings.Index(rest, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(rest[:eq])
		value := strings.TrimSpace(rest[eq+1:])

		// Include if it's an array line OR the key is not in the schema.
		if prefix != "" || !schemaKeys[curSec+"|"+key] {
			idx := secMap[curSec]
			result[idx].Lines = append(result[idx].Lines, RawLine{Prefix: prefix, Key: key, Value: value})
		}
	}

	// Drop sections with no lines.
	out := result[:0]
	for _, s := range result {
		if len(s.Lines) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// normalizeValue validates and normalises a user-supplied value for a given
// setting type, returning the canonical string ready for the INI file.
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
	return raw, nil
}

// shortSectionName strips the script/module prefix: "/Script/DuneSandbox.BuildingSettings" → "BuildingSettings".
func shortSectionName(section string) string {
	if i := strings.LastIndex(section, "."); i >= 0 {
		return section[i+1:]
	}
	return section
}

// inferType guesses a setting's type from its INI value string.
func inferType(value string) settingType {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "true" || lower == "false" {
		return settingBool
	}
	if _, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
		return settingInt
	}
	if _, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
		return settingFloat
	}
	return settingString
}

func iniDir() (string, error) {
	if serverIniDir != "" {
		return serverIniDir, nil
	}
	if loadedConfig.ServerIniDir != "" {
		return loadedConfig.ServerIniDir, nil
	}
	if globalControl != nil && globalExecutor != nil {
		return globalControl.DiscoverIniDir(context.Background(), globalExecutor)
	}
	return "", fmt.Errorf("server_ini_dir not configured")
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// defaultINICache holds DefaultGame.ini / DefaultEngine.ini content for the
// lifetime of the process. These files are part of the game image and don't
// change at runtime, so one read per process is sufficient.
var defaultINICache sync.Map // key: filename → string content

// readDefaultINIContent returns DefaultGame.ini or DefaultEngine.ini content,
// serving from the in-process cache after the first successful read.
func readDefaultINIContent(iniDir, filename string) string {
	if v, ok := defaultINICache.Load(filename); ok {
		return v.(string)
	}
	content := discoverDefaultINI(iniDir, filename)
	if content != "" {
		defaultINICache.Store(filename, content)
	}
	return content
}

// discoverDefaultINI does the actual multi-strategy search for a Default INI
// file. Search order:
//  1. Configured default_ini_dir (local read or via executor)
//  2. Common host paths — /home, /root, /dune first, then k3s containerd layers
//  3. Relative path traversal from iniDir (Hyper-V / bare-metal layouts)
//  4. kubectl/docker exec into the game container (requires pod running)
func discoverDefaultINI(iniDir, filename string) string {
	// 1. Explicit config path.
	if loadedConfig.DefaultIniDir != "" {
		p := filepath.Join(loadedConfig.DefaultIniDir, filename)
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			return string(data)
		}
		if c := readINIContent(p); c != "" {
			return c
		}
	}

	if globalExecutor != nil {
		// 2a. Well-known host directories — tried in order before any find.
		for _, p := range []string{
			"/home/dune/" + filename,
			"/home/" + filename,
			"/root/" + filename,
			"/dune/" + filename,
			"/home/dune/server/DuneSandbox/Config/" + filename,
		} {
			if c := readINIContent(p); c != "" {
				return c
			}
		}

		// 2b. Host filesystem scan: /home /root /dune first, then k3s containerd
		// paths. These require sudo but the executor already runs with sudo access.
		out, _ := globalExecutor.Exec(fmt.Sprintf(
			"sudo find /home /root /dune /run/k3s/containerd /var/lib/rancher/k3s/agent/containerd"+
				" -maxdepth 10 -name %s -not -path '*/Saved/*' -not -path '*/saved/*' 2>/dev/null | head -1",
			shellQuote(filename)))
		if p := strings.TrimSpace(out); p != "" {
			if c := readINIContent(p); c != "" {
				return c
			}
		}

		// 3. Relative candidates from iniDir (non-k8s layouts).
		for _, p := range []string{
			filepath.Join(iniDir, "..", "..", "..", "Config", filename),
			filepath.Join(iniDir, "..", "..", "Config", filename),
			filepath.Join(iniDir, "..", "..", "..", "..", "Config", filename),
		} {
			if c := readINIContent(p); c != "" {
				return c
			}
		}
	}

	// 4. Container exec fallback (kubectl / docker — requires container running).
	if globalControl != nil && globalExecutor != nil {
		if c := globalControl.ReadDefaultINI(context.Background(), globalExecutor, filename); c != "" {
			return c
		}
	}

	return ""
}

func readINIContent(path string) string {
	if globalExecutor == nil {
		return ""
	}
	out, err := globalExecutor.Exec(fmt.Sprintf("sudo cat %s 2>/dev/null", shellQuote(path)))
	if err != nil {
		return ""
	}
	if idx := strings.Index(out, "; >>>>> AMP: UserOverrides.ini appended below"); idx >= 0 {
		out = out[:idx]
	}
	return out
}

func handleGetServerSettings(w http.ResponseWriter, r *http.Request) {
	if globalExecutor == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	dir, err := iniDir()
	if err != nil {
		jsonErr(w, err, 503)
		return
	}

	gameContent          := readINIContent(dir + "/UserGame.ini")
	overrideContent      := readINIContent(dir + "/UserOverrides.ini")
	engineContent        := readINIContent(dir + "/UserEngine.ini")
	defaultGameContent   := readDefaultINIContent(dir, "DefaultGame.ini")
	defaultEngineContent := readDefaultINIContent(dir, "DefaultEngine.ini")

	gameIni          := parseINI(gameContent)
	overrideIni      := parseINI(overrideContent)
	engineIni        := parseINI(engineContent)
	defaultGameIni   := parseINI(defaultGameContent)
	defaultEngineIni := parseINI(defaultEngineContent)

	// Build schema key set for raw-line filtering.
	schemaKeys := map[string]bool{}
	for _, def := range serverSettingsSchema {
		schemaKeys[def.Section+"|"+def.Key] = true
	}

	// Schema settings — build layers from all INI sources and always include all entries.
	type layerSource struct {
		name string
		ini  map[string]map[string]string
	}
	layerSources := []layerSource{
		{"defaultEngine", defaultEngineIni},
		{"defaultGame", defaultGameIni},
		{"userEngine", engineIni},
		{"userGame", gameIni},
		{"userOverrides", overrideIni},
	}

	var settings []ServerSetting
	for _, def := range serverSettingsSchema {
		s := ServerSetting{
			Section:     def.Section,
			Key:         def.Key,
			Type:        string(def.Type),
			Default:     def.Default,
			Label:       def.Label,
			Description: def.Description,
			Category:    def.Category,
			Current:     def.Default,
		}
		for _, src := range layerSources {
			if v, ok := src.ini[def.Section][def.Key]; ok {
				s.Layers = append(s.Layers, SettingLayer{Source: src.name, Value: v})
				s.Current = v
				s.Source = src.name
			}
		}
		s.IsOverride = s.Source == "userOverrides"
		if s.Layers == nil {
			s.Layers = []SettingLayer{}
		}
		settings = append(settings, s)
	}

	// Discover all keys present in any INI file that aren't in the hardcoded schema.
	// These get typed settings with inferred types so they participate in the layer UI.
	type discoveredKey struct{ section, key string }
	seenKeys := make(map[string]bool, len(schemaKeys))
	for k := range schemaKeys {
		seenKeys[k] = true
	}
	var discovered []discoveredKey
	for _, src := range layerSources {
		for section, keys := range src.ini {
			for key := range keys {
				// Skip array-prefix lines (+/-); they belong in raw sections.
				if strings.HasPrefix(key, "+") || strings.HasPrefix(key, "-") {
					continue
				}
				k := section + "|" + key
				if !seenKeys[k] {
					seenKeys[k] = true
					discovered = append(discovered, discoveredKey{section, key})
				}
			}
		}
	}
	sort.Slice(discovered, func(i, j int) bool {
		if discovered[i].section != discovered[j].section {
			return discovered[i].section < discovered[j].section
		}
		return discovered[i].key < discovered[j].key
	})
	for _, dk := range discovered {
		s := ServerSetting{
			Section:  dk.section,
			Key:      dk.key,
			Label:    dk.key,
			Category: shortSectionName(dk.section),
			Layers:   []SettingLayer{},
		}
		for _, src := range layerSources {
			if v, ok := src.ini[dk.section][dk.key]; ok {
				if s.Type == "" {
					s.Type = string(inferType(v))
				}
				s.Layers = append(s.Layers, SettingLayer{Source: src.name, Value: v})
				s.Current = v
				s.Source = src.name
			}
		}
		if s.Type == "" {
			s.Type = string(settingString)
		}
		s.IsOverride = s.Source == "userOverrides"
		settings = append(settings, s)
		schemaKeys[dk.section+"|"+dk.key] = true
	}

	// Raw lines — array entries and anything not promoted to a typed setting.
	// All five files use the same schemaKeys (which now includes discovered keys)
	// so nothing appears in both the typed panel and the raw panel.
	var raw []RawSection
	raw = append(raw, parseINILines(defaultGameContent, "defaultGame", schemaKeys)...)
	raw = append(raw, parseINILines(defaultEngineContent, "defaultEngine", schemaKeys)...)
	raw = append(raw, parseINILines(gameContent, "userGame", schemaKeys)...)
	raw = append(raw, parseINILines(overrideContent, "userOverrides", schemaKeys)...)
	raw = append(raw, parseINILines(engineContent, "userEngine", schemaKeys)...)

	jsonOK(w, map[string]any{
		"settings": settings,
		"raw":      raw,
	})
}

// patchINI applies key updates to raw INI text without disturbing array lines
// (+/- prefix) or any other content that isn't being changed.
// updates: section → key → newValue ("" means delete the key).
func patchINI(content string, updates map[string]map[string]string) string {
	if len(updates) == 0 {
		return content
	}

	applied := map[string]map[string]bool{}
	for sec := range updates {
		applied[sec] = map[string]bool{}
	}
	seenSection := map[string]bool{}

	rawLines := strings.Split(strings.TrimRight(content, "\r\n"), "\n")
	curSec := ""
	var out []string
	sectionBodyEnd := map[string]int{} // section → index of last substantive line in out

	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			curSec = trimmed[1 : len(trimmed)-1]
			seenSection[curSec] = true
			out = append(out, line)
			sectionBodyEnd[curSec] = len(out) - 1
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			out = append(out, line)
			continue
		}
		// Array line — always preserve unchanged.
		if trimmed[0] == '+' || trimmed[0] == '-' {
			out = append(out, line)
			if curSec != "" {
				sectionBodyEnd[curSec] = len(out) - 1
			}
			continue
		}
		// Plain k=v.
		eq := strings.Index(trimmed, "=")
		if eq <= 0 {
			out = append(out, line)
			if curSec != "" {
				sectionBodyEnd[curSec] = len(out) - 1
			}
			continue
		}
		key := strings.TrimSpace(trimmed[:eq])
		if kvs, inUpdate := updates[curSec]; inUpdate {
			if newVal, ok := kvs[key]; ok {
				applied[curSec][key] = true
				if newVal != "" {
					out = append(out, key+"="+newVal)
					sectionBodyEnd[curSec] = len(out) - 1
				}
				continue // "" = delete: skip line
			}
		}
		out = append(out, line)
		if curSec != "" {
			sectionBodyEnd[curSec] = len(out) - 1
		}
	}

	// Collect unapplied new keys, grouped by insertion point.
	type ins struct {
		afterIdx int
		lines    []string
	}
	insMap := map[int]*ins{}
	var newSections []string
	seenNewSec := map[string]bool{}

	for sec, kvs := range updates {
		var toAdd []string
		for k, v := range kvs {
			if v != "" && !applied[sec][k] {
				toAdd = append(toAdd, k+"="+v)
			}
		}
		if len(toAdd) == 0 {
			continue
		}
		sort.Strings(toAdd)
		if seenSection[sec] {
			idx := sectionBodyEnd[sec]
			i := insMap[idx]
			if i == nil {
				i = &ins{afterIdx: idx}
				insMap[idx] = i
			}
			i.lines = append(i.lines, toAdd...)
		} else if !seenNewSec[sec] {
			seenNewSec[sec] = true
			newSections = append(newSections, sec)
		}
	}

	// Insert new keys into existing sections (reverse order to maintain indices).
	if len(insMap) > 0 {
		type idxLines struct {
			idx   int
			lines []string
		}
		var sorted []idxLines
		for idx, i := range insMap {
			sorted = append(sorted, idxLines{idx, i.lines})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].idx > sorted[j].idx })
		for _, il := range sorted {
			tail := make([]string, len(out[il.idx+1:]))
			copy(tail, out[il.idx+1:])
			out = out[:il.idx+1]
			out = append(out, il.lines...)
			out = append(out, tail...)
		}
	}

	// Append new sections at end.
	sort.Strings(newSections)
	for _, sec := range newSections {
		out = append(out, "")
		out = append(out, "["+sec+"]")
		kvs := updates[sec]
		var keys []string
		for k, v := range kvs {
			if v != "" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, k+"="+kvs[k])
		}
	}

	return strings.Join(out, "\n") + "\n"
}

func handleUpdateServerSettings(w http.ResponseWriter, r *http.Request) {
	if globalExecutor == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	dir, err := iniDir()
	if err != nil {
		jsonErr(w, err, 503)
		return
	}

	var req struct {
		Updates []serverSettingUpdate `json:"updates"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}

	// Build schema lookup for type validation.
	schemaMap := map[string]settingDef{}
	for _, d := range serverSettingsSchema {
		schemaMap[d.Section+"|"+d.Key] = d
	}

	// Normalise values and build the update map.
	updates := map[string]map[string]string{}
	applied, cleared := 0, 0
	for _, u := range req.Updates {
		if updates[u.Section] == nil {
			updates[u.Section] = map[string]string{}
		}
		if u.Value == "" {
			updates[u.Section][u.Key] = ""
			cleared++
		} else {
			def, known := schemaMap[u.Section+"|"+u.Key]
			if known {
				norm, err := normalizeValue(def.Type, u.Value)
				if err != nil {
					jsonErr(w, fmt.Errorf("invalid value for %s: %w", u.Key, err), 400)
					return
				}
				updates[u.Section][u.Key] = norm
			} else {
				updates[u.Section][u.Key] = u.Value
			}
			applied++
		}
	}

	overridePath := dir + "/UserOverrides.ini"
	patched := patchINI(readINIContent(overridePath), updates)
	if err := globalExecutor.WriteFile(overridePath, strings.NewReader(patched)); err != nil {
		jsonErr(w, fmt.Errorf("write UserOverrides.ini: %w", err), 500)
		return
	}

	jsonOK(w, map[string]any{
		"ok":      fmt.Sprintf("Saved (%d set, %d cleared). Restart the game server to apply.", applied, cleared),
		"applied": applied,
		"cleared": cleared,
	})
}

// handleUpdateRawSection replaces a single INI section in UserOverrides.ini
// with the caller-supplied raw lines. All edits go to UserOverrides.ini
// regardless of source — it is the final overlay layer for all settings.
func handleUpdateRawSection(w http.ResponseWriter, r *http.Request) {
	if globalExecutor == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}
	dir, err := iniDir()
	if err != nil {
		jsonErr(w, err, 503)
		return
	}

	var req struct {
		Section string `json:"section"` // INI section name (without brackets)
		Lines   string `json:"lines"`   // raw INI lines for this section (no header)
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}

	filePath := dir + "/UserOverrides.ini"

	existing := readINIContent(filePath)
	updated := replaceSectionContent(existing, req.Section, strings.TrimSpace(req.Lines))

	var buf bytes.Buffer
	buf.WriteString(updated)
	if err := globalExecutor.WriteFile(filePath, &buf); err != nil {
		jsonErr(w, fmt.Errorf("write %s: %w", filePath, err), 500)
		return
	}
	jsonOK(w, map[string]string{"ok": "Saved. Restart the game server to apply."})
}

// replaceSectionContent replaces or appends a section's array lines in raw INI
// content, preserving plain k=v lines in the same section (those are managed by
// the typed-settings panel). Other sections are preserved exactly.
// When newLines is empty the entire section is removed.
func replaceSectionContent(content, section, newLines string) string {
	header := "[" + section + "]"
	remove := newLines == ""
	var out []string
	var existingKV []string // plain k=v lines from the target section
	inTarget := false
	sectionFound := false

	flushTarget := func() {
		if remove {
			return
		}
		// Emit preserved k=v lines before the new array content.
		out = append(out, existingKV...)
		if newLines != "" {
			out = append(out, newLines)
		}
		out = append(out, "")
	}

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		isHeader := strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")

		if isHeader {
			if inTarget {
				flushTarget()
				inTarget = false
				existingKV = nil
			}
			if trimmed == header {
				inTarget = true
				sectionFound = true
				if !remove {
					out = append(out, line)
				}
				continue
			}
		}

		if inTarget {
			// Collect plain k=v lines to re-emit; skip array lines (+/-).
			if trimmed != "" && !strings.HasPrefix(trimmed, ";") && !strings.HasPrefix(trimmed, "#") &&
				len(trimmed) > 0 && trimmed[0] != '+' && trimmed[0] != '-' {
				if strings.Index(trimmed, "=") > 0 {
					existingKV = append(existingKV, line)
				}
			}
			continue
		}

		out = append(out, line)
	}

	if inTarget {
		flushTarget()
	} else if !sectionFound && !remove {
		// Section doesn't exist yet — append it.
		if len(out) > 0 && out[len(out)-1] != "" {
			out = append(out, "")
		}
		out = append(out, header)
		if newLines != "" {
			out = append(out, newLines)
		}
		out = append(out, "")
	}

	return strings.Join(out, "\n")
}
