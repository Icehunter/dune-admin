package main

import (
	"math"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestParseDurabilityText(t *testing.T) {
	t.Parallel()

	if got := parseDurabilityText(pgtype.Text{}); got != 0 {
		t.Fatalf("expected invalid text to parse as 0, got %v", got)
	}
	if got := parseDurabilityText(pgtype.Text{String: "12.5", Valid: true}); got != 12.5 {
		t.Fatalf("expected 12.5, got %v", got)
	}
	if got := parseDurabilityText(pgtype.Text{String: "not-a-number", Valid: true}); got != 0 {
		t.Fatalf("expected parse failure to fall back to 0, got %v", got)
	}
}

func TestRepairTargetForItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		maxText pgtype.Text
		want    float64
	}{
		{
			name:    "in-row MaxDurability is the source of truth (200-scale item)",
			maxText: pgtype.Text{String: "200", Valid: true},
			want:    200,
		},
		{
			name:    "plain gear without MaxDurability defaults to 100",
			maxText: pgtype.Text{},
			want:    100,
		},
		{
			name:    "unparseable MaxDurability defaults to 100",
			maxText: pgtype.Text{String: "oops", Valid: true},
			want:    100,
		},
		{
			name:    "zero MaxDurability defaults to 100",
			maxText: pgtype.Text{String: "0", Valid: true},
			want:    100,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := repairTargetForItem(tt.maxText); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestBuildRepairCandidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		itemID        int64
		maxDurability pgtype.Text
		current       pgtype.Text
		decayed       pgtype.Text
		wantNeedsFix  bool
		wantTarget    float64
	}{
		{
			name:          "already at target",
			itemID:        1,
			maxDurability: pgtype.Text{String: "100", Valid: true},
			current:       pgtype.Text{String: "100", Valid: true},
			decayed:       pgtype.Text{String: "100", Valid: true},
			wantNeedsFix:  false,
		},
		{
			name:          "plain 0-100 gear restores to 100",
			itemID:        2,
			maxDurability: pgtype.Text{},
			current:       pgtype.Text{String: "75", Valid: true},
			decayed:       pgtype.Text{String: "100", Valid: true},
			wantNeedsFix:  true,
			wantTarget:    100,
		},
		{
			name:          "200-scale item restores to 200, not capped at 100",
			itemID:        3,
			maxDurability: pgtype.Text{String: "200", Valid: true},
			current:       pgtype.Text{String: "50", Valid: true},
			decayed:       pgtype.Text{String: "200", Valid: true},
			wantNeedsFix:  true,
			wantTarget:    200,
		},
		{
			name:          "never lowers below an existing value when MaxDurability absent",
			itemID:        4,
			maxDurability: pgtype.Text{},
			current:       pgtype.Text{String: "150", Valid: true},
			decayed:       pgtype.Text{String: "120", Valid: true},
			wantNeedsFix:  true,
			wantTarget:    150,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			candidate, needsFix := buildRepairCandidate(tt.itemID, tt.maxDurability, tt.current, tt.decayed)
			if needsFix != tt.wantNeedsFix {
				t.Fatalf("expected needsFix=%v, got %v", tt.wantNeedsFix, needsFix)
			}
			if !needsFix {
				return
			}
			if candidate.id != tt.itemID {
				t.Fatalf("expected item ID %d, got %d", tt.itemID, candidate.id)
			}
			if math.Abs(candidate.target-tt.wantTarget) > 0.0001 {
				t.Fatalf("expected target %.4f, got %.4f", tt.wantTarget, candidate.target)
			}
		})
	}
}

func TestValidateRepairPlayerGearInput(t *testing.T) {
	originalDB := globalDB
	t.Cleanup(func() { globalDB = originalDB })

	globalDB = nil
	if err := validateRepairPlayerGearInput(globalDB, 42); err == nil || err.Error() != "not connected" {
		t.Fatalf("expected not connected error, got %v", err)
	}

	globalDB = &pgxpool.Pool{}
	if err := validateRepairPlayerGearInput(globalDB, 0); err == nil || err.Error() != "player ID required" {
		t.Fatalf("expected player ID required error, got %v", err)
	}
	if err := validateRepairPlayerGearInput(globalDB, 42); err != nil {
		t.Fatalf("expected valid input, got %v", err)
	}
}
