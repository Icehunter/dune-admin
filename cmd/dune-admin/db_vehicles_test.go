package main

import "testing"

// Vehicle actor names default to '##<Type>' (e.g. ##MediumOrnithopter) or
// 'None' when the player never renamed the vehicle. Those are placeholders, not
// names, and must not reach the UI (#313).
func TestCleanVehicleName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"default hash name", "##MediumOrnithopter", ""},
		{"default buggy", "##BuggyChoam", ""},
		{"literal None", "None", ""},
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"real player name", "ICE-Scout-Stinger", "ICE-Scout-Stinger"},
		{"real name is trimmed", "  Sand Rider  ", "Sand Rider"},
		{"hash mid-string is kept", "Rig##2", "Rig##2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanVehicleName(tc.in); got != tc.want {
				t.Errorf("cleanVehicleName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// permission_actor_rank.rank is 1 for the owner (exactly one per vehicle) and
// 2+ for players granted access. The old query filtered on player_id without
// distinguishing, so a vehicle you merely had access to looked like your own.
func TestVehicleAccessLabel(t *testing.T) {
	cases := []struct {
		rank int
		want string
	}{
		{1, "owner"},
		{2, "granted"},
		{3, "granted"},
		{0, "unknown"},
		{-1, "unknown"},
	}
	for _, tc := range cases {
		if got := vehicleAccessLabel(tc.rank); got != tc.want {
			t.Errorf("vehicleAccessLabel(%d) = %q, want %q", tc.rank, got, tc.want)
		}
	}
}

func TestVehicleIsOwner(t *testing.T) {
	if !vehicleIsOwner(1) {
		t.Error("rank 1 must be owner")
	}
	for _, r := range []int{0, 2, 3, 99} {
		if vehicleIsOwner(r) {
			t.Errorf("rank %d must not be owner", r)
		}
	}
}

// Vehicles live on a specific partition and dimension. A stock server spreads
// them across sietches (HaggaBasin p1, Story_Faction_Outpost_Atre p16), which
// the old single-map column could not express.
func TestFormatVehicleLocation(t *testing.T) {
	cases := []struct {
		name      string
		mapName   string
		partition int64
		dimension int
		want      string
	}{
		{"map with partition and dimension", "HaggaBasin", 1, 0, "HaggaBasin p1/d0"},
		{"other sietch", "Story_Faction_Outpost_Atre", 16, 0, "Story_Faction_Outpost_Atre p16/d0"},
		{"nonzero dimension", "HaggaBasin", 2, 3, "HaggaBasin p2/d3"},
		{"no map falls back to partition", "", 4, 0, "p4/d0"},
		{"nothing known", "", 0, 0, ""},
		{"map only, no partition", "HaggaBasin", 0, 0, "HaggaBasin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatVehicleLocation(tc.mapName, tc.partition, tc.dimension)
			if got != tc.want {
				t.Errorf("formatVehicleLocation(%q,%d,%d) = %q, want %q",
					tc.mapName, tc.partition, tc.dimension, got, tc.want)
			}
		})
	}
}

// The old column showed COALESCE(recovered_vehicles.chassis_durability, 1.0) as
// a percentage. recovered_vehicles is empty on a normal server, so every
// vehicle read "100%" — a fabricated number. Real durability comes from
// vehicle_modules, where only a minority of parts carry a max, so a percentage
// is only reported when the denominator genuinely exists.
func TestChassisCondition(t *testing.T) {
	cases := []struct {
		name    string
		cur     float64
		max     float64
		wantPct float64
		wantHas bool
	}{
		{"max present", 1750, 3500, 50, true},
		{"max equals current", 3499.98, 3499.98, 100, true},
		{"no max known", 3500, 0, 0, false},
		{"no data at all", 0, 0, 0, false},
		{"max present but zero current", 0, 2000, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pct, has := chassisConditionPct(tc.cur, tc.max)
			if has != tc.wantHas {
				t.Fatalf("hasPct = %v, want %v", has, tc.wantHas)
			}
			if has && pct != tc.wantPct {
				t.Errorf("pct = %v, want %v", pct, tc.wantPct)
			}
		})
	}
}

// A negative or inverted max must never produce a nonsense percentage.
func TestChassisCondition_RejectsBadDenominator(t *testing.T) {
	if _, has := chassisConditionPct(100, -5); has {
		t.Error("negative max must not yield a percentage")
	}
}

// Vehicle classes arrive as Unreal paths whose object name repeats the package
// name: /Game/.../BP_Buggy_CHOAM.BP_Buggy_CHOAM_C. shortClass alone leaves
// "BP_Buggy_CHOAM.BP_Buggy_CHOAM", which is twice as long as it needs to be and
// gets clipped in the table (#313).
func TestVehicleClassLabel(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"buggy",
			"/Game/Dune/Systems/Vehicles/Blueprints/GroundVehicles/BP_Buggy_CHOAM.BP_Buggy_CHOAM_C",
			"Buggy CHOAM",
		},
		{
			"light ornithopter",
			"/Game/Dune/Systems/Vehicles/Blueprints/FlyingVehicles/BP_LightOrnithopter_Choam.BP_LightOrnithopter_Choam_C",
			"Light Ornithopter Choam",
		},
		{
			"treadwheel",
			"/Game/Dune/Systems/Vehicles/Blueprints/GroundVehicles/BP_TreadWheel.BP_TreadWheel_C",
			"Tread Wheel",
		},
		{
			"transport ornithopter",
			"/Game/Dune/Systems/Vehicles/Blueprints/FlyingVehicles/BP_TransportOrnithopter_CHOAM.BP_TransportOrnithopter_CHOAM_C",
			"Transport Ornithopter CHOAM",
		},
		{
			"sandbike",
			"/Game/Dune/Systems/Vehicles/Blueprints/GroundVehicles/BP_Sandbike_CHOAM.BP_Sandbike_CHOAM_C",
			"Sandbike CHOAM",
		},
		{"already short", "BP_Buggy_CHOAM", "Buggy CHOAM"},
		// Only a package name that matches its object name is collapsed; a
		// genuinely different pair is left joined, though the rest of the
		// cleanup still applies.
		{"package differs from object", "BP_Thing.BP_Other_C", "Thing.BP Other"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := vehicleClassLabel(tc.in); got != tc.want {
				t.Errorf("vehicleClassLabel(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// An acronym must not be exploded into single letters by the camel-case split.
func TestSplitCamelCase_PreservesAcronyms(t *testing.T) {
	if got := splitCamelCase("CHOAM"); got != "CHOAM" {
		t.Errorf("splitCamelCase(CHOAM) = %q, want CHOAM", got)
	}
	if got := splitCamelCase("OneManGroundcarChoam"); got != "One Man Groundcar Choam" {
		t.Errorf("got %q", got)
	}
}
