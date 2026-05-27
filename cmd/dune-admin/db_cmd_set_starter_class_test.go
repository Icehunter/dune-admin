package main

import (
	"strings"
	"testing"
)

func TestResolveStarterClassAbility(t *testing.T) {
	originalTagsData := tagsData
	t.Cleanup(func() { tagsData = originalTagsData })

	tagsData = tagsDataFile{
		JobSkillBlocks: map[string][]string{
			"Trooper": {"Skills.Key.Trooper1"},
		},
	}

	ability, err := resolveStarterClassAbility("Trooper")
	if err != nil {
		t.Fatalf("unexpected error resolving known job: %v", err)
	}
	if ability != "Skills.Ability.SuspensorGrenade_Reduction" {
		t.Fatalf("unexpected starter ability: %q", ability)
	}

	if _, err := resolveStarterClassAbility("Unknown"); err == nil {
		t.Fatalf("expected error for unknown job")
	}
}

func TestStarterKeysToRemove(t *testing.T) {
	t.Parallel()

	if keys := starterKeysToRemove("", "Trooper"); len(keys) != 0 {
		t.Fatalf("expected no keys when old starter is empty, got %#v", keys)
	}
	if keys := starterKeysToRemove("Skills.Key.Trooper1", "Trooper"); len(keys) != 0 {
		t.Fatalf("expected no keys when switching to same job, got %#v", keys)
	}

	keys := starterKeysToRemove("Skills.Key.Mentat1", "Trooper")
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys to remove, got %#v", keys)
	}
	if keys[0] != `(TagName="Skills.Key.Mentat1")` {
		t.Fatalf("unexpected starter key removal: %q", keys[0])
	}
	if keys[1] != `(TagName="Skills.Ability.PoisonCapsuleLauncher")` {
		t.Fatalf("unexpected ability key removal: %q", keys[1])
	}
}

func TestStarterClassTagAndKeys(t *testing.T) {
	t.Parallel()

	tag, starterKey, abilityKey := starterClassTagAndKeys("Trooper", "Skills.Ability.SuspensorGrenade_Reduction")
	if tag != "Skills.Key.Trooper1" {
		t.Fatalf("unexpected starter tag: %q", tag)
	}
	if starterKey != `(TagName="Skills.Key.Trooper1")` {
		t.Fatalf("unexpected starter key: %q", starterKey)
	}
	if abilityKey != `(TagName="Skills.Ability.SuspensorGrenade_Reduction")` {
		t.Fatalf("unexpected ability key: %q", abilityKey)
	}
}

func TestFormatStarterClassMessage(t *testing.T) {
	t.Parallel()

	msg := formatStarterClassMessage("Trooper", "Skills.Key.Trooper1", "Skills.Ability.SuspensorGrenade_Reduction", 2)
	if !strings.Contains(msg, "Starter class set to Trooper") ||
		!strings.Contains(msg, "Skills.Key.Trooper1 + Skills.Ability.SuspensorGrenade_Reduction active") ||
		!strings.Contains(msg, "cleared previous starter (2 module(s))") {
		t.Fatalf("unexpected message: %q", msg)
	}
}
