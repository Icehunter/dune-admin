package main

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidateRepairVehicleInput(t *testing.T) {
	originalDB := globalDB
	t.Cleanup(func() { globalDB = originalDB })

	globalDB = nil
	if err := validateRepairVehicleInput(42); err == nil || err.Error() != "not connected" {
		t.Fatalf("expected not connected error, got %v", err)
	}

	globalDB = &pgxpool.Pool{}
	if err := validateRepairVehicleInput(0); err == nil || err.Error() != "player ID required" {
		t.Fatalf("expected player ID required error, got %v", err)
	}
	if err := validateRepairVehicleInput(42); err != nil {
		t.Fatalf("expected valid input, got %v", err)
	}
}

func TestVehicleRepairTarget(t *testing.T) {
	originalItemData := itemData
	itemData = itemDataFile{
		Items: map[string]itemRule{
			"dune.vehicle.catalog": {MaxDurability: floatPtr(300)},
		},
	}
	t.Cleanup(func() { itemData = originalItemData })

	target, ok := vehicleRepairTarget("Dune.Vehicle.Catalog")
	if !ok || target != 300 {
		t.Fatalf("expected catalog target=300, ok=true, got target=%v ok=%v", target, ok)
	}

	if target, ok = vehicleRepairTarget("Dune.Vehicle.Unknown"); ok || target != 0 {
		t.Fatalf("expected unknown template to be skipped, got target=%v ok=%v", target, ok)
	}
}

func TestRunVehicleModuleRepairs(t *testing.T) {
	originalItemData := itemData
	itemData = itemDataFile{
		Items: map[string]itemRule{
			"dune.vehicle.repairable": {MaxDurability: floatPtr(125)},
		},
	}
	t.Cleanup(func() { itemData = originalItemData })

	modules := []vehicleModule{
		{id: 1, templateID: "Dune.Vehicle.Repairable"},
		{id: 2, templateID: "Dune.Vehicle.Missing"},
		{id: 3, templateID: "Dune.Vehicle.Repairable"},
	}

	var repairedIDs []int64
	summary := runVehicleModuleRepairs(modules, func(module vehicleModule, target float64) error {
		if target != 125 {
			t.Fatalf("expected target 125, got %v", target)
		}
		repairedIDs = append(repairedIDs, module.id)
		return nil
	})
	if summary.err != nil {
		t.Fatalf("unexpected error: %v", summary.err)
	}
	if summary.total != 3 || summary.repaired != 2 || summary.skipped != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if len(repairedIDs) != 2 || repairedIDs[0] != 1 || repairedIDs[1] != 3 {
		t.Fatalf("unexpected repaired IDs: %v", repairedIDs)
	}
}

func TestRunVehicleModuleRepairs_StopsOnError(t *testing.T) {
	originalItemData := itemData
	itemData = itemDataFile{
		Items: map[string]itemRule{
			"dune.vehicle.repairable": {MaxDurability: floatPtr(125)},
		},
	}
	t.Cleanup(func() { itemData = originalItemData })

	modules := []vehicleModule{
		{id: 10, templateID: "Dune.Vehicle.Repairable"},
		{id: 11, templateID: "Dune.Vehicle.Repairable"},
		{id: 12, templateID: "Dune.Vehicle.Missing"},
	}

	failErr := errors.New("boom")
	summary := runVehicleModuleRepairs(modules, func(module vehicleModule, _ float64) error {
		if module.id == 11 {
			return failErr
		}
		return nil
	})
	if summary.err == nil {
		t.Fatalf("expected repair error")
	}
	if summary.err.Error() != "repair module 11: boom" {
		t.Fatalf("unexpected error: %v", summary.err)
	}
	if summary.total != 3 || summary.repaired != 1 || summary.skipped != 0 {
		t.Fatalf("unexpected summary after failure: %+v", summary)
	}
}
