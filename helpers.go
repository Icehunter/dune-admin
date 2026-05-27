package main

import (
	"regexp"
	"strings"
)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// shortClass strips the UE class path prefix and _C suffix from an actor class name.
func shortClass(s string) string {
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		s = s[idx+1:]
	}
	s = strings.TrimSuffix(s, "_C")
	replacer := strings.NewReplacer(
		"BP_DunePlayerCharacter", "PlayerCharacter",
		"BP_DunePlayerController", "PlayerController",
		"DunePlayerState", "PlayerState",
	)
	return replacer.Replace(s)
}

// Per-position locomotion modules (e.g. OrnithopterLightLocomotionFrontLeft_6) share
// a single generic catalog entry (ornithopterlightlocomotion_6).
var locomotionPositionRe = regexp.MustCompile(`locomotion(frontleft|frontright|backleft|backright|backcenter)`)

// itemMaxDurability returns the catalog-defined max_durability for a template_id,
// retrying with the position suffix stripped for per-position locomotion modules.
// Returns (0, false) when the catalog has no max_durability for the template.
func itemMaxDurability(templateID string) (float64, bool) {
	key := strings.ToLower(templateID)
	if rule, ok := itemData.Items[key]; ok && rule.MaxDurability != nil {
		return *rule.MaxDurability, true
	}
	if stripped := locomotionPositionRe.ReplaceAllString(key, "locomotion"); stripped != key {
		if rule, ok := itemData.Items[stripped]; ok && rule.MaxDurability != nil {
			return *rule.MaxDurability, true
		}
	}
	return 0, false
}
