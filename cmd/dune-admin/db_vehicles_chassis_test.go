package main

import "testing"

// Vehicle chassis condition (#329: "Vehicles > Type does not get populated /
// vehicle health ... sometimes a percentage for the Chassis health shown and
// sometimes not").
//
// The game only writes DecayedMaxDurability into
// dune.vehicle_modules.stats once a part has actually decayed. Measured on a
// live 1.5.3 server, 12 of 13 chassis modules had CurrentDurability but no
// DecayedMaxDurability, so the query COALESCEd the max to 0, chassisConditionPct
// returned has=false, and the UI showed nothing at all for those vehicles.
//
// A missing decayed max does not mean "unknown" — it means the part has never
// decayed, so its ceiling is still the template's base durability, which
// item-data.json records. These are the real template/value pairs observed on
// that server; every chassis template in use resolves.
func f64ptr(f float64) *float64 { return &f }

func stubItemLookup(items map[string]itemRule) func(string) (itemRule, bool) {
	return func(id string) (itemRule, bool) {
		r, ok := items[id]
		return r, ok
	}
}

func TestResolveChassisMax(t *testing.T) {
	t.Parallel()
	catalog := map[string]itemRule{
		"BuggyChassis_6":                {MaxDurability: f64ptr(3500)},
		"OrnithopterLightChassis_4":     {MaxDurability: f64ptr(3500)},
		"OrnithopterLightChassis_5":     {MaxDurability: f64ptr(3500)},
		"OrnithopterMediumChassis_6":    {MaxDurability: f64ptr(4000)},
		"OrnithopterTransportChassis_6": {MaxDurability: f64ptr(6000)},
		"SandbikeChassis_6":             {MaxDurability: f64ptr(2000)},
		"TreadwheelChassis_6":           {MaxDurability: f64ptr(2000)},
		"NoDurabilityRecorded":          {MaxDurability: nil},
		"ZeroDurability":                {MaxDurability: f64ptr(0)},
	}
	lookup := stubItemLookup(catalog)

	tests := []struct {
		name       string
		decayedMax float64
		templateID string
		want       float64
	}{
		{
			name:       "decayed max wins when the game recorded one",
			decayedMax: 3499.980957,
			templateID: "OrnithopterLightChassis_5",
			want:       3499.980957,
		},
		{name: "buggy falls back to template base", templateID: "BuggyChassis_6", want: 3500},
		{name: "light ornithopter falls back", templateID: "OrnithopterLightChassis_4", want: 3500},
		{name: "medium ornithopter falls back", templateID: "OrnithopterMediumChassis_6", want: 4000},
		{name: "transport ornithopter falls back", templateID: "OrnithopterTransportChassis_6", want: 6000},
		{name: "sandbike falls back", templateID: "SandbikeChassis_6", want: 2000},
		{name: "treadwheel falls back", templateID: "TreadwheelChassis_6", want: 2000},

		// Error paths: never invent a ceiling we cannot source.
		{name: "unknown template stays unknown", templateID: "NotInCatalog", want: 0},
		{name: "empty template stays unknown", templateID: "", want: 0},
		{name: "template with no durability stays unknown", templateID: "NoDurabilityRecorded", want: 0},
		{name: "template with zero durability stays unknown", templateID: "ZeroDurability", want: 0},
		{
			name:       "negative decayed max is ignored, falls back",
			decayedMax: -5,
			templateID: "BuggyChassis_6",
			want:       3500,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveChassisMax(tt.decayedMax, tt.templateID, lookup); got != tt.want {
				t.Errorf("resolveChassisMax(%v, %q) = %v, want %v",
					tt.decayedMax, tt.templateID, got, tt.want)
			}
		})
	}
}

// TestChassisPctWithResolvedMax checks the end-to-end display outcome for the
// exact rows that were showing nothing on the live server.
func TestChassisPctWithResolvedMax(t *testing.T) {
	t.Parallel()
	lookup := stubItemLookup(map[string]itemRule{
		"OrnithopterLightChassis_4":  {MaxDurability: f64ptr(3500)},
		"OrnithopterMediumChassis_5": {MaxDurability: f64ptr(4000)},
		"BuggyChassis_6":             {MaxDurability: f64ptr(3500)},
	})

	tests := []struct {
		name     string
		current  float64
		decayed  float64
		template string
		wantPct  float64
		wantHas  bool
	}{
		{
			name: "slightly worn light ornithopter now reports a percentage",
			// vehicle 1376 on the live server
			current: 3499.276367, template: "OrnithopterLightChassis_4",
			wantPct: 3499.276367 / 3500 * 100, wantHas: true,
		},
		{
			name: "slightly worn medium ornithopter now reports a percentage",
			// vehicle 1377
			current: 3999.429687, template: "OrnithopterMediumChassis_5",
			wantPct: 3999.429687 / 4000 * 100, wantHas: true,
		},
		{
			name:    "pristine buggy reports 100%",
			current: 3500, template: "BuggyChassis_6",
			wantPct: 100, wantHas: true,
		},
		{
			name: "module with no durability data at all still reports nothing",
			// vehicle 531 had neither CurrentDurability nor a decayed max
			current: 0, template: "UnknownChassis",
			wantPct: 0, wantHas: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			maxd := resolveChassisMax(tt.decayed, tt.template, lookup)
			gotPct, gotHas := chassisConditionPct(tt.current, maxd)
			if gotHas != tt.wantHas {
				t.Fatalf("has percentage = %v, want %v (resolved max %v)", gotHas, tt.wantHas, maxd)
			}
			if gotHas && gotPct != tt.wantPct {
				t.Errorf("pct = %v, want %v", gotPct, tt.wantPct)
			}
		})
	}
}

// TestVehicleClassLabelNeverBlank covers the other half of #329 ("There is a
// complete empty row at the end of the vehicle list").
//
// The backup arm of vehiclesQuery selects COALESCE(a.class, ”) — a backup whose
// vehicle actor has since been deleted is the only remaining trace of it, so the
// row is still listed, but with no class. Class is the DataTable's row header, and
// the same arm hardcodes ” for map/owner/name, so the row rendered completely
// blank. A row that exists must say what it is.
func TestVehicleClassLabelNeverBlank(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "deleted vehicle actor yields a placeholder, not a blank row",
			raw:  "",
			want: deletedVehicleClassLabel,
		},
		{
			name: "whitespace-only class is treated as absent",
			raw:  "   ",
			want: deletedVehicleClassLabel,
		},
		{
			name: "a real class is unchanged",
			raw:  "/Game/Dune/Systems/Vehicles/Blueprints/GroundVehicles/BP_Buggy_CHOAM.BP_Buggy_CHOAM_C",
			want: "Buggy CHOAM",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := vehicleClassLabel(tt.raw); got != tt.want {
				t.Errorf("vehicleClassLabel(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
