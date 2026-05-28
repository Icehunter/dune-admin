package main

import (
	"strings"
	"testing"
)

func boolPtr(v bool) *bool {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func TestBlueprintItemStatsJSON(t *testing.T) {
	withName := blueprintItemStatsJSON(123, "My Blueprint")
	if !strings.Contains(withName, `"PlayerBlueprintId":"!!bbp#123"`) {
		t.Fatalf("missing blueprint id in stats JSON: %s", withName)
	}
	if !strings.Contains(withName, `"BuildingBlueprintName":"My Blueprint"`) {
		t.Fatalf("missing blueprint name in stats JSON: %s", withName)
	}

	withoutName := blueprintItemStatsJSON(77, "")
	if strings.Contains(withoutName, "BuildingBlueprintName") {
		t.Fatalf("expected no blueprint name when empty, got: %s", withoutName)
	}
}

func TestResolveBlueprintImportInstance(t *testing.T) {
	inst := blueprintInstance{
		BuildingType: "Atreides_Outpost_Foundation",
		X:            1,
		Y:            2,
		Z:            3,
		Rotation:     90,
	}
	id, transform, stability := resolveBlueprintImportInstance(50, 2, inst)
	if id != 53 {
		t.Fatalf("expected fallback instance id 53, got %d", id)
	}
	if transform != "{1,2,3,90}" {
		t.Fatalf("unexpected transform: %q", transform)
	}
	if !stability {
		t.Fatal("expected structural building fallback to set stability=true")
	}

	customID := 900
	inst.InstanceID = intPtr(customID)
	inst.ProvidesStability = boolPtr(false)
	id, _, stability = resolveBlueprintImportInstance(0, 0, inst)
	if id != customID {
		t.Fatalf("expected explicit instance id %d, got %d", customID, id)
	}
	if stability {
		t.Fatal("expected explicit ProvidesStability override to win")
	}
}

func TestResolveBlueprintImportPlaceable(t *testing.T) {
	pl := blueprintPlaceable{
		BuildingType: "SomePlaceable",
		X:            4,
		Y:            5,
		Z:            6,
		RX:           1,
		RY:           2,
		RZ:           3,
	}
	id, transform := resolveBlueprintImportPlaceable(10, 1, pl)
	if id != 12 {
		t.Fatalf("expected fallback placeable id 12, got %d", id)
	}
	if transform != "{4,5,6,1,2,3}" {
		t.Fatalf("unexpected transform: %q", transform)
	}

	customID := 321
	pl.PlaceableID = intPtr(customID)
	id, _ = resolveBlueprintImportPlaceable(0, 0, pl)
	if id != customID {
		t.Fatalf("expected explicit placeable id %d, got %d", customID, id)
	}
}
