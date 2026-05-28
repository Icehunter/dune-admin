package marketbot

import (
	"fmt"
	"testing"
)

func TestUniqueSchematicsMask(t *testing.T) {
	cases := []struct {
		category string
		wantMask int32
	}{
		// GARMENTS(0) → UNIQUE SCHEMATICS(5) → lightarmor(0)/heavyarmor(1)/stillsuits(2)/social(4)
		{"items/garment/lightarmor/chest", 0x00050000},
		{"items/garment/heavyarmor/head", 0x00050100},
		{"items/garment/stillsuits/hands", 0x00050200},
		{"items/garment/socialwearables/chest", 0x00050400},
		// WEAPONS(1) → UNIQUE SCHEMATICS(3) — confirmed D3 codes from in-game screenshots
		{"items/weapons/shortblades", 0x01030000},
		{"items/weapons/longblades", 0x01030100},
		{"items/weapons/pistol", 0x01030200},      // Maula Pistol (Light.Pistol)
		{"items/weapons/heavypistol", 0x01030300}, // Karpov 38 (Heavy.Pistol)
		{"items/weapons/heavyrifle", 0x01030400},  // GRDA 44 (Heavy.Rifle)
		{"items/weapons/smg", 0x01030500},         // Disruptor M11
		{"items/weapons/spitdart", 0x01030600},    // Jabal Spitdart
		{"items/weapons/shotgun", 0x01030700},     // Rafiq Snubnose (Light.Shotgun)
		{"items/weapons/battlerifle", 0x01030800}, // Drillshot FK7 (was wrongly 3 in old code)
		{"items/weapons/heavyshotgun", 0x01030900},
		{"items/weapons/missilelauncher", 0x01030A00},
		{"items/weapons/flamethrower", 0x01030B00},
		{"items/weapons/fireballer", 0x01030C00}, // Pyrocket
		{"items/weapons/lasgun", 0x01030D00},
		// VEHICLES(2) → UNIQUE SCHEMATICS(6)
		{"items/vehicles/sandbike", 0x02060000},
		{"items/vehicles/buggy", 0x02060100},
		{"items/vehicles/lightornithopter", 0x02060200},
		{"items/vehicles/mediumornithopter", 0x02060300},
		{"items/vehicles/transportornithopter", 0x02060400},
		{"items/vehicles/sandcrawler", 0x02060500},
		// UTILITY(3) → UNIQUE SCHEMATICS(7)
		{"items/utility/deployables", 0x03070000},
		{"items/utility/watertools", 0x03070100},
		{"items/utility/cutteray", 0x03070300},
		{"items/utility/suspensor", 0x03070700},
		{"items/utility/powerpack", 0x03070800},
		// AUGMENTATIONS(4) → UNIQUE SCHEMATICS(4)
		{"items/augment/armor", 0x04040000},
		{"items/augment/melee", 0x04040100},
		{"items/augment/ranged", 0x04040200},
		{"items/augment/misc", 0x04040300},
	}

	for _, tc := range cases {
		mask, depth, ok := UniqueSchematicsMask(tc.category)
		if !ok {
			t.Errorf("%-50s: got ok=false, want mask=0x%08X", tc.category, uint32(tc.wantMask))
			continue
		}
		if mask != tc.wantMask {
			t.Errorf("%-50s: got=0x%08X depth=%d want=0x%08X", tc.category, uint32(mask), depth, uint32(tc.wantMask))
		} else {
			fmt.Printf("OK %-50s 0x%08X depth=%d\n", tc.category, uint32(mask), depth)
		}
	}

	// Only MISC has no UNIQUE SCHEMATICS section
	for _, cat := range []string{"items/misc/components", "items/misc/rawresources"} {
		if _, _, ok := UniqueSchematicsMask(cat); ok {
			t.Errorf("%s: expected ok=false (no unique schematics remapping)", cat)
		}
	}
}

func TestCategoryMask(t *testing.T) {
	cases := []struct {
		category string
		wantD1   byte
		wantD2   byte
		wantD3   byte
		wantMask int32
	}{
		// Weapons: shortblades/longblades remapped to items/weapons/melee/*
		{"items/weapons/shortblades", 1, 0, 0, 0x01000000},
		{"items/weapons/longblades", 1, 0, 1, 0x01000100},
		// Ranged weapon item types (depth-2 codes, same order as UniqueSchematicsMask d3)
		{"items/weapons/pistol", 1, 2, 0, 0x01020000},
		{"items/weapons/heavypistol", 1, 3, 0, 0x01030000},
		{"items/weapons/heavyrifle", 1, 4, 0, 0x01040000},
		{"items/weapons/smg", 1, 5, 0, 0x01050000},
		{"items/weapons/spitdart", 1, 6, 0, 0x01060000},
		{"items/weapons/shotgun", 1, 7, 0, 0x01070000},
		{"items/weapons/battlerifle", 1, 8, 0, 0x01080000},
		{"items/weapons/heavyshotgun", 1, 9, 0, 0x01090000},
		{"items/weapons/missilelauncher", 1, 10, 0, 0x010A0000},
		{"items/weapons/flamethrower", 1, 11, 0, 0x010B0000},
		{"items/weapons/fireballer", 1, 12, 0, 0x010C0000},
		{"items/weapons/lasgun", 1, 13, 0, 0x010D0000},
		{"items/weapons/ammunition", 1, 14, 0, 0x010E0000},
		// AUGMENTATIONS item types including GENERIC/misc
		{"items/augment/armor", 4, 0, 0, 0x04000000},
		{"items/augment/melee", 4, 1, 0, 0x04010000},
		{"items/augment/ranged", 4, 2, 0, 0x04020000},
		{"items/augment/misc", 4, 3, 0, 0x04030000},
		// MISC
		{"items/misc/refinedresources", 5, 1, 0, 0x05010000},
		{"items/misc/components", 5, 2, 0, 0x05020000},
		{"items/misc/rawresources", 5, 3, 0, 0x05030000},
		// GARMENTS
		{"items/garment/lightarmor/chest", 0, 0, 1, 0x00000100},
		{"items/garment/heavyarmor/head", 0, 1, 0, 0x00010000},
		// VEHICLES
		{"items/vehicles/mediumornithopter/chassis", 2, 3, 0, 0x02030000},
		// UTILITY
		{"items/utility/consumables", 3, 6, 0, 0x03060000},
		{"items/utility/gatheringtools/cutteray", 3, 3, 0, 0x03030000},
	}

	catalog := make([]CatalogItem, len(cases))
	for i, tc := range cases {
		catalog[i] = CatalogItem{Category: tc.category}
	}
	idx := buildSegmentIndex(catalog)

	for _, tc := range cases {
		mask, _ := CategoryMask(tc.category, idx)
		d1 := byte((mask >> 24) & 0xFF)
		d2 := byte((mask >> 16) & 0xFF)
		d3 := byte((mask >> 8) & 0xFF)
		d0 := byte(mask & 0xFF)
		if mask != tc.wantMask {
			t.Errorf("%-50s got=0x%08X want=0x%08X [d1=%d d2=%d d3=%d d0=%d]",
				tc.category, uint32(mask), uint32(tc.wantMask), d1, d2, d3, d0)
		} else {
			fmt.Printf("OK %-50s 0x%08X\n", tc.category, uint32(mask))
		}
	}
}
