package main

import (
	"math"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func floatPtr(v float64) *float64 {
	return &v
}

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
	originalItemData := itemData
	itemData = itemDataFile{
		Items: map[string]itemRule{
			"dune.item.catalog": {MaxDurability: floatPtr(250)},
		},
	}
	t.Cleanup(func() { itemData = originalItemData })

	tests := []struct {
		name       string
		templateID string
		maxText    pgtype.Text
		want       float64
	}{
		{
			name:       "catalog value wins",
			templateID: "Dune.Item.Catalog",
			maxText:    pgtype.Text{String: "125", Valid: true},
			want:       250,
		},
		{
			name:       "falls back to max durability text",
			templateID: "Dune.Item.Unknown",
			maxText:    pgtype.Text{String: "125", Valid: true},
			want:       125,
		},
		{
			name:       "invalid max durability text",
			templateID: "Dune.Item.Unknown",
			maxText:    pgtype.Text{String: "oops", Valid: true},
			want:       100,
		},
		{
			name:       "missing max durability text",
			templateID: "Dune.Item.Unknown",
			maxText:    pgtype.Text{},
			want:       100,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := repairTargetForItem(tt.templateID, tt.maxText); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestBuildRepairCandidate(t *testing.T) {
	originalItemData := itemData
	itemData = itemDataFile{
		Items: map[string]itemRule{
			"dune.item.catalog": {MaxDurability: floatPtr(250)},
		},
	}
	t.Cleanup(func() { itemData = originalItemData })

	tests := []struct {
		name          string
		itemID        int64
		templateID    string
		maxDurability pgtype.Text
		current       pgtype.Text
		decayed       pgtype.Text
		wantNeedsFix  bool
		wantTarget    float64
	}{
		{
			name:          "already at target",
			itemID:        1,
			templateID:    "Dune.Item.Unknown",
			maxDurability: pgtype.Text{String: "100", Valid: true},
			current:       pgtype.Text{String: "100", Valid: true},
			decayed:       pgtype.Text{String: "100", Valid: true},
			wantNeedsFix:  false,
		},
		{
			name:          "needs repair by current durability",
			itemID:        2,
			templateID:    "Dune.Item.Unknown",
			maxDurability: pgtype.Text{String: "100", Valid: true},
			current:       pgtype.Text{String: "75", Valid: true},
			decayed:       pgtype.Text{String: "100", Valid: true},
			wantNeedsFix:  true,
			wantTarget:    100,
		},
		{
			name:          "catalog durability target",
			itemID:        3,
			templateID:    "Dune.Item.Catalog",
			maxDurability: pgtype.Text{String: "100", Valid: true},
			current:       pgtype.Text{String: "150", Valid: true},
			decayed:       pgtype.Text{String: "150", Valid: true},
			wantNeedsFix:  true,
			wantTarget:    250,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			candidate, needsFix := buildRepairCandidate(tt.itemID, tt.templateID, tt.maxDurability, tt.current, tt.decayed)
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
	if err := validateRepairPlayerGearInput(42); err == nil || err.Error() != "not connected" {
		t.Fatalf("expected not connected error, got %v", err)
	}

	globalDB = &pgxpool.Pool{}
	if err := validateRepairPlayerGearInput(0); err == nil || err.Error() != "player ID required" {
		t.Fatalf("expected player ID required error, got %v", err)
	}
	if err := validateRepairPlayerGearInput(42); err != nil {
		t.Fatalf("expected valid input, got %v", err)
	}
}
