package main

import (
	"strings"
	"time"
)

// parseDurString parses a duration string (e.g. "5m0s") falling back to def.
func parseDurString(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

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
