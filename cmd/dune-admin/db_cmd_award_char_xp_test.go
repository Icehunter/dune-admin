package main

import (
	"strings"
	"testing"
)

func TestComputeAwardCharXPOutcome(t *testing.T) {
	t.Parallel()

	t.Run("caps xp and marks capped", func(t *testing.T) {
		t.Parallel()
		outcome := computeAwardCharXPOutcome(maxCharXP-10, 5, 3, 99)
		if outcome.newXP != maxCharXP {
			t.Fatalf("expected capped xp %d, got %d", maxCharXP, outcome.newXP)
		}
		if !outcome.capped {
			t.Fatalf("expected capped=true")
		}
		if outcome.newTotalSP != outcome.newLevel+3 {
			t.Fatalf("expected total SP to include keystone bonus, got level=%d total=%d", outcome.newLevel, outcome.newTotalSP)
		}
	})

	t.Run("clamps unspent to zero", func(t *testing.T) {
		t.Parallel()
		outcome := computeAwardCharXPOutcome(0, 999, 0, 0)
		if outcome.newUnspentSP != 0 {
			t.Fatalf("expected unspent SP clamp to 0, got %d", outcome.newUnspentSP)
		}
		if outcome.capped {
			t.Fatalf("expected capped=false")
		}
	})
}

func TestFormatAwardCharXPSuccess(t *testing.T) {
	t.Parallel()

	outcome := charXPOutcome{
		newXP:        1234,
		newLevel:     42,
		newUnspentSP: 7,
		newIntel:     99,
		capped:       true,
	}
	msg := formatAwardCharXPSuccess(777, outcome, 11)
	if !strings.Contains(msg, "Player 777") ||
		!strings.Contains(msg, "level 42 (capped at level 200)") ||
		!strings.Contains(msg, "XP 1234") ||
		!strings.Contains(msg, "SP 7 unspent (11 spent)") ||
		!strings.Contains(msg, "Intel 99") {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestLoadControllerKeystoneIDs_NoController(t *testing.T) {
	t.Parallel()

	ids, err := loadControllerKeystoneIDs(t.Context(), 0)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ids != nil {
		t.Fatalf("expected nil ids, got %#v", ids)
	}
}
