package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestNormalizeGrantQuality covers the quality-clamping and schematic-blocking rules.
func TestNormalizeGrantQuality(t *testing.T) {
	t.Parallel()

	minQual := int64(3)
	oldItemData := itemData
	itemData = itemDataFile{
		Items: map[string]itemRule{
			// augment with min_quality_level=3
			"t6_augment_armor1": {
				IsSchematic:     false,
				IsGradeable:     true,
				MinQualityLevel: &minQual,
			},
			// gradeable item without min_quality_level
			"gradeable_no_min": {
				IsGradeable: true,
			},
			// schematic — must be blocked
			"t6_lasgun_schematic": {
				IsSchematic: true,
				IsGradeable: true,
			},
			// plain (non-gradeable, non-schematic) — quality passes through
			"dune.item.sword": {
				IsGradeable: false,
			},
		},
	}
	t.Cleanup(func() { itemData = oldItemData })

	tests := []struct {
		name        string
		template    string
		quality     int64
		wantQuality int64
		wantErr     bool
		errContains string
	}{
		// schematic → always blocked
		{
			name:        "schematic blocked",
			template:    "T6_Lasgun_Schematic",
			quality:     1,
			wantErr:     true,
			errContains: "schematic",
		},
		{
			name:        "schematic blocked at quality 0",
			template:    "T6_Lasgun_Schematic",
			quality:     0,
			wantErr:     true,
			errContains: "schematic",
		},
		// gradeable with min_quality_level=3
		{
			name:        "gradeable quality 0 clamped to min",
			template:    "T6_Augment_Armor1",
			quality:     0,
			wantQuality: 3,
		},
		{
			name:        "gradeable quality below min clamped",
			template:    "T6_Augment_Armor1",
			quality:     1,
			wantQuality: 3,
		},
		{
			name:        "gradeable quality at min unchanged",
			template:    "T6_Augment_Armor1",
			quality:     3,
			wantQuality: 3,
		},
		{
			name:        "gradeable quality above min unchanged",
			template:    "T6_Augment_Armor1",
			quality:     5,
			wantQuality: 5,
		},
		// gradeable without min_quality_level — floor to 1
		{
			name:        "gradeable no-min quality 0 floored to 1",
			template:    "Gradeable_No_Min",
			quality:     0,
			wantQuality: 1,
		},
		{
			name:        "gradeable no-min quality 1 unchanged",
			template:    "Gradeable_No_Min",
			quality:     1,
			wantQuality: 1,
		},
		{
			name:        "gradeable no-min quality 4 unchanged",
			template:    "Gradeable_No_Min",
			quality:     4,
			wantQuality: 4,
		},
		// non-gradeable — quality passes through as-is
		{
			name:        "non-gradeable quality 0 passes through",
			template:    "Dune.Item.Sword",
			quality:     0,
			wantQuality: 0,
		},
		{
			name:        "non-gradeable quality 5 passes through",
			template:    "Dune.Item.Sword",
			quality:     5,
			wantQuality: 5,
		},
		// unknown template — no rule → passes through unchanged
		{
			name:        "unknown template passes through",
			template:    "Unknown_Item",
			quality:     0,
			wantQuality: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeGrantQuality(tt.template, tt.quality)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (quality=%d)", got)
				}
				if tt.errContains != "" && !containsStr(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantQuality {
				t.Fatalf("normalizeGrantQuality(%q, %d) = %d, want %d",
					tt.template, tt.quality, got, tt.wantQuality)
			}
		})
	}
}

// TestItemRuleGradeableFields verifies that the new IsGradeable and
// MinQualityLevel fields are parsed correctly from JSON.
func TestItemRuleGradeableFields(t *testing.T) {
	t.Parallel()

	minQ := int64(2)
	tests := []struct {
		name string
		json string
		want itemRule
	}{
		{
			name: "gradeable with min_quality_level",
			json: `{"is_gradeable":true,"min_quality_level":2,"is_schematic":false}`,
			want: itemRule{IsGradeable: true, MinQualityLevel: &minQ},
		},
		{
			name: "gradeable without min_quality_level",
			json: `{"is_gradeable":true}`,
			want: itemRule{IsGradeable: true, MinQualityLevel: nil},
		},
		{
			name: "schematic",
			json: `{"is_schematic":true,"is_gradeable":true}`,
			want: itemRule{IsSchematic: true, IsGradeable: true},
		},
		{
			name: "plain non-gradeable",
			json: `{"is_gradeable":false}`,
			want: itemRule{IsGradeable: false},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var r itemRule
			if err := json.Unmarshal([]byte(tt.json), &r); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if r.IsGradeable != tt.want.IsGradeable {
				t.Errorf("IsGradeable: got %v, want %v", r.IsGradeable, tt.want.IsGradeable)
			}
			if r.IsSchematic != tt.want.IsSchematic {
				t.Errorf("IsSchematic: got %v, want %v", r.IsSchematic, tt.want.IsSchematic)
			}
			switch {
			case tt.want.MinQualityLevel == nil && r.MinQualityLevel != nil:
				t.Errorf("MinQualityLevel: got %v, want nil", *r.MinQualityLevel)
			case tt.want.MinQualityLevel != nil && r.MinQualityLevel == nil:
				t.Errorf("MinQualityLevel: got nil, want %v", *tt.want.MinQualityLevel)
			case tt.want.MinQualityLevel != nil && r.MinQualityLevel != nil && *r.MinQualityLevel != *tt.want.MinQualityLevel:
				t.Errorf("MinQualityLevel: got %v, want %v", *r.MinQualityLevel, *tt.want.MinQualityLevel)
			}
		})
	}
}

// TestProcessGiveItems_SchematicBlocked verifies schematics produce a skippedItem
// and are never sent to the DB or RMQ.
func TestProcessGiveItems_SchematicBlocked(t *testing.T) {
	t.Parallel()

	minQ := int64(3)
	oldItemData := itemData
	itemData = itemDataFile{
		Items: map[string]itemRule{
			"t6_lasgun_schematic": {IsSchematic: true, IsGradeable: true, MinQualityLevel: &minQ},
			"t6_augment_armor1":   {IsGradeable: true, MinQualityLevel: &minQ},
		},
	}
	t.Cleanup(func() { itemData = oldItemData })

	req := giveItemsRequest{
		PlayerID: 42,
		Items: []giveItemInput{
			{Template: "T6_Lasgun_Schematic", Qty: 1, Quality: 1},
			{Template: "T6_Augment_Armor1", Qty: 1, Quality: 0},
		},
	}

	var dbCalled, rmqCalled bool
	given, skipped := processGiveItems(context.Background(), req, false, "", giveItemsDeps{
		checkCapacity: func(context.Context, int64, string, int64) error { return nil },
		rmqAdd: func(string, string, int, float64) error {
			rmqCalled = true
			return nil
		},
		dbGive: func(_ int64, template string, _ int64, quality int64) (msgMutate, bool) {
			dbCalled = true
			if template == "T6_Augment_Armor1" && quality < 3 {
				t.Errorf("augment granted with quality %d, want ≥3", quality)
			}
			return msgMutate{ok: "done"}, true
		},
		needsDBPath:      func(string) bool { return false },
		normalizeQuality: normalizeGrantQuality,
	})

	if rmqCalled {
		t.Error("RMQ must not be called for any item in this test")
	}
	if !dbCalled {
		t.Error("DB should have been called for the augment")
	}
	if len(given) != 1 || given[0] != "T6_Augment_Armor1" {
		t.Errorf("unexpected given: %v", given)
	}
	if len(skipped) != 1 || skipped[0].Template != "T6_Lasgun_Schematic" {
		t.Errorf("unexpected skipped: %+v", skipped)
	}
	if !containsStr(skipped[0].Reason, "schematic") {
		t.Errorf("skip reason should mention schematic, got %q", skipped[0].Reason)
	}
}

// containsStr is a strings.Contains wrapper for test readability.
func containsStr(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
