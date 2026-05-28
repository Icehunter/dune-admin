package main

import "testing"

func TestBuildBlueprintInstance(t *testing.T) {
	t.Parallel()

	instance, ok := buildBlueprintInstance(7, "TypeA", []float32{1, 2, 3, 4}, true)
	if !ok {
		t.Fatalf("expected valid transform to produce an instance")
	}
	if instance.InstanceID == nil || *instance.InstanceID != 7 {
		t.Fatalf("unexpected instance id: %+v", instance.InstanceID)
	}
	if instance.BuildingType != "TypeA" || instance.X != 1 || instance.Y != 2 || instance.Z != 3 || instance.Rotation != 4 {
		t.Fatalf("unexpected instance payload: %+v", instance)
	}
	if instance.ProvidesStability == nil || !*instance.ProvidesStability {
		t.Fatalf("expected stability=true pointer in instance")
	}

	if _, ok := buildBlueprintInstance(1, "TypeB", []float32{1, 2, 3}, false); ok {
		t.Fatalf("expected short transform to be rejected")
	}
}

func TestBuildBlueprintPlaceable(t *testing.T) {
	t.Parallel()

	placeable, ok := buildBlueprintPlaceable(9, "TypeP", []float32{1, 2, 3, 4, 5, 6})
	if !ok {
		t.Fatalf("expected valid transform to produce a placeable")
	}
	if placeable.PlaceableID == nil || *placeable.PlaceableID != 9 {
		t.Fatalf("unexpected placeable id: %+v", placeable.PlaceableID)
	}
	if placeable.BuildingType != "TypeP" || placeable.X != 1 || placeable.Y != 2 || placeable.Z != 3 || placeable.RX != 4 || placeable.RY != 5 || placeable.RZ != 6 {
		t.Fatalf("unexpected placeable payload: %+v", placeable)
	}

	if _, ok := buildBlueprintPlaceable(1, "TypeBad", []float32{1, 2, 3, 4, 5}); ok {
		t.Fatalf("expected short placeable transform to be rejected")
	}
}

func TestBuildBlueprintPentashield(t *testing.T) {
	t.Parallel()

	pentashield, ok := buildBlueprintPentashield(11, []int16{10, 20, 30})
	if !ok {
		t.Fatalf("expected valid scale to produce pentashield")
	}
	if pentashield.PlaceableID != 11 || pentashield.Scale != [3]int{10, 20, 30} {
		t.Fatalf("unexpected pentashield payload: %+v", pentashield)
	}

	if _, ok := buildBlueprintPentashield(1, []int16{1, 2}); ok {
		t.Fatalf("expected short pentashield scale to be rejected")
	}
}
