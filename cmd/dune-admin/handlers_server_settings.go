package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
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
	Source      string         `json:"source"` // "userGame"|"userEngine"|"defaultGame"|"defaultEngine"|""
	Layers      []SettingLayer `json:"layers"` // ordered low→high priority; empty when setting is unconfigured
}

// SettingLayer records one file's contribution to a setting's value,
// ordered low → high priority in the Layers slice.
type SettingLayer struct {
	Source string `json:"source"` // "defaultEngine"|"defaultGame"|"userEngine"|"userGame"
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
	Source  string    `json:"source"` // "userGame"|"userEngine"|"defaultGame"|"defaultEngine"
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

// parseINIRaw parses raw INI lines preserving +/- prefixes as part of the key
// (e.g. "+ActiveMod=SomeMod" is stored as key "+ActiveMod"). Duplicate keys
// (common for UE array entries like multiple "+ActiveMod=" lines) are stored
// with a unique null-byte suffix ("\x00N") so they survive the map round-trip.
// renderDuneAdminBlock strips the suffix when writing the file.
func parseINIRaw(content string) map[string]map[string]string {
	sections := map[string]map[string]string{}
	counts := map[string]map[string]int{} // section → key → occurrence count
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
				counts[cur] = map[string]int{}
			}
			continue
		}
		if cur == "" {
			continue
		}
		rest := line
		prefix := ""
		if len(line) > 0 && (line[0] == '+' || line[0] == '-') {
			prefix = string(line[0])
			rest = line[1:]
		}
		if eq := strings.Index(rest, "="); eq > 0 {
			baseKey := prefix + strings.TrimSpace(rest[:eq])
			val := strings.TrimSpace(rest[eq+1:])
			n := counts[cur][baseKey]
			counts[cur][baseKey] = n + 1
			// First occurrence uses the plain key; subsequent ones get "\x00N" suffix.
			storeKey := baseKey
			if n > 0 {
				storeKey = fmt.Sprintf("%s\x00%d", baseKey, n)
			}
			sections[cur][storeKey] = val
		}
	}
	return sections
}

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
//  2. If iniDir is k8s://..., derive nearby Config paths in the same pod
//  3. Common host paths — /home, /root, /dune first, then k3s containerd layers
//  4. Relative path traversal from iniDir (Hyper-V / bare-metal layouts)
//  5. kubectl/docker exec into the game container (requires pod running)
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

	// 1b. When INI dir points to a k8s pod path, derive nearby Config paths in
	// the same pod first. This is the most reliable source in deployed mode.
	if ns, pod, inPodDir, ok := parseK8sINIPath(iniDir); ok {
		candidates := []string{
			pathpkg.Clean(pathpkg.Join(inPodDir, "..", "..", "..", "Config", filename)),
			pathpkg.Clean(pathpkg.Join(inPodDir, "..", "..", "Config", filename)),
			pathpkg.Clean(pathpkg.Join(inPodDir, "..", "..", "..", "..", "Config", filename)),
			"/DuneSandbox/Config/" + filename,
			"/home/dune/server/DuneSandbox/Config/" + filename,
			"/home/dune/DuneSandbox/Config/" + filename,
			"/game/DuneSandbox/Config/" + filename,
		}
		for _, inPodPath := range candidates {
			if c := readINIContent(fmt.Sprintf("k8s://%s/%s%s", ns, pod, inPodPath)); c != "" {
				return c
			}
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

func parseK8sINIPath(path string) (ns, pod, inPodPath string, ok bool) {
	const prefix = "k8s://"
	if !strings.HasPrefix(path, prefix) {
		return "", "", "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	ns, pod, inPodPath = parts[0], parts[1], "/"+strings.TrimLeft(parts[2], "/")
	return ns, pod, inPodPath, true
}

func readINIContent(path string) string {
	if globalExecutor == nil {
		return ""
	}
	if ns, pod, inPodPath, ok := parseK8sINIPath(path); ok {
		kctl := kubectlCLI(globalExecutor)
		out, err := globalExecutor.Exec(fmt.Sprintf(
			"%s exec -n %s %s -- cat %s 2>/dev/null",
			kctl, ns, pod, shellQuote(inPodPath)))
		if err != nil {
			return ""
		}
		return out
	}
	out, err := globalExecutor.Exec(fmt.Sprintf("sudo cat %s 2>/dev/null", shellQuote(path)))
	if err != nil {
		return ""
	}
	return out
}

type layerSource struct {
	name string
	ini  map[string]map[string]string
}

type discoveredKey struct {
	section string
	key     string
}

func serverSettingsSchemaKeys() map[string]bool {
	schemaKeys := make(map[string]bool, len(serverSettingsSchema))
	for _, def := range serverSettingsSchema {
		schemaKeys[def.Section+"|"+def.Key] = true
	}
	return schemaKeys
}

func buildLayerSources(
	defaultEngineIni,
	defaultGameIni,
	engineIni,
	gameIni map[string]map[string]string,
) []layerSource {
	return []layerSource{
		{name: "defaultEngine", ini: defaultEngineIni},
		{name: "defaultGame", ini: defaultGameIni},
		{name: "userEngine", ini: engineIni},
		{name: "userGame", ini: gameIni},
	}
}

func applySettingLayers(s *ServerSetting, layerSources []layerSource) {
	for _, src := range layerSources {
		if v, ok := src.ini[s.Section][s.Key]; ok {
			if s.Type == "" {
				s.Type = string(inferType(v))
			}
			s.Layers = append(s.Layers, SettingLayer{Source: src.name, Value: v})
			s.Current = v
			s.Source = src.name
		}
	}
	s.IsOverride = strings.HasPrefix(s.Source, "user")
	if s.Layers == nil {
		s.Layers = []SettingLayer{}
	}
}

func buildSchemaSettings(layerSources []layerSource) []ServerSetting {
	settings := make([]ServerSetting, 0, len(serverSettingsSchema))
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
		applySettingLayers(&s, layerSources)
		settings = append(settings, s)
	}
	return settings
}

func discoverUnknownSettings(layerSources []layerSource, schemaKeys map[string]bool) []discoveredKey {
	seenKeys := make(map[string]bool, len(schemaKeys))
	for k := range schemaKeys {
		seenKeys[k] = true
	}

	discovered := make([]discoveredKey, 0, 32)
	for _, src := range layerSources {
		for section, keys := range src.ini {
			for key := range keys {
				// Skip array-prefix lines (+/-); they belong in raw sections.
				if strings.HasPrefix(key, "+") || strings.HasPrefix(key, "-") {
					continue
				}
				composite := section + "|" + key
				if seenKeys[composite] {
					continue
				}
				seenKeys[composite] = true
				discovered = append(discovered, discoveredKey{section: section, key: key})
			}
		}
	}
	sort.Slice(discovered, func(i, j int) bool {
		if discovered[i].section != discovered[j].section {
			return discovered[i].section < discovered[j].section
		}
		return discovered[i].key < discovered[j].key
	})
	return discovered
}

func buildDiscoveredSettings(
	discovered []discoveredKey,
	layerSources []layerSource,
	schemaKeys map[string]bool,
) []ServerSetting {
	settings := make([]ServerSetting, 0, len(discovered))
	for _, dk := range discovered {
		s := ServerSetting{
			Section:  dk.section,
			Key:      dk.key,
			Label:    dk.key,
			Category: shortSectionName(dk.section),
			Layers:   []SettingLayer{},
		}
		applySettingLayers(&s, layerSources)
		if s.Type == "" {
			s.Type = string(settingString)
		}
		settings = append(settings, s)
		schemaKeys[dk.section+"|"+dk.key] = true
	}
	return settings
}

func buildServerSettingsRawSections(
	defaultGameContent,
	defaultEngineContent,
	gameContent,
	engineContent string,
	schemaKeys map[string]bool,
) []RawSection {
	raw := make([]RawSection, 0, 16)
	raw = append(raw, parseINILines(defaultGameContent, "defaultGame", schemaKeys)...)
	raw = append(raw, parseINILines(defaultEngineContent, "defaultEngine", schemaKeys)...)
	raw = append(raw, parseINILines(gameContent, "userGame", schemaKeys)...)
	raw = append(raw, parseINILines(engineContent, "userEngine", schemaKeys)...)
	return raw
}

func writeINIContent(path, body string) error {
	if globalExecutor == nil {
		return fmt.Errorf("not connected")
	}
	if ns, pod, inPodPath, ok := parseK8sINIPath(path); ok {
		kctl := kubectlCLI(globalExecutor)
		payload := base64.StdEncoding.EncodeToString([]byte(body))
		cmd := fmt.Sprintf(
			"echo %s | base64 -d | %s exec -i -n %s %s -- sh -lc 'cat > %s' 2>/dev/null",
			shellQuote(payload), kctl, ns, pod, shellQuote(inPodPath),
		)
		out, err := globalExecutor.Exec(cmd)
		if err != nil {
			return fmt.Errorf("write %s: %w — %s", inPodPath, err, strings.TrimSpace(out))
		}
		return nil
	}
	return globalExecutor.WriteFile(path, strings.NewReader(body))
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

	gameContent := readINIContent(dir + "/UserGame.ini")
	engineContent := readINIContent(dir + "/UserEngine.ini")
	defaultGameContent := readDefaultINIContent(dir, "DefaultGame.ini")
	defaultEngineContent := readDefaultINIContent(dir, "DefaultEngine.ini")

	gameIni := parseINI(gameContent)
	engineIni := parseINI(engineContent)
	defaultGameIni := parseINI(defaultGameContent)
	defaultEngineIni := parseINI(defaultEngineContent)

	layerSources := buildLayerSources(defaultEngineIni, defaultGameIni, engineIni, gameIni)
	schemaKeys := serverSettingsSchemaKeys()
	settings := buildSchemaSettings(layerSources)
	discovered := discoverUnknownSettings(layerSources, schemaKeys)
	settings = append(settings, buildDiscoveredSettings(discovered, layerSources, schemaKeys)...)
	raw := buildServerSettingsRawSections(defaultGameContent, defaultEngineContent, gameContent, engineContent, schemaKeys)

	jsonOK(w, map[string]any{
		"settings": settings,
		"raw":      raw,
	})
}

// patchINI applies key updates to raw INI text without disturbing array lines
// (+/- prefix) or any other content that isn't being changed.
// updates: section → key → newValue ("" means delete the key).
// dune-admin manages its writes inside a single delimited region at the end of
// the INI file. Hand-edits above the BEGIN marker are preserved verbatim. UE5's
// "last-key-wins" semantics make the dune-admin region authoritative for any
// keys it sets, even if they're also set in the hand-edited region above.
const (
	duneAdminBeginMarker = "; >>>>> dune-admin managed section BEGIN — do not hand-edit between these markers >>>>>"
	duneAdminEndMarker   = "; <<<<< dune-admin managed section END <<<<<"
)

// splitAtDuneAdminMarker separates an INI file into the hand-edited prefix and
// any previously-written dune-admin managed region. The managed region is
// parsed back into section→key→value so that incoming updates can be merged
// before re-rendering.
//
// If no marker is found, the entire content is treated as hand-edited and
// managed comes back empty.
//
// Returns an error when a BEGIN marker is present but the matching END marker
// is absent — this indicates a truncated or manually-corrupted file. Callers
// must not proceed with a save in this case, as doing so would silently drop
// all previously-managed settings.
func splitAtDuneAdminMarker(content string) (preMarker string, managed map[string]map[string]string, err error) {
	managed = map[string]map[string]string{}
	idx := strings.Index(content, duneAdminBeginMarker)
	if idx < 0 {
		return content, managed, nil
	}
	preMarker = strings.TrimRight(content[:idx], "\n")
	if preMarker != "" {
		preMarker += "\n"
	}

	rest := content[idx:]
	endIdx := strings.Index(rest, duneAdminEndMarker)
	if endIdx < 0 {
		return "", nil, fmt.Errorf("dune-admin managed section BEGIN marker found but END marker is missing — the file may be truncated; refusing to overwrite to avoid data loss")
	}
	block := rest[len(duneAdminBeginMarker):endIdx]
	for sec, kvs := range parseINIRaw(block) {
		managed[sec] = kvs
	}
	return preMarker, managed, nil
}

// applyManagedUpdates merges the incoming updates into the existing managed
// state. Empty values delete keys; sections that end up empty are dropped.
func applyManagedUpdates(managed map[string]map[string]string, updates map[string]map[string]string) {
	for sec, kvs := range updates {
		if managed[sec] == nil {
			managed[sec] = map[string]string{}
		}
		for k, v := range kvs {
			if v == "" {
				delete(managed[sec], k)
			} else {
				managed[sec][k] = v
			}
		}
		if len(managed[sec]) == 0 {
			delete(managed, sec)
		}
	}
}

func managedKeyParts(k string) (base string, idx int, hasSuffix bool) {
	base = k
	if nul := strings.IndexByte(k, '\x00'); nul >= 0 {
		base = k[:nul]
		if n, err := strconv.Atoi(k[nul+1:]); err == nil {
			return base, n, true
		}
		return base, 0, true
	}
	return base, 0, false
}

// renderDuneAdminBlock emits the marker-delimited managed region. Sections are
// sorted alphabetically. Keys are sorted with \x00N-suffixed duplicates
// collated after their base key in numeric order; the suffix is stripped before
// writing so the output file contains the original key name.
func renderDuneAdminBlock(managed map[string]map[string]string) string {
	if len(managed) == 0 {
		return ""
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	var b strings.Builder
	b.WriteString(duneAdminBeginMarker + "\n")
	b.WriteString("; Managed by dune-admin (https://github.com/Icehunter/dune-admin)\n")
	b.WriteString("; Keys below are owned by the dune-admin web UI. UE5 reads the file\n")
	b.WriteString("; top-to-bottom with last-key-wins semantics, so values here override\n")
	b.WriteString("; anything set above. Last write: " + timestamp + "\n")
	b.WriteString(";\n")

	secs := make([]string, 0, len(managed))
	for s := range managed {
		secs = append(secs, s)
	}
	sort.Strings(secs)
	for _, sec := range secs {
		b.WriteString("\n[" + sec + "]\n")
		keys := make([]string, 0, len(managed[sec]))
		for k := range managed[sec] {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			baseI, idxI, dupI := managedKeyParts(keys[i])
			baseJ, idxJ, dupJ := managedKeyParts(keys[j])
			if baseI != baseJ {
				return baseI < baseJ
			}
			if dupI != dupJ {
				// The unsuffixed key sorts before duplicates.
				return !dupI
			}
			if dupI && idxI != idxJ {
				return idxI < idxJ
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			// Strip the \x00N dedup suffix before writing — it is internal only.
			displayKey := k
			if idx := strings.IndexByte(k, '\x00'); idx >= 0 {
				displayKey = k[:idx]
			}
			b.WriteString(displayKey + "=" + managed[sec][k] + "\n")
		}
	}
	b.WriteString("\n" + duneAdminEndMarker + "\n")
	return b.String()
}

// legacyHeaderSentinel matches the comment block emitted by the brief
// "header-at-top-of-file" version of dune-admin (commits between the initial
// merge and the managed-region rewrite). The new code never emits this; the
// strip runs as a one-time migration when the legacy block is encountered.
const legacyHeaderSentinel = "Managed by dune-admin (https://github.com/Icehunter/dune-admin)"

// stripLegacyHeader removes the orphaned top-of-file comment block written by
// the earlier "header-only" build. It matches a block of the form:
//
//	; ====...
//	; Managed by dune-admin (https://github.com/Icehunter/dune-admin)
//	; ...optional comment lines...
//	; ====...
//
// Anything that doesn't match this exact shape is left alone, so user-written
// comment blocks that happen to start with `; ====` are safe.
func stripLegacyHeader(content string) string {
	if !strings.Contains(content, legacyHeaderSentinel) {
		return content
	}
	lines := strings.Split(content, "\n")
	start := -1
	for i, l := range lines {
		trim := strings.TrimSpace(l)
		if !strings.HasPrefix(trim, "; ====") {
			continue
		}
		if start == -1 {
			start = i
			continue
		}
		if stripped, ok := tryStripBlock(lines, start, i); ok {
			return stripped
		}
		// Not our block — keep scanning. Reset to look for a fresh opening "; ====".
		start = i
	}
	return content
}

// tryStripBlock checks whether lines[start..end] contains legacyHeaderSentinel
// and, if so, returns the content with that block removed.
func tryStripBlock(lines []string, start, end int) (string, bool) {
	for j := start; j <= end; j++ {
		if strings.Contains(lines[j], legacyHeaderSentinel) {
			stripEnd := end + 1
			if stripEnd < len(lines) && strings.TrimSpace(lines[stripEnd]) == "" {
				stripEnd++
			}
			return strings.Join(append(append([]string{}, lines[:start]...), lines[stripEnd:]...), "\n"), true
		}
	}
	return "", false
}

// stripEmptySections removes `[section]` headers whose bodies are entirely
// whitespace. A body is "empty" only when it contains no comments, no array
// entries, and no k=v lines — just blank lines. This preserves hand-written
// comments and documentation, removing only the truly orphaned section
// headers left behind after dedup.
func stripEmptySections(content string) string {
	if content == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	// Find each section's (headerIdx, nextHeaderIdx). For each, check whether
	// the body is empty; if so, mark the header line for removal.
	skip := make(map[int]bool)
	var headerIdxs []int
	for i, l := range lines {
		trim := strings.TrimSpace(l)
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			headerIdxs = append(headerIdxs, i)
		}
	}
	for k, idx := range headerIdxs {
		end := len(lines)
		if k+1 < len(headerIdxs) {
			end = headerIdxs[k+1]
		}
		empty := true
		for j := idx + 1; j < end; j++ {
			if strings.TrimSpace(lines[j]) != "" {
				empty = false
				break
			}
		}
		if empty {
			skip[idx] = true
			// Also consume the trailing blank lines so we don't leave a void.
			for j := idx + 1; j < end; j++ {
				skip[j] = true
			}
		}
	}
	if len(skip) == 0 {
		return content
	}
	out := make([]string, 0, len(lines))
	for i, l := range lines {
		if skip[i] {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// stripKeysFromContent removes the listed keys from their sections in content.
// Both plain k=v and prefixed array lines (+k=v, -k=v) are matched: if "+Foo"
// or "-Foo" appears in owned, any line with that exact prefixed key is removed;
// if plain "Foo" is owned, all variants (plain, +Foo, -Foo) are removed.
// Comments and unrelated lines are left alone.
func stripKeysFromContent(content string, owned map[string]map[string]bool) string {
	if len(owned) == 0 || content == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	curSec := ""
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		// Section header.
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			curSec = trim[1 : len(trim)-1]
			out = append(out, line)
			continue
		}
		// Drop lines where the (possibly-prefixed) key is dune-admin-owned.
		// Comments, blank lines, and section headers are preserved as-is.
		if curSec != "" && len(trim) > 0 && trim[0] != ';' && trim[0] != '#' {
			rest := trim
			pfx := ""
			if trim[0] == '+' || trim[0] == '-' {
				pfx = string(trim[0])
				rest = trim[1:]
			}
			if eq := strings.Index(rest, "="); eq > 0 {
				baseKey := strings.TrimSpace(rest[:eq])
				lineKey := pfx + baseKey
				secOwned := owned[curSec]
				// Drop if: exact prefixed key owned, OR plain base key owned (covers all variants).
				if secOwned != nil && (secOwned[lineKey] || secOwned[baseKey]) {
					continue
				}
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// ownedKeySet builds the (section → set-of-base-keys) lookup that dune-admin
// owns, so we can strip duplicates from the hand-edited region.
// \x00N dedup suffixes are stripped so ownership matches on the base key name.
func ownedKeySet(managed map[string]map[string]string) map[string]map[string]bool {
	owned := make(map[string]map[string]bool, len(managed))
	for sec, kvs := range managed {
		set := make(map[string]bool, len(kvs))
		for k := range kvs {
			base := k
			if idx := strings.IndexByte(k, '\x00'); idx >= 0 {
				base = k[:idx]
			}
			set[base] = true
		}
		owned[sec] = set
	}
	return owned
}

// applyDuneAdminUpdates rewrites an INI file's dune-admin managed region with
// the incoming updates merged in. Hand-edited content above the BEGIN marker
// is preserved exactly EXCEPT for keys dune-admin now owns — those are stripped
// from above so the file has exactly one definition per dune-admin-owned key.
// If after merging the managed region would be empty, the markers are dropped
// and the pre-marker content is left intact.
func applyDuneAdminUpdates(content string, updates map[string]map[string]string) (string, error) {
	// One-time migration: strip the orphaned top-of-file header from the
	// earlier "header-only" build, if present.
	content = stripLegacyHeader(content)

	preMarker, managed, err := splitAtDuneAdminMarker(content)
	if err != nil {
		return "", err
	}
	applyManagedUpdates(managed, updates)
	block := renderDuneAdminBlock(managed)
	if block == "" {
		// No managed keys left — return just the hand-edited prefix.
		return stripEmptySections(preMarker), nil
	}
	// Remove pre-marker copies of keys dune-admin now owns to prevent
	// duplicates. Hand-edited keys dune-admin doesn't own stay untouched.
	preMarker = stripKeysFromContent(preMarker, ownedKeySet(managed))
	// Drop any section headers whose bodies became empty after dedup.
	preMarker = stripEmptySections(preMarker)
	// Ensure exactly one blank line between hand-edited content and the marker.
	if strings.TrimSpace(preMarker) == "" {
		return block, nil
	}
	return strings.TrimRight(preMarker, "\n") + "\n\n" + block, nil
}

// applyDuneAdminRawSection rewrites a single section's content inside the
// dune-admin managed region without touching anything else. Used by the raw
// (array-line) section editor. Any keys dune-admin now owns in the supplied
// section are stripped from the hand-edited region above the marker so the file
// has one authoritative definition per owned key.
func applyDuneAdminRawSection(content, section, rawLines string) (string, error) {
	content = stripLegacyHeader(content)
	preMarker, managed, err := splitAtDuneAdminMarker(content)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(rawLines) == "" {
		delete(managed, section)
	} else {
		// Use parseINIRaw to preserve +/- prefixes as part of the key so array
		// entries round-trip correctly through the managed block.
		parsed := parseINIRaw("[" + section + "]\n" + rawLines)
		managed[section] = parsed[section]
		if managed[section] == nil {
			managed[section] = map[string]string{}
		}
	}
	block := renderDuneAdminBlock(managed)
	if block == "" {
		return stripEmptySections(preMarker), nil
	}
	preMarker = stripKeysFromContent(preMarker, ownedKeySet(managed))
	preMarker = stripEmptySections(preMarker)
	if strings.TrimSpace(preMarker) == "" {
		return block, nil
	}
	return strings.TrimRight(preMarker, "\n") + "\n\n" + block, nil
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

	// Route each section to UserGame.ini or UserEngine.ini based on which default
	// file declares it. Sections found in DefaultEngine.ini go to UserEngine.ini;
	// everything else goes to UserGame.ini.
	defaultEngineIni := parseINI(readDefaultINIContent(dir, "DefaultEngine.ini"))
	gameUpdates, engineUpdates := map[string]map[string]string{}, map[string]map[string]string{}
	for sec, kvs := range updates {
		if _, inEngine := defaultEngineIni[sec]; inEngine {
			engineUpdates[sec] = kvs
		} else {
			gameUpdates[sec] = kvs
		}
	}
	if len(gameUpdates) > 0 {
		path := dir + "/UserGame.ini"
		body, err := applyDuneAdminUpdates(readINIContent(path), gameUpdates)
		if err != nil {
			jsonErr(w, fmt.Errorf("UserGame.ini: %w", err), 409)
			return
		}
		if err := writeINIContent(path, body); err != nil {
			jsonErr(w, fmt.Errorf("write UserGame.ini: %w", err), 500)
			return
		}
	}
	if len(engineUpdates) > 0 {
		path := dir + "/UserEngine.ini"
		body, err := applyDuneAdminUpdates(readINIContent(path), engineUpdates)
		if err != nil {
			jsonErr(w, fmt.Errorf("UserEngine.ini: %w", err), 409)
			return
		}
		if err := writeINIContent(path, body); err != nil {
			jsonErr(w, fmt.Errorf("write UserEngine.ini: %w", err), 500)
			return
		}
	}

	jsonOK(w, map[string]any{
		"ok":      fmt.Sprintf("Saved (%d set, %d cleared). Restart the game server to apply.", applied, cleared),
		"applied": applied,
		"cleared": cleared,
	})
}

// handleUpdateRawSection replaces a single INI section in the appropriate user
// INI file. Sections declared in DefaultEngine.ini are written to UserEngine.ini;
// all others are written to UserGame.ini.
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

	defaultEngineIni := parseINI(readDefaultINIContent(dir, "DefaultEngine.ini"))
	var filePath string
	if _, inEngine := defaultEngineIni[req.Section]; inEngine {
		filePath = dir + "/UserEngine.ini"
	} else {
		filePath = dir + "/UserGame.ini"
	}

	existing := readINIContent(filePath)
	updated, err := applyDuneAdminRawSection(existing, req.Section, strings.TrimSpace(req.Lines))
	if err != nil {
		jsonErr(w, fmt.Errorf("%s: %w", filePath, err), 409)
		return
	}

	if err := writeINIContent(filePath, updated); err != nil {
		jsonErr(w, fmt.Errorf("write %s: %w", filePath, err), 500)
		return
	}
	jsonOK(w, map[string]string{"ok": "Saved. Restart the game server to apply."})
}
