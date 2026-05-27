package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestValidateGiveItemInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		playerID  int64
		template  string
		qty       int64
		wantTpl   string
		wantError string
	}{
		{name: "valid", playerID: 123, template: "  Dune.Item  ", qty: 2, wantTpl: "Dune.Item"},
		{name: "missing-player", playerID: 0, template: "x", qty: 1, wantError: "player ID required"},
		{name: "missing-template", playerID: 1, template: "   ", qty: 1, wantError: "item template required"},
		{name: "invalid-qty", playerID: 1, template: "x", qty: 0, wantError: "quantity must be > 0"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := validateGiveItemInput(tt.playerID, tt.template, tt.qty)
			if tt.wantError != "" {
				if err == nil || err.Error() != tt.wantError {
					t.Fatalf("expected error %q, got %v", tt.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantTpl {
				t.Fatalf("expected template %q, got %q", tt.wantTpl, got)
			}
		})
	}
}

func TestPlanGiveItemStacks(t *testing.T) {
	t.Parallel()

	stacks := []giveItemStackSlot{
		{id: 1, size: 8},
		{id: 2, size: 10},
		{id: 3, size: 2},
	}
	updates, created := planGiveItemStacks(17, 10, stacks)

	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}
	if updates[0].id != 1 || updates[0].add != 2 {
		t.Fatalf("unexpected first update: %+v", updates[0])
	}
	if updates[1].id != 3 || updates[1].add != 8 {
		t.Fatalf("unexpected second update: %+v", updates[1])
	}
	if len(created) != 1 || created[0] != 7 {
		t.Fatalf("unexpected created stacks: %#v", created)
	}
}

func TestPlanGiveItemStacks_NoStacking(t *testing.T) {
	t.Parallel()

	updates, created := planGiveItemStacks(3, 1, []giveItemStackSlot{{id: 1, size: 1}})
	if len(updates) != 0 {
		t.Fatalf("expected no updates, got %#v", updates)
	}
	if len(created) != 3 || created[0] != 1 || created[1] != 1 || created[2] != 1 {
		t.Fatalf("unexpected created stacks: %#v", created)
	}
}

func TestEnsureGiveItemSlotCapacity(t *testing.T) {
	t.Parallel()

	inv := giveItemInventory{maxSlots: 5, hasSlotCap: true}
	state := giveItemInventoryState{usedSlots: 3}
	if err := ensureGiveItemSlotCapacity(inv, state, 2); err != nil {
		t.Fatalf("expected capacity to fit, got %v", err)
	}
	if err := ensureGiveItemSlotCapacity(inv, state, 3); err == nil {
		t.Fatalf("expected slot-capacity error")
	}
}

func TestInventoryItemVolume(t *testing.T) {
	t.Parallel()

	oldItemData := itemData
	itemData = itemDataFile{
		DefaultVolume: 2.5,
		Items: map[string]itemRule{
			"dune.item.known": {Volume: 1.25},
			"dune.item.zero":  {Volume: 0},
		},
	}
	t.Cleanup(func() { itemData = oldItemData })

	override := pgtype.Float8{Float64: 3.0, Valid: true}
	if got := inventoryItemVolume("any", override); got != 3.0 {
		t.Fatalf("expected override volume 3.0, got %v", got)
	}
	if got := inventoryItemVolume("Dune.Item.Known", pgtype.Float8{}); got != 1.25 {
		t.Fatalf("expected known-rule volume 1.25, got %v", got)
	}
	if got := inventoryItemVolume("Dune.Item.Zero", pgtype.Float8{}); got != 0 {
		t.Fatalf("expected zero volume from rule, got %v", got)
	}
	if got := inventoryItemVolume("Dune.Item.Unknown", pgtype.Float8{}); got != 2.5 {
		t.Fatalf("expected default volume 2.5, got %v", got)
	}
}

func TestFormatGiveItemResult(t *testing.T) {
	t.Parallel()

	got := formatGiveItemResult(42, "Dune.Item", 3, 1, 2)
	want := "Added 3 × Dune.Item to player 42 (1 stack(s) topped up, 2 new stack(s))"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestEnsureGiveItemVolumeCapacity(t *testing.T) {
	t.Parallel()

	oldItemData := itemData
	itemData = itemDataFile{
		Items: map[string]itemRule{
			"dune.item": {Volume: 2},
		},
	}
	t.Cleanup(func() { itemData = oldItemData })

	inv := giveItemInventory{hasVolumeCap: true, maxVolume: 10}
	state := giveItemInventoryState{usedVolume: 4}

	if err := ensureGiveItemVolumeCapacity(t.Context(), inv, state, "Dune.Item", 3); err != nil {
		t.Fatalf("expected capacity to fit, got %v", err)
	}
	err := ensureGiveItemVolumeCapacity(t.Context(), inv, state, "Dune.Item", 4)
	if err == nil {
		t.Fatalf("expected volume-capacity error")
	}
}

func TestMaxItemsByVolume(t *testing.T) {
	t.Parallel()

	if got := maxItemsByVolume(100, 40, 15); got != 4 {
		t.Fatalf("expected 4 items by volume, got %d", got)
	}
	if got := maxItemsByVolume(100, 140, 10); got != 0 {
		t.Fatalf("expected clamped 0 items by volume, got %d", got)
	}
}

func TestRequiredStackCount(t *testing.T) {
	t.Parallel()

	if got := requiredStackCount(10, 3); got != 4 {
		t.Fatalf("expected 4 required stacks, got %d", got)
	}
	if got := requiredStackCount(1, 1); got != 1 {
		t.Fatalf("expected 1 required stack, got %d", got)
	}
}
