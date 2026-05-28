package main

import (
	"math"
	"testing"
)

func nearlyEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) <= epsilon
}

func TestParseBasePathID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantID    int64
		wantError string
	}{
		{name: "valid", input: "123", wantID: 123},
		{name: "invalid", input: "abc", wantError: "invalid id"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseBasePathID(tt.input)
			if tt.wantError != "" {
				if err == nil || err.Error() != tt.wantError {
					t.Fatalf("expected error %q, got %v", tt.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantID {
				t.Fatalf("expected ID %d, got %d", tt.wantID, got)
			}
		})
	}
}

func TestCalculateBaseCentroid(t *testing.T) {
	t.Parallel()

	raws := []rawBaseInstance{
		{transform: []float32{10, 20, 30, 0, 0, 0, 1}},
		{transform: []float32{14, 24, 34, 0, 0, 0, 1}},
	}

	cx, cy, cz := calculateBaseCentroid(raws)
	if cx != 12 || cy != 22 || cz != 32 {
		t.Fatalf("unexpected centroid: (%v, %v, %v)", cx, cy, cz)
	}
}

func TestBuildBlueprintInstances(t *testing.T) {
	t.Parallel()

	raws := []rawBaseInstance{
		{
			buildingType: "A",
			transform:    []float32{10, 20, 30, 0, 0, 0, 1},
		},
		{
			buildingType: "B",
			transform:    []float32{14, 24, 34, 0, 0, 0.70710677, 0.70710677},
		},
	}

	cx, cy, cz := calculateBaseCentroid(raws)
	got := buildBlueprintInstances(raws, cx, cy, cz)

	if len(got) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(got))
	}

	if got[0].BuildingType != "A" || got[1].BuildingType != "B" {
		t.Fatalf("unexpected building types: %#v", got)
	}

	if got[0].X != -2 || got[0].Y != -2 || got[0].Z != -2 {
		t.Fatalf("unexpected first offsets: %+v", got[0])
	}
	if !nearlyEqual(got[0].Rotation, 0, 0.001) {
		t.Fatalf("unexpected first rotation: %v", got[0].Rotation)
	}
	if !nearlyEqual(got[1].Rotation, 90, 0.01) {
		t.Fatalf("unexpected second rotation: %v", got[1].Rotation)
	}
}

func TestConvertExportPlaceable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		raw            rawBasePlaceable
		cx             float64
		cy             float64
		cz             float64
		placeableID    int
		wantInclude    bool
		wantHasPenta   bool
		wantPlaceableX float64
	}{
		{
			name:        "skip totem",
			raw:         rawBasePlaceable{buildingType: "Totem_Placeable"},
			wantInclude: false,
		},
		{
			name: "skip invalid transform",
			raw: rawBasePlaceable{
				buildingType: "SomeType_Placeable",
				location:     "invalid",
				rotation:     "(0,0,0,1)",
			},
			wantInclude: false,
		},
		{
			name: "normal placeable",
			raw: rawBasePlaceable{
				buildingType: "SomeType_Placeable",
				location:     "(10,20,30)",
				rotation:     "(0,0,0,1)",
			},
			cx:             1,
			cy:             2,
			cz:             3,
			wantInclude:    true,
			wantHasPenta:   false,
			wantPlaceableX: 9,
		},
		{
			name: "pentashield with scale",
			raw: rawBasePlaceable{
				buildingType: "MyPentashieldSurface_Placeable",
				location:     "(10,20,30)",
				rotation:     "(0,0,0,1)",
				properties: map[string]any{
					"MyPentashieldSurface_C": map[string]any{
						"m_Scale": []any{3.0, 4.0, 5.0},
					},
				},
			},
			cx:             1,
			cy:             2,
			cz:             3,
			placeableID:    7,
			wantInclude:    true,
			wantHasPenta:   true,
			wantPlaceableX: 9,
		},
		{
			name: "pentashield without scale skipped",
			raw: rawBasePlaceable{
				buildingType: "MyPentashieldSurface_Placeable",
				location:     "(10,20,30)",
				rotation:     "(0,0,0,1)",
				properties: map[string]any{
					"MyPentashieldSurface_C": map[string]any{},
				},
			},
			wantInclude: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			placeable, pentashield, include := convertExportPlaceable(tt.raw, tt.cx, tt.cy, tt.cz, tt.placeableID)
			if include != tt.wantInclude {
				t.Fatalf("expected include=%v, got %v", tt.wantInclude, include)
			}
			if !include {
				return
			}
			if placeable.X != tt.wantPlaceableX {
				t.Fatalf("unexpected placeable X: %v", placeable.X)
			}
			if (pentashield != nil) != tt.wantHasPenta {
				t.Fatalf("expected pentashield=%v, got %v", tt.wantHasPenta, pentashield != nil)
			}
			if pentashield != nil {
				if pentashield.PlaceableID != tt.placeableID {
					t.Fatalf("expected pentashield placeable id %d, got %d", tt.placeableID, pentashield.PlaceableID)
				}
				if pentashield.Scale != [3]int{3, 4, 5} {
					t.Fatalf("unexpected pentashield scale: %#v", pentashield.Scale)
				}
			}
		})
	}
}

func TestBuildBlueprintPlaceables_AssignsPentashieldIndex(t *testing.T) {
	t.Parallel()

	raws := []rawBasePlaceable{
		{buildingType: "Totem_Placeable"},
		{
			buildingType: "Normal_Placeable",
			location:     "(1,1,1)",
			rotation:     "(0,0,0,1)",
		},
		{
			buildingType: "ShieldPentashieldSurface_Placeable",
			location:     "(2,2,2)",
			rotation:     "(0,0,0,1)",
			properties: map[string]any{
				"ShieldPentashieldSurface_C": map[string]any{
					"m_Scale": []any{1.0, 2.0, 3.0},
				},
			},
		},
	}

	placeables, pentashields := buildBlueprintPlaceables(raws, 0, 0, 0)
	if len(placeables) != 2 {
		t.Fatalf("expected 2 placeables, got %d", len(placeables))
	}
	if len(pentashields) != 1 {
		t.Fatalf("expected 1 pentashield, got %d", len(pentashields))
	}
	if pentashields[0].PlaceableID != 1 {
		t.Fatalf("expected pentashield PlaceableID=1, got %d", pentashields[0].PlaceableID)
	}
}
