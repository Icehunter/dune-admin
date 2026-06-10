package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
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
	// FieldName is the AMP config node suffix and the canonical identifier for a
	// setting, e.g. "ConsoleVariables.Dune.GlobalMiningOutputMultiplier". Section
	// and Key are derived from it via fieldNameToSectionKey so they cannot drift.
	FieldName   string
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
	// FieldName is the AMP config node suffix for curated settings; empty for
	// discovered ones. Its presence marks a setting as AMP-managed (written via
	// the AMP API under the AMP control plane) so the UI can label it.
	FieldName string `json:"field_name,omitempty"`
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

// INI section paths shared by the curated schema. These match what
// fieldNameToSectionKey derives from each setting's FieldName.
const (
	secConsoleVars = "ConsoleVariables"
	secStorm       = "/Script/DuneSandbox.SandStormConfig"
	secBuilding    = "/Script/DuneSandbox.BuildingSettings"
	secPvP         = "/Script/DuneSandbox.PvpPveSettings"
	secSecurity    = "/Script/DuneSandbox.SecurityZonesSubsystem"
	secDurab       = "/DeteriorationSystem.ItemDeteriorationConstants"
)

// Curated categories. AMP groups these under Subcategories; we use flat
// categories the frontend renders as collapsible groups.
const (
	catMultipliers    = "Multipliers"
	catWorldCombat    = "World & Combat"
	catPersistence    = "Persistence & Building"
	catServerIdentity = "Server Identity"
)

// fieldNameToSectionKey decomposes an AMP FieldName into its INI (section, key).
//
//   - ConsoleVariables CVars live under a single [ConsoleVariables] section with
//     the dotted CVar name as the key (split on the FIRST dot):
//     "ConsoleVariables.Dune.GlobalMiningOutputMultiplier"
//     → ("ConsoleVariables", "Dune.GlobalMiningOutputMultiplier")
//   - UPROPERTY fields use the class path as the section and the trailing member
//     as the key (split on the LAST dot):
//     "/Script/DuneSandbox.BuildingSettings.m_MaxNumLandclaimSegments"
//     → ("/Script/DuneSandbox.BuildingSettings", "m_MaxNumLandclaimSegments")
//
// A bare AMP-orchestration field (e.g. "WorldTitle") has no INI representation
// and returns ("WorldTitle", "").
func fieldNameToSectionKey(fieldName string) (section, key string) {
	if rest, ok := strings.CutPrefix(fieldName, secConsoleVars+"."); ok {
		return secConsoleVars, rest
	}
	if i := strings.LastIndex(fieldName, "."); i >= 0 {
		return fieldName[:i], fieldName[i+1:]
	}
	return fieldName, ""
}

// gameSetting builds a curated settingDef from its AMP FieldName plus metadata,
// deriving Section and Key so they stay consistent with FieldName by
// construction.
func gameSetting(fieldName string, typ settingType, def, label, category, desc string) settingDef {
	section, key := fieldNameToSectionKey(fieldName)
	return settingDef{
		FieldName:   fieldName,
		Section:     section,
		Key:         key,
		Type:        typ,
		Default:     def,
		Label:       label,
		Description: desc,
		Category:    category,
	}
}

// serverSettingsSchema is the curated, evidence-validated set of gameplay
// settings (#122/#124). Every entry maps to a real engine CVar or a real
// /Script UPROPERTY confirmed against the shipping binary and AMP's own
// configmanifest — the earlier fictional m_Global*Multiplier keys (proven
// no-ops, absent from the real DefaultGame.ini) have been removed. FieldNames
// mirror AMP's nodes so the AMP control plane can write them via the AMP API,
// while non-AMP planes write the derived INI section/key directly.
//
// AMP-orchestration settings (WorldTitle, Region, PublicIPMode, FlsServiceAuthToken,
// instance pool, …) are intentionally out of scope here: they have no
// cross-plane INI representation and are managed in AMP's own panel. See the
// settings-rework notes for the follow-up to surface those under AMP only.
var serverSettingsSchema = []settingDef{
	// Multipliers (engine CVars).
	gameSetting("ConsoleVariables.Dune.GlobalMiningOutputMultiplier", settingFloat, "1.0",
		"Mining Output", catMultipliers, "Hand-mined resource yield scalar (1.0 = normal)."),
	gameSetting("ConsoleVariables.Dune.GlobalVehicleMiningOutputMultiplier", settingFloat, "1.0",
		"Vehicle Mining Output", catMultipliers, "Vehicle-mining resource yield scalar."),
	gameSetting("ConsoleVariables.SecurityZones.PvpResourceMultiplier", settingFloat, "2.5",
		"PvP Zone Resource", catMultipliers, "Bonus resource scalar applied inside PvP-enabled zones."),
	gameSetting("ConsoleVariables.dw.VehicleDurabilityDamageMultiplier", settingFloat, "1.0",
		"Vehicle Durability Damage", catMultipliers, "Damage scalar to vehicle durability. 0 disables damage; 10 = max."),
	gameSetting("ConsoleVariables.Vehicle.SandwormInvulnerabilitySecondsOnExit", settingFloat, "900.0",
		"Sandworm Invuln (Vehicle Exit) seconds", catMultipliers, "Vehicle invulnerability window after exiting a vehicle."),
	gameSetting("ConsoleVariables.Vehicle.SandwormInvulnerabilitySecondsOnServerRestart", settingFloat, "7200.0",
		"Sandworm Invuln (Server Restart) seconds", catMultipliers, "Vehicle invulnerability window after a server restart."),

	// World & Combat.
	gameSetting("/Script/DuneSandbox.PvpPveSettings.m_bShouldForceEnablePvpOnAllPartitions", settingBool, "False",
		"Force PvP On All Partitions", catWorldCombat, "If enabled, every map allows PvP regardless of partition setting."),
	gameSetting("/Script/DuneSandbox.SecurityZonesSubsystem.m_bAreSecurityZonesEnabled", settingBool, "True",
		"Security Zones Enabled", catWorldCombat, "Disable to allow PvP and abilities everywhere on the map (no safe zones)."),
	gameSetting("ConsoleVariables.Vehicle.SandwormCollisionInteraction", settingBool, "False",
		"Sandworm Vehicle Collision", catWorldCombat, "Sandworms can push/damage vehicles."),
	gameSetting("ConsoleVariables.Sandstorm.Enabled", settingBool, "True",
		"Sandstorms Enabled", catWorldCombat, "Enable sandstorm weather events."),
	gameSetting("ConsoleVariables.Sandstorm.Treasure.Enabled", settingBool, "True",
		"Sandstorm Treasure Drops", catWorldCombat, "Loot drops during sandstorms."),
	gameSetting("ConsoleVariables.sandworm.dune.Enabled", settingBool, "True",
		"Sandworms Enabled", catWorldCombat, "Enable sandworm spawns."),
	gameSetting("ConsoleVariables.Sandworm.SandwormDangerZonesEnabled", settingBool, "True",
		"Sandworm Danger Zones", catWorldCombat, "Visible danger zones where sandworms can attack."),
	gameSetting("/Script/DuneSandbox.SandStormConfig.m_bCoriolisAutoSpawnEnabled", settingBool, "True",
		"Coriolis Storm Auto-Spawn", catWorldCombat, "Coriolis storms spawn automatically."),

	// Persistence & Building.
	gameSetting("/DeteriorationSystem.ItemDeteriorationConstants.UpdateRateInSeconds", settingFloat, "1.0",
		"Item Deterioration Rate", catPersistence, "Item deterioration tick interval in seconds. 0 disables decay; 10 = fastest."),
	gameSetting("/Script/DuneSandbox.BuildingSettings.m_MaxNumLandclaimSegments", settingInt, "6",
		"Max Landclaim Segments", catPersistence, "Maximum number of land-claim flags a player may own."),
	gameSetting("/Script/DuneSandbox.BuildingSettings.m_BuildingBlueprintMaxExtensions", settingInt, "4",
		"Building Blueprint Max Extensions", catPersistence, "How many times a blueprinted building can be extended."),
	gameSetting("/Script/DuneSandbox.BuildingSettings.m_BaseBackupMaxExtensions", settingInt, "8",
		"Base Backup Max Extensions", catPersistence, "How many times a base backup can be extended."),
	gameSetting("/Script/DuneSandbox.BuildingSettings.m_bBuildingRestrictionLimitsEnabled", settingBool, "True",
		"Building Restriction Limits", catPersistence, "Enforce limits on what may be built (e.g. inside dungeons)."),

	// Server Identity (engine CVars).
	gameSetting("ConsoleVariables.Bgd.ServerDisplayName", settingString, "Sietch Cubern",
		"In-game Server Name", catServerIdentity, "Name shown to players in the in-game server browser and UI."),
	gameSetting("ConsoleVariables.Bgd.ServerLoginPassword", settingString, "",
		"Server Login Password", catServerIdentity, "Optional. Players must enter this password to join. Leave blank to disable."),
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
		if shouldSkipINILine(line) {
			continue
		}
		if section, ok := parseINISectionHeader(line); ok {
			curSec = section
			ensureRawSectionIndex(curSec, source, secMap, &result)
			continue
		}
		if curSec == "" {
			continue
		}
		rawLine, ok := parseRawINILine(line)
		if !ok {
			continue
		}
		if shouldIncludeRawLine(curSec, rawLine, schemaKeys) {
			idx := ensureRawSectionIndex(curSec, source, secMap, &result)
			result[idx].Lines = append(result[idx].Lines, rawLine)
		}
	}

	return compactRawSections(result)
}

func shouldSkipINILine(line string) bool {
	return line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#")
}

func parseINISectionHeader(line string) (string, bool) {
	if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
		return "", false
	}
	return line[1 : len(line)-1], true
}

func parseRawINILine(line string) (RawLine, bool) {
	prefix, rest := "", line
	if line[0] == '+' || line[0] == '-' {
		prefix = string(line[0])
		rest = line[1:]
	}
	eq := strings.Index(rest, "=")
	if eq <= 0 {
		return RawLine{}, false
	}
	return RawLine{
		Prefix: prefix,
		Key:    strings.TrimSpace(rest[:eq]),
		Value:  strings.TrimSpace(rest[eq+1:]),
	}, true
}

func shouldIncludeRawLine(section string, line RawLine, schemaKeys map[string]bool) bool {
	if line.Prefix != "" {
		return true
	}
	return !schemaKeys[section+"|"+line.Key]
}

func ensureRawSectionIndex(section, source string, secMap map[string]int, result *[]RawSection) int {
	if idx, ok := secMap[section]; ok {
		return idx
	}
	idx := len(*result)
	secMap[section] = idx
	*result = append(*result, RawSection{Section: section, Source: source})
	return idx
}

func compactRawSections(result []RawSection) []RawSection {
	out := result[:0]
	for _, section := range result {
		if len(section.Lines) == 0 {
			continue
		}
		out = append(out, section)
	}
	return out
}

func storeINIEntry(line, cur string, sections map[string]map[string]string, counts map[string]map[string]int) {
	rest := line
	prefix := ""
	if len(line) > 0 && (line[0] == '+' || line[0] == '-') {
		prefix = string(line[0])
		rest = line[1:]
	}
	eq := strings.Index(rest, "=")
	if eq <= 0 {
		return
	}
	baseKey := prefix + strings.TrimSpace(rest[:eq])
	val := strings.TrimSpace(rest[eq+1:])
	n := counts[cur][baseKey]
	counts[cur][baseKey] = n + 1
	storeKey := baseKey
	if n > 0 {
		storeKey = fmt.Sprintf("%s\x00%d", baseKey, n)
	}
	sections[cur][storeKey] = val
}

func applyINILine(line, cur string, sections map[string]map[string]string, counts map[string]map[string]int) string {
	if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
		return cur
	}
	if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
		cur = line[1 : len(line)-1]
		if sections[cur] == nil {
			sections[cur] = map[string]string{}
			counts[cur] = map[string]int{}
		}
		return cur
	}
	if cur == "" {
		return cur
	}
	storeINIEntry(line, cur, sections, counts)
	return cur
}

// parseINIRaw parses raw INI lines preserving +/- prefixes as part of the key
// (e.g. "+ActiveMod=SomeMod" is stored as key "+ActiveMod"). Duplicate keys
// (common for UE array entries like multiple "+ActiveMod=" lines) are stored
// with a unique null-byte suffix ("\x00N") so they survive the map round-trip.
// renderDuneAdminBlock strips the suffix when writing the file.
func parseINIRaw(content string) map[string]map[string]string {
	sections := map[string]map[string]string{}
	counts := map[string]map[string]int{}
	var cur string
	for _, raw := range strings.Split(content, "\n") {
		cur = applyINILine(strings.TrimSpace(raw), cur, sections, counts)
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

// gameOverrideProvider is implemented by control planes that manage UserGame.ini
// themselves and expect dune-admin's game-scoped settings in a separate
// overrides file. AMP appends UserOverrides.ini to UserGame.ini at boot, so
// writing there leaves AMP's dashboard-managed UserGame.ini untouched.
type gameOverrideProvider interface {
	gameOverridePath(dir string) string
}

// serverSettingsWriter is implemented by control planes that own the game's
// config files themselves and must be written through their own API rather than
// by editing the INI files directly. AMP regenerates UserEngine.ini /
// UserGame.ini from its config on every start, so a direct INI edit is
// clobbered; writing through AMP's API persists and survives restarts. Control
// planes that don't implement this fall through to the INI write path.
type serverSettingsWriter interface {
	// writeServerSettings applies fieldName→value updates through the control
	// plane's own configuration API.
	writeServerSettings(ctx context.Context, exec Executor, updates map[string]string) error
}

// serverSettingsReader is implemented by control planes that own the curated
// settings in their own config (AMP) rather than the INI files. It lets the GET
// path read current values back from the control plane so the UI reflects values
// saved through the control plane's API immediately, without waiting for a game
// restart to regenerate the INIs (#173). Control planes that don't implement it
// fall back to the INI-derived values.
type serverSettingsReader interface {
	// readServerSettings returns current fieldName→value for the given curated
	// FieldNames, read from the control plane's own configuration API.
	readServerSettings(ctx context.Context, exec Executor, fields []string) (map[string]string, error)
}

// ampSettingsSource marks a curated setting whose current value was read back
// from the AMP control plane's live config (rather than an INI file).
const ampSettingsSource = "amp"

// curatedFieldNamesFrom collects the AMP FieldNames present in the built
// settings (curated settings carry a FieldName; discovered ones don't).
func curatedFieldNamesFrom(settings []ServerSetting) []string {
	out := make([]string, 0, len(settings))
	for _, s := range settings {
		if s.FieldName != "" {
			out = append(out, s.FieldName)
		}
	}
	return out
}

// overlayAMPSettings makes the control plane's live config authoritative for
// curated settings: each setting whose FieldName is in ampValues takes that
// value as its Current. When the value differs from the schema default it is
// also marked overridden and recorded as the top-priority "amp" layer, so the
// detail view and the "modified" filter stay consistent. A value equal to the
// default leaves the source/layers untouched (the setting is unconfigured).
// A nil/empty map is a no-op, so non-AMP planes are unaffected.
func overlayAMPSettings(settings []ServerSetting, ampValues map[string]string) []ServerSetting {
	if len(ampValues) == 0 {
		return settings
	}
	for i := range settings {
		fn := settings[i].FieldName
		if fn == "" {
			continue
		}
		v, ok := ampValues[fn]
		if !ok {
			continue
		}
		settings[i].Current = v
		settings[i].IsOverride = v != settings[i].Default
		if v != settings[i].Default {
			settings[i].Source = ampSettingsSource
			settings[i].Layers = append(settings[i].Layers, SettingLayer{Source: ampSettingsSource, Value: v})
		}
	}
	return settings
}

// splitCuratedFromINI partitions normalized (section→key→value) updates into
// curated settings and everything else. Curated settings (those in the schema,
// which carry an AMP FieldName) are returned as FieldName→value for the AMP API;
// because AMP has no "unset" — every node holds a value — a clear (empty value)
// resolves to the curated schema default. All other (section,key) pairs are
// returned as section→key→value for the INI write path.
//
// Under AMP the caller writes the curated settings through the API and the rest
// to the INI files, so operators can still set custom settings AMP doesn't
// manage rather than being rejected.
func splitCuratedFromINI(updates map[string]map[string]string) (fieldUpdates map[string]string, iniUpdates map[string]map[string]string) {
	schemaMap := buildServerSettingsSchemaMap()
	fieldUpdates = map[string]string{}
	iniUpdates = map[string]map[string]string{}
	for sec, kvs := range updates {
		for key, val := range kvs {
			if def, ok := schemaMap[sec+"|"+key]; ok {
				v := val
				if v == "" {
					v = def.Default
				}
				fieldUpdates[def.FieldName] = v
				continue
			}
			if iniUpdates[sec] == nil {
				iniUpdates[sec] = map[string]string{}
			}
			iniUpdates[sec][key] = val
		}
	}
	return fieldUpdates, iniUpdates
}

// gameWritePath returns the file dune-admin writes game-scoped settings to.
// When the active control plane provides a game-overrides path (AMP), writes go
// there (UserOverrides.ini); otherwise game settings are written directly to
// UserGame.ini in dir, matching the historical behaviour for kubectl/docker/local.
func gameWritePath(dir string) string {
	if provider, ok := globalControl.(gameOverrideProvider); ok {
		if path := provider.gameOverridePath(dir); path != "" {
			return path
		}
	}
	return dir + "/UserGame.ini"
}

func iniDir() (string, error) {
	// Always try the control plane first so amp can probe for ue5-saved/UserSettings
	// even when server_ini_dir is explicitly configured. Control planes that don't
	// support auto-discovery (docker, local without kubectl) return an error and we
	// fall through to the configured value.
	if globalControl != nil && globalExecutor != nil {
		if dir, err := globalControl.DiscoverIniDir(context.Background(), globalExecutor); err == nil {
			return dir, nil
		}
	}
	if serverIniDir != "" {
		return serverIniDir, nil
	}
	if loadedConfig.ServerIniDir != "" {
		return loadedConfig.ServerIniDir, nil
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
func configuredDefaultINIPath(filename string) string {
	if loadedConfig.DefaultIniDir == "" {
		return ""
	}
	return filepath.Join(loadedConfig.DefaultIniDir, filepath.Base(filename))
}

func k8sDerivedDefaultINICandidates(inPodDir, filename string) []string {
	return []string{
		pathpkg.Clean(pathpkg.Join(inPodDir, "..", "..", "..", "Config", filename)),
		pathpkg.Clean(pathpkg.Join(inPodDir, "..", "..", "Config", filename)),
		pathpkg.Clean(pathpkg.Join(inPodDir, "..", "..", "..", "..", "Config", filename)),
		"/DuneSandbox/Config/" + filename,
		"/home/dune/server/DuneSandbox/Config/" + filename,
		"/home/dune/DuneSandbox/Config/" + filename,
		"/game/DuneSandbox/Config/" + filename,
	}
}

func hostDefaultINICandidates(filename string) []string {
	return []string{
		"/home/dune/" + filename,
		"/home/" + filename,
		"/root/" + filename,
		"/dune/" + filename,
		"/home/dune/server/DuneSandbox/Config/" + filename,
	}
}

func relativeDefaultINICandidates(iniDir, filename string) []string {
	return []string{
		filepath.Join(iniDir, "..", "..", "..", "Config", filename),
		filepath.Join(iniDir, "..", "..", "Config", filename),
		filepath.Join(iniDir, "..", "..", "..", "..", "Config", filename),
	}
}

// defaultINIDirProvider is implemented by control planes that can locate the
// game's stock Default*.ini directory on the host without configuration. AMP
// derives it from the instance layout.
type defaultINIDirProvider interface {
	defaultINIDir(iniDir string) string
}

// discoverViaControlDefaultDir reads a Default INI from the directory the active
// control plane derives for the host (AMP's extracted game-server tree).
// Returns "" when the control plane provides no such directory.
func discoverViaControlDefaultDir(iniDir, filename string) string {
	provider, ok := globalControl.(defaultINIDirProvider)
	if !ok {
		return ""
	}
	dir := provider.defaultINIDir(iniDir)
	if dir == "" {
		return ""
	}
	return readINIContent(dir + "/" + filename)
}

func discoverViaConfiguredPath(filename string) string {
	path := configuredDefaultINIPath(filename)
	if path == "" {
		return ""
	}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		return string(data)
	}
	return readINIContent(path)
}

func discoverViaK8sDerivedPath(iniDir, filename string) string {
	ns, pod, inPodDir, ok := parseK8sINIPath(iniDir)
	if !ok {
		return ""
	}
	for _, inPodPath := range k8sDerivedDefaultINICandidates(inPodDir, filename) {
		if content := readINIContent(fmt.Sprintf("k8s://%s/%s%s", ns, pod, inPodPath)); content != "" {
			return content
		}
	}
	return ""
}

func discoverViaHostPaths(filename string) string {
	for _, path := range hostDefaultINICandidates(filename) {
		if content := readINIContent(path); content != "" {
			return content
		}
	}
	return ""
}

func discoverViaHostFind(filename string) string {
	out, _ := globalExecutor.Exec(fmt.Sprintf(
		"sudo find /home /root /dune /run/k3s/containerd /var/lib/rancher/k3s/agent/containerd"+
			" -maxdepth 10 -name %s -not -path '*/Saved/*' -not -path '*/saved/*' 2>/dev/null | head -1",
		shellQuote(filename)))
	if path := strings.TrimSpace(out); path != "" {
		return readINIContent(path)
	}
	return ""
}

func discoverViaRelativePath(iniDir, filename string) string {
	for _, path := range relativeDefaultINICandidates(iniDir, filename) {
		if content := readINIContent(path); content != "" {
			return content
		}
	}
	return ""
}

func discoverDefaultINI(iniDir, filename string) string {
	if content := discoverViaConfiguredPath(filename); content != "" {
		return content
	}

	// 1a. Control-plane-derived host directory (AMP's extracted game-server tree).
	// Deterministic and configuration-free; the stock defaults live deeper than
	// the host-find maxdepth, so this must run before the find fallback.
	if content := discoverViaControlDefaultDir(iniDir, filename); content != "" {
		return content
	}

	// 1b. When INI dir points to a k8s pod path, derive nearby Config paths in
	// the same pod first. This is the most reliable source in deployed mode.
	if content := discoverViaK8sDerivedPath(iniDir, filename); content != "" {
		return content
	}

	if globalExecutor != nil {
		// 2a. Well-known host directories — tried in order before any find.
		if content := discoverViaHostPaths(filename); content != "" {
			return content
		}

		// 2b. Host filesystem scan: /home /root /dune first, then k3s containerd
		// paths. These require sudo but the executor already runs with sudo access.
		if content := discoverViaHostFind(filename); content != "" {
			return content
		}

		// 3. Relative candidates from iniDir (non-k8s layouts).
		if content := discoverViaRelativePath(iniDir, filename); content != "" {
			return content
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
	// Try plain cat first: when the service runs as the file owner (e.g. AMP
	// where the service user is amp and owns UserGame.ini), no sudo is needed
	// and sudo cat may not be in the sudoers at all. Fall back to sudo cat for
	// setups where the service user is not the file owner (docker, SSH, etc.).
	out, err := globalExecutor.Exec(fmt.Sprintf("cat %s 2>/dev/null", shellQuote(path)))
	if err == nil && strings.TrimSpace(out) != "" {
		return out
	}
	out, _ = globalExecutor.Exec(fmt.Sprintf("sudo cat %s 2>/dev/null", shellQuote(path)))
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
	gameIni,
	gameOverridesIni map[string]map[string]string,
) []layerSource {
	// Ordered low → high priority. userGameOverrides is highest: AMP appends
	// UserOverrides.ini after UserGame.ini at boot, so its keys win at runtime.
	return []layerSource{
		{name: "defaultEngine", ini: defaultEngineIni},
		{name: "defaultGame", ini: defaultGameIni},
		{name: "userEngine", ini: engineIni},
		{name: "userGame", ini: gameIni},
		{name: "userGameOverrides", ini: gameOverridesIni},
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
			FieldName:   def.FieldName,
		}
		applySettingLayers(&s, layerSources)
		settings = append(settings, s)
	}
	return settings
}

func discoverSectionKeys(section string, keys map[string]string, seenKeys map[string]bool, out *[]discoveredKey) {
	for key := range keys {
		if strings.HasPrefix(key, "+") || strings.HasPrefix(key, "-") {
			continue
		}
		composite := section + "|" + key
		if seenKeys[composite] {
			continue
		}
		seenKeys[composite] = true
		*out = append(*out, discoveredKey{section: section, key: key})
	}
}

func discoverUnknownSettings(layerSources []layerSource, schemaKeys map[string]bool) []discoveredKey {
	seenKeys := make(map[string]bool, len(schemaKeys))
	for k := range schemaKeys {
		seenKeys[k] = true
	}
	discovered := make([]discoveredKey, 0, 32)
	for _, src := range layerSources {
		for section, keys := range src.ini {
			discoverSectionKeys(section, keys, seenKeys, &discovered)
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
	engineContent,
	gameOverridesContent string,
	schemaKeys map[string]bool,
) []RawSection {
	raw := make([]RawSection, 0, 16)
	raw = append(raw, parseINILines(defaultGameContent, "defaultGame", schemaKeys)...)
	raw = append(raw, parseINILines(defaultEngineContent, "defaultEngine", schemaKeys)...)
	raw = append(raw, parseINILines(gameContent, "userGame", schemaKeys)...)
	raw = append(raw, parseINILines(engineContent, "userEngine", schemaKeys)...)
	raw = append(raw, parseINILines(gameOverridesContent, "userGameOverrides", schemaKeys)...)
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

// @Summary Get server INI settings and raw sections
// @Tags server-settings
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} map[string]string
// @Router /api/v1/server-settings [get]
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

	// When the control plane manages UserGame.ini itself (AMP), dune-admin's
	// game settings live in a separate overrides file that wins at runtime.
	// Surface it as the highest-priority layer. Skip the read when the override
	// path is just UserGame.ini (non-AMP) to avoid duplicating that layer.
	var overridesContent string
	if overridePath := gameWritePath(dir); overridePath != dir+"/UserGame.ini" {
		overridesContent = readINIContent(overridePath)
	}

	gameIni := parseINI(gameContent)
	engineIni := parseINI(engineContent)
	defaultGameIni := parseINI(defaultGameContent)
	defaultEngineIni := parseINI(defaultEngineContent)
	gameOverridesIni := parseINI(overridesContent)

	layerSources := buildLayerSources(defaultEngineIni, defaultGameIni, engineIni, gameIni, gameOverridesIni)
	schemaKeys := serverSettingsSchemaKeys()
	settings := buildSchemaSettings(layerSources)
	discovered := discoverUnknownSettings(layerSources, schemaKeys)
	settings = append(settings, buildDiscoveredSettings(discovered, layerSources, schemaKeys)...)
	raw := buildServerSettingsRawSections(defaultGameContent, defaultEngineContent, gameContent, engineContent, overridesContent, schemaKeys)

	// AMP-style planes own curated settings in their own config and regenerate
	// the INIs on restart, so read current values back from the control plane to
	// reflect saved values immediately rather than showing stale/default INI
	// values until the next restart (#173). Degrade gracefully: on error, keep
	// the INI-derived values.
	if reader, ok := globalControl.(serverSettingsReader); ok {
		if amp, err := reader.readServerSettings(r.Context(), globalExecutor, curatedFieldNamesFrom(settings)); err != nil {
			log.Printf("handleGetServerSettings: amp settings read-back: %v", err)
		} else {
			settings = overlayAMPSettings(settings, amp)
		}
	}

	controlName := ""
	if globalControl != nil {
		controlName = globalControl.Name()
	}
	jsonOK(w, map[string]any{
		"settings": settings,
		"raw":      raw,
		"control":  controlName,
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
		renderManagedSection(&b, sec, managed[sec])
	}
	b.WriteString("\n" + duneAdminEndMarker + "\n")
	return b.String()
}

func sortedManagedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		baseI, idxI, dupI := managedKeyParts(keys[i])
		baseJ, idxJ, dupJ := managedKeyParts(keys[j])
		if baseI != baseJ {
			return baseI < baseJ
		}
		if dupI != dupJ {
			return !dupI
		}
		if dupI && idxI != idxJ {
			return idxI < idxJ
		}
		return keys[i] < keys[j]
	})
	return keys
}

func renderManagedSection(b *strings.Builder, sec string, m map[string]string) {
	b.WriteString("\n[" + sec + "]\n")
	for _, k := range sortedManagedKeys(m) {
		displayKey := k
		if idx := strings.IndexByte(k, '\x00'); idx >= 0 {
			displayKey = k[:idx]
		}
		b.WriteString(displayKey + "=" + m[k] + "\n")
	}
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
	skip := map[int]bool{}
	headerIdxs := findINISectionHeaders(lines)
	for i, headerIdx := range headerIdxs {
		sectionEnd := nextINISectionStart(i, headerIdxs, len(lines))
		if isINISectionBodyEmpty(lines, headerIdx+1, sectionEnd) {
			markINISectionRange(skip, headerIdx, sectionEnd)
		}
	}
	if len(skip) == 0 {
		return content
	}
	return joinINILinesWithoutSkipped(lines, skip)
}

func findINISectionHeaders(lines []string) []int {
	headerIdxs := make([]int, 0, len(lines))
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			headerIdxs = append(headerIdxs, i)
		}
	}
	return headerIdxs
}

func nextINISectionStart(current int, headerIdxs []int, lineCount int) int {
	if current+1 < len(headerIdxs) {
		return headerIdxs[current+1]
	}
	return lineCount
}

func isINISectionBodyEmpty(lines []string, start, end int) bool {
	for i := start; i < end; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			return false
		}
	}
	return true
}

func markINISectionRange(skip map[int]bool, start, end int) {
	for i := start; i < end; i++ {
		skip[i] = true
	}
}

func joinINILinesWithoutSkipped(lines []string, skip map[int]bool) string {
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if skip[i] {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// stripKeysFromContent removes the listed keys from their sections in content.
// Both plain k=v and prefixed array lines (+k=v, -k=v) are matched: if "+Foo"
// or "-Foo" appears in owned, any line with that exact prefixed key is removed;
// if plain "Foo" is owned, all variants (plain, +Foo, -Foo) are removed.
// Comments and unrelated lines are left alone.
func parseOwnedINIKey(trim string) (lineKey, baseKey string, ok bool) {
	if len(trim) == 0 || trim[0] == ';' || trim[0] == '#' {
		return "", "", false
	}
	rest := trim
	prefix := ""
	if trim[0] == '+' || trim[0] == '-' {
		prefix = string(trim[0])
		rest = trim[1:]
	}
	eq := strings.Index(rest, "=")
	if eq <= 0 {
		return "", "", false
	}
	baseKey = strings.TrimSpace(rest[:eq])
	return prefix + baseKey, baseKey, true
}

func shouldStripOwnedLine(section, trim string, owned map[string]map[string]bool) bool {
	if section == "" {
		return false
	}
	lineKey, baseKey, ok := parseOwnedINIKey(trim)
	if !ok {
		return false
	}
	secOwned := owned[section]
	if secOwned == nil {
		return false
	}
	return secOwned[lineKey] || secOwned[baseKey]
}

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
		if shouldStripOwnedLine(curSec, trim, owned) {
			continue
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

type normalizedServerSettingUpdates struct {
	updates map[string]map[string]string
	applied int
	cleared int
}

func buildServerSettingsSchemaMap() map[string]settingDef {
	schemaMap := make(map[string]settingDef, len(serverSettingsSchema))
	for _, d := range serverSettingsSchema {
		schemaMap[d.Section+"|"+d.Key] = d
	}
	return schemaMap
}

func normalizeServerSettingsUpdates(
	requested []serverSettingUpdate,
	schemaMap map[string]settingDef,
) (normalizedServerSettingUpdates, error) {
	normalized := normalizedServerSettingUpdates{
		updates: make(map[string]map[string]string, len(requested)),
	}
	for _, update := range requested {
		if normalized.updates[update.Section] == nil {
			normalized.updates[update.Section] = map[string]string{}
		}
		if update.Value == "" {
			normalized.updates[update.Section][update.Key] = ""
			normalized.cleared++
			continue
		}

		def, known := schemaMap[update.Section+"|"+update.Key]
		if known {
			norm, err := normalizeValue(def.Type, update.Value)
			if err != nil {
				return normalizedServerSettingUpdates{}, fmt.Errorf("invalid value for %s: %w", update.Key, err)
			}
			normalized.updates[update.Section][update.Key] = norm
		} else {
			normalized.updates[update.Section][update.Key] = update.Value
		}
		normalized.applied++
	}
	return normalized, nil
}

// isEngineSection reports whether an INI section belongs in UserEngine.ini
// rather than UserGame.ini. [ConsoleVariables] is always engine-scoped — CVars
// are applied from an Engine ini, and this must hold even when DefaultEngine.ini
// is unreadable on a given deployment. Any section declared in DefaultEngine.ini
// is likewise engine-scoped.
func isEngineSection(section string, defaultEngineIni map[string]map[string]string) bool {
	if section == secConsoleVars {
		return true
	}
	_, ok := defaultEngineIni[section]
	return ok
}

func splitServerSettingsUpdatesByFile(
	defaultEngineIni map[string]map[string]string,
	updates map[string]map[string]string,
) (gameUpdates, engineUpdates map[string]map[string]string) {
	gameUpdates = map[string]map[string]string{}
	engineUpdates = map[string]map[string]string{}
	for sec, kvs := range updates {
		if isEngineSection(sec, defaultEngineIni) {
			engineUpdates[sec] = kvs
		} else {
			gameUpdates[sec] = kvs
		}
	}
	return gameUpdates, engineUpdates
}

func buildUpdatedINIContent(path string, updates map[string]map[string]string) (string, error) {
	if len(updates) == 0 {
		return "", nil
	}
	return applyDuneAdminUpdates(readINIContent(path), updates)
}

// applyServerSettingsToINI writes updates to the user INI files
// (UserGame/UserOverrides + UserEngine), routing each section to the right file.
// It returns an HTTP status code alongside any error: 409 when a managed-marker
// conflict would risk data loss, 500 on write failure, 0 on success.
func applyServerSettingsToINI(dir string, updates map[string]map[string]string) (int, error) {
	// Route each section to UserGame.ini or UserEngine.ini based on which default
	// file declares it (ConsoleVariables is always engine-scoped).
	defaultEngineIni := parseINI(readDefaultINIContent(dir, "DefaultEngine.ini"))
	gameUpdates, engineUpdates := splitServerSettingsUpdatesByFile(defaultEngineIni, updates)

	// Game settings route to UserOverrides.ini under AMP (leaving AMP's
	// dashboard-managed UserGame.ini untouched) and to UserGame.ini otherwise.
	gamePath := gameWritePath(dir)
	gameName := pathpkg.Base(gamePath)
	gameBody, err := buildUpdatedINIContent(gamePath, gameUpdates)
	if err != nil {
		return 409, fmt.Errorf("%s: %w", gameName, err)
	}
	if len(gameUpdates) > 0 {
		if err := writeINIContent(gamePath, gameBody); err != nil {
			return 500, fmt.Errorf("write %s: %w", gameName, err)
		}
	}

	enginePath := dir + "/UserEngine.ini"
	engineBody, err := buildUpdatedINIContent(enginePath, engineUpdates)
	if err != nil {
		return 409, fmt.Errorf("UserEngine.ini: %w", err)
	}
	if len(engineUpdates) > 0 {
		if err := writeINIContent(enginePath, engineBody); err != nil {
			return 500, fmt.Errorf("write UserEngine.ini: %w", err)
		}
	}
	return 0, nil
}

// @Summary Apply one or more server setting changes
// @Tags server-settings
// @Accept json
// @Produce json
// @Param body body object true "List of setting updates"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/server-settings [put]
func handleUpdateServerSettings(w http.ResponseWriter, r *http.Request) {
	if globalExecutor == nil {
		jsonErr(w, fmt.Errorf("not connected"), 503)
		return
	}

	var req struct {
		Updates []serverSettingUpdate `json:"updates"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}

	normalized, err := normalizeServerSettingsUpdates(req.Updates, buildServerSettingsSchemaMap())
	if err != nil {
		jsonErr(w, err, 400)
		return
	}

	// AMP-style control planes own the game INIs and regenerate them on start, so
	// AMP-managed (curated) settings must go through their API (Core/SetConfig) to
	// survive. Everything else — and all settings on non-AMP planes — takes the
	// direct INI path. Splitting (rather than rejecting non-curated keys) lets
	// operators still set custom settings AMP doesn't manage.
	iniUpdates := normalized.updates
	if writer, ok := globalControl.(serverSettingsWriter); ok {
		fieldUpdates, rest := splitCuratedFromINI(normalized.updates)
		if len(fieldUpdates) > 0 {
			if err := writer.writeServerSettings(r.Context(), globalExecutor, fieldUpdates); err != nil {
				jsonErr(w, err, 502)
				return
			}
		}
		iniUpdates = rest
	}

	if len(iniUpdates) > 0 {
		dir, err := iniDir()
		if err != nil {
			jsonErr(w, err, 503)
			return
		}
		if code, err := applyServerSettingsToINI(dir, iniUpdates); err != nil {
			jsonErr(w, err, code)
			return
		}
	}

	jsonOK(w, map[string]any{
		"ok":      fmt.Sprintf("Saved (%d set, %d cleared). Restart the game server to apply.", normalized.applied, normalized.cleared),
		"applied": normalized.applied,
		"cleared": normalized.cleared,
	})
}

// handleUpdateRawSection replaces a single INI section in the appropriate user
// INI file. Sections declared in DefaultEngine.ini are written to UserEngine.ini;
// all others are written to UserGame.ini.
//
// @Summary Replace a raw INI section (array lines included)
// @Tags server-settings
// @Accept json
// @Produce json
// @Param body body object true "Section name and raw INI lines"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/v1/server-settings/raw [put]
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
	if isEngineSection(req.Section, defaultEngineIni) {
		filePath = dir + "/UserEngine.ini"
	} else {
		filePath = gameWritePath(dir)
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
