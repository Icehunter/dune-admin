package main

import (
	"strings"
	"testing"
)

func TestProgressionFactionConfigFor(t *testing.T) {
	t.Parallel()

	atreides, err := progressionFactionConfigFor("atreides")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atreides.factionID != 1 || atreides.alignedFlag != "DialogueFlags.Factions.AlignedAtreides" {
		t.Fatalf("unexpected atreides config: %+v", atreides)
	}

	if _, err := progressionFactionConfigFor("unknown"); err == nil {
		t.Fatalf("expected error for unknown faction")
	}
}

func TestProgressionTargetTierForPreset(t *testing.T) {
	t.Parallel()

	if tier, err := progressionTargetTierForPreset("ch3_start"); err != nil || tier != 5 {
		t.Fatalf("expected ch3_start => 5, got tier=%d err=%v", tier, err)
	}
	if tier, err := progressionTargetTierForPreset("rank19_eligible"); err != nil || tier != 19 {
		t.Fatalf("expected rank19_eligible => 19, got tier=%d err=%v", tier, err)
	}
	if _, err := progressionTargetTierForPreset("bad"); err == nil {
		t.Fatalf("expected error for bad preset")
	}
}

func TestProgressionUnlockTags(t *testing.T) {
	t.Parallel()

	cfg, _ := progressionFactionConfigFor("atreides")
	tags := progressionUnlockTags(cfg, 19)

	required := []string{
		"DialogueFlags.Factions.SentToMeetHawat",
		"DialogueFlags.Factions.AlignedAtreides",
		"Journey.LandsraadContractsUnlocked",
		"Faction.Atreides.Tier0",
		"Faction.Atreides.Tier5",
	}
	for _, tag := range required {
		if !containsString(tags, tag) {
			t.Fatalf("expected tag %q in output: %#v", tag, tags)
		}
	}
	if containsString(tags, "Faction.Atreides.Tier6") {
		t.Fatalf("did not expect Tier6 tag in output")
	}
}

func TestFormatProgressionUnlockSuccess(t *testing.T) {
	t.Parallel()

	msg := formatProgressionUnlockSuccess("ch3_start", "atreides", 12, "Atreides", 5, 777)
	expectParts := []string{
		"Progression unlock (ch3_start/atreides)",
		"12 journey nodes completed",
		"Atreides tier tags 0–5",
		"rep tier 5 on controller 777",
	}
	for _, part := range expectParts {
		if !strings.Contains(msg, part) {
			t.Fatalf("expected message to contain %q, got %q", part, msg)
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
