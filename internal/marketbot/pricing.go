package marketbot

import (
	"math"
	"sort"
	"strings"
)

// buildSegmentIndex collects every unique path segment at each depth level
// across the full catalog, returning a sorted slice per level used by CategoryMask.
func buildSegmentIndex(catalog []CatalogItem) [4][]string {
	sets := [4]map[string]struct{}{}
	for i := range sets {
		sets[i] = make(map[string]struct{})
	}
	for _, item := range catalog {
		if item.Category == "" {
			continue
		}
		parts := strings.Split(item.Category, "/")
		for i, p := range parts {
			if i < 4 {
				sets[i][p] = struct{}{}
			}
		}
	}
	var idx [4][]string
	for i, s := range sets {
		for k := range s {
			idx[i] = append(idx[i], k)
		}
		sort.Strings(idx[i])
	}
	return idx
}

// Category encoding: depth1 in bits 24-31 (MSB), depth2 in bits 16-23,
// depth3 in bits 8-15, depth0 (root "items") always 0 in bits 0-7.
// Code = 0-indexed position in the game UI list, confirmed from live player orders
// and in-game category screenshots.
//
// Depth-1 UI tab order: GARMENTS(0) WEAPONS(1) VEHICLES(2) UTILITY(3) AUGMENTATIONS(4) MISC(5)
var knownCodes = [4]map[string]byte{
	0: {},
	1: {
		"garment":  0,
		"weapons":  1,
		"vehicles": 2,
		"utility":  3,
		"augment":  4,
		"misc":     5,
	},
	2: {
		// GARMENTS: LIGHT ARMOR(0) HEAVY ARMOR(1) STILLSUITS(2) UTILITY(3) SOCIAL(4)
		"lightarmor":       0,
		"heavyarmor":       1,
		"stillsuits":       2,
		"utilitywearables": 3,
		"socialwearables":  4,
		// WEAPONS: MELEE(0) individual-type codes (depth-2), AMMUNITION(2)
		// Ranged weapon item types — codes confirmed from UniqueSchematicsMask ordering
		// (same positional order used for both item depth-2 and schematic depth-3).
		"pistol":          2,
		"heavypistol":     3,
		"heavyrifle":      4,
		"smg":             5,
		"spitdart":        6,
		"shotgun":         7,
		"battlerifle":     8,
		"heavyshotgun":    9,
		"missilelauncher": 10,
		"flamethrower":    11,
		"fireballer":      12,
		"lasgun":          13,
		"ammunition":      14,
		// VEHICLES: ONE MAN(0) FOUR MAN/buggy(1) LIGHT ORNITHOPTER(2) MEDIUM ORNITHOPTER(3) CARRY-ALL/transport(4) SANDCRAWLER(5)
		"sandbike":             0,
		"buggy":                1,
		"lightornithopter":     2,
		"mediumornithopter":    3,
		"transportornithopter": 4,
		"sandcrawler":          5,
		// UTILITY: BUILDING TOOLS(0) DEPLOYABLES(1) HYDRATION TOOLS(2) GATHERING TOOLS(3) CARTOGRAPHY TOOLS(4) UTILITY TOOLS(5) CONSUMABLES(6)
		"buildingtools":    0,
		"hydrationtools":   2,
		"gatheringtools":   3,
		"cartographytools": 4,
		"utilitytools":     5,
		"consumables":      6,
		// AUGMENTATIONS: GARMENT/armor(0) MELEE(1) RANGED(2) GENERIC/misc(3)
		"armor":  0,
		"melee":  1,
		"ranged": 2,
		"misc":   3,
		// MISC: FUEL(0) REFINED RESOURCES(1) COMPONENTS(2) RAW RESOURCES(3)
		"fuel":             0,
		"refinedresources": 1,
		"components":       2,
		"rawresources":     3,
	},
	3: {
		// GATHERING TOOLS: CUTTERAY(0) COMPACTOR(1) — confirmed from player orders
		"cutteray":  0,
		"compactor": 1,
	},
}

// depth3Parent resolves depth-3 codes using (parent-depth2, segment) context.
// All codes read from in-game UI screenshots (position = code, 0-indexed).
var depth3Parent = map[[2]string]byte{
	// GARMENTS > LIGHT ARMOR: HEAD(0) CHEST(1) LEGS(2) HANDS(3) FEET(4)
	{"lightarmor", "head"}: 0, {"lightarmor", "chest"}: 1, {"lightarmor", "legs"}: 2,
	{"lightarmor", "hands"}: 3, {"lightarmor", "feet"}: 4,
	// GARMENTS > HEAVY ARMOR: HEAD(0) CHEST(1) LEGS(2) HANDS(3) FEET(4)
	{"heavyarmor", "head"}: 0, {"heavyarmor", "chest"}: 1, {"heavyarmor", "legs"}: 2,
	{"heavyarmor", "hands"}: 3, {"heavyarmor", "feet"}: 4,
	// GARMENTS > STILLSUITS: HEAD(0) CHEST(1) HANDS(2) FEET(3)
	{"stillsuits", "head"}: 0, {"stillsuits", "chest"}: 1,
	{"stillsuits", "hands"}: 2, {"stillsuits", "feet"}: 3,
	// GARMENTS > SOCIAL: CHEST(0) LEGS(1) HANDS(2) FEET(3)
	{"socialwearables", "chest"}: 0, {"socialwearables", "legs"}: 1,
	{"socialwearables", "hands"}: 2, {"socialwearables", "feet"}: 3,

	// VEHICLES > ONE MAN GROUNDCAR: CHASSIS(0) HULL(1) ENGINE(2) PSU(3) LOCOMOTION(4) UTILITY(5)
	{"sandbike", "chassis"}: 0, {"sandbike", "hull"}: 1, {"sandbike", "engine"}: 2,
	{"sandbike", "psu"}: 3, {"sandbike", "locomotion"}: 4, {"sandbike", "utility"}: 5,
	// VEHICLES > FOUR MAN GROUNDCAR: CHASSIS(0) HULL(1) REAR HULL(2) ENGINE(3) PSU(4) LOCOMOTION(5) TURRET(6) UTILITY(7)
	{"buggy", "chassis"}: 0, {"buggy", "hull"}: 1, {"buggy", "rear"}: 2,
	{"buggy", "engine"}: 3, {"buggy", "psu"}: 4, {"buggy", "locomotion"}: 5,
	{"buggy", "turret"}: 6, {"buggy", "utility"}: 7,
	// VEHICLES > LIGHT ORNITHOPTER: CHASSIS(0) COCKPIT(1) HULL(2) ENGINE(3) PSU(4) WING/locomotion(5) UTILITY(6)
	{"lightornithopter", "chassis"}: 0, {"lightornithopter", "cockpit"}: 1,
	{"lightornithopter", "hull"}: 2, {"lightornithopter", "engine"}: 3,
	{"lightornithopter", "psu"}: 4, {"lightornithopter", "locomotion"}: 5,
	{"lightornithopter", "utility"}: 6,
	// VEHICLES > MEDIUM ORNITHOPTER: CHASSIS(0) CABIN(1) COCKPIT(2) TAIL(3) ENGINE(4) PSU(5) WING/locomotion(6) UTILITY(7)
	{"mediumornithopter", "chassis"}: 0, {"mediumornithopter", "cabin"}: 1,
	{"mediumornithopter", "cockpit"}: 2, {"mediumornithopter", "tail"}: 3,
	{"mediumornithopter", "engine"}: 4, {"mediumornithopter", "psu"}: 5,
	{"mediumornithopter", "locomotion"}: 6, {"mediumornithopter", "utility"}: 7,
	// VEHICLES > CARRY-ALL: CHASSIS(0) HULL(1) ENGINE(2) PSU(3) WING/locomotion(4) UTILITY(5)
	{"transportornithopter", "chassis"}: 0, {"transportornithopter", "hull"}: 1,
	{"transportornithopter", "engine"}: 2, {"transportornithopter", "psu"}: 3,
	{"transportornithopter", "locomotion"}: 4, {"transportornithopter", "utility"}: 5,
	// VEHICLES > SANDCRAWLER: CHASSIS(0) CABIN(1) ENGINE(2) PSU(3) LOCOMOTION(4) UTILITY(5)
	{"sandcrawler", "chassis"}: 0, {"sandcrawler", "cabin"}: 1, {"sandcrawler", "engine"}: 2,
	{"sandcrawler", "psu"}: 3, {"sandcrawler", "locomotion"}: 4, {"sandcrawler", "utility"}: 5,

	// UTILITY > HYDRATION TOOLS: WATER TOOL(0) BLOOD TOOL(1)
	{"hydrationtools", "watertools"}: 0, {"hydrationtools", "bloodtools"}: 1,
	// UTILITY > UTILITY TOOLS: POWER PACK(0) SUSPENSOR BELT(1) SHIELD(2) UTILITY(3)
	{"utilitytools", "powerpack"}: 0, {"utilitytools", "suspensor"}: 1,
	{"utilitytools", "utility"}: 3,
	// UTILITY > CONSUMABLES: HEALKIT(0) SPICE(1) UTILITY(2)
	{"consumables", "utility"}: 2,
}

// weaponPathRemap corrects the structural mismatch where item-data.json has melee
// weapon sub-types at depth-2 (items/weapons/shortblades) but the game puts them at
// depth-3 under melee (items/weapons/melee/shortblades).
// Maps depth-2 segment → (d2_code, d3_code).
var weaponPathRemap = map[string][2]byte{
	"shortblades": {0, 0}, // MELEE WEAPONS(0) > SHORT BLADES(0)
	"longblades":  {0, 1}, // MELEE WEAPONS(0) > LONG BLADES(1)
}

// uniqueSchematicsD2 is the depth-2 code for UNIQUE SCHEMATICS under each
// depth-1 category. Confirmed from in-game UI screenshots (0-indexed position).
//
//	GARMENTS:      LIGHT ARMOR(0) HEAVY ARMOR(1) STILLSUITS(2) UTILITY(3) SOCIAL(4) → UNIQUE SCHEMATICS(5)
//	WEAPONS:       MELEE(0) RANGED(1) AMMUNITION(2) → UNIQUE SCHEMATICS(3)
//	VEHICLES:      ONE MAN(0) FOUR MAN(1) LIGHT ORNITHOPTER(2) MEDIUM ORNITHOPTER(3) CARRY-ALL(4) SANDCRAWLER(5) → UNIQUE SCHEMATICS(6)
//	UTILITY:       BUILDING TOOLS(0) DEPLOYABLES(1) HYDRATION(2) GATHERING(3) CARTOGRAPHY(4) UTILITY TOOLS(5) CONSUMABLES(6) → UNIQUE SCHEMATICS(7)
//	AUGMENTATIONS: GARMENT(0) MELEE(1) RANGED(2) GENERIC(3) → UNIQUE SCHEMATICS(4)
var uniqueSchematicsD2 = map[string]byte{
	"garment":  5,
	"weapons":  3,
	"vehicles": 6,
	"utility":  7,
	"augment":  4,
}

// uniqueSchematicsD3 maps the last segment of an item's category path to its
// depth-3 position within the UNIQUE SCHEMATICS subcategory.
// Confirmed from in-game UI screenshots.
var uniqueSchematicsD3 = map[string]byte{
	// GARMENTS/UNIQUE SCHEMATICS
	"lightarmor":       0,
	"heavyarmor":       1,
	"stillsuits":       2,
	"utilitywearables": 3,
	"socialwearables":  4,

	// WEAPONS/UNIQUE SCHEMATICS (confirmed from screenshots — previous mapping was wrong)
	"shortblades":     0,
	"longblades":      1,
	"pistol":          2,  // MAULA PISTOL (Light.Pistol)
	"heavypistol":     3,  // KARPOV 38 (Heavy.Pistol)
	"heavyrifle":      4,  // GRDA 44 (Heavy.Rifle / LMG)
	"smg":             5,  // DISRUPTOR M11
	"spitdart":        6,  // JABAL SPITDART
	"shotgun":         7,  // RAFIQ SNUBNOSE (Light.Shotgun)
	"battlerifle":     8,  // DRILLSHOT FK7 (Light.Rifle.BattleRifle)
	"heavyshotgun":    9,  // VULCAN GAU-92 (Heavy.Shotgun)
	"missilelauncher": 10, // MISSILE LAUNCHER
	"flamethrower":    11, // FLAMETHROWER
	"fireballer":      12, // PYROCKET (Exotic.Fireballer)
	"lasgun":          13, // LASGUN

	// VEHICLES/UNIQUE SCHEMATICS
	"sandbike":             0,
	"buggy":                1,
	"lightornithopter":     2,
	"mediumornithopter":    3,
	"transportornithopter": 4,
	"sandcrawler":          5,

	// UTILITY/UNIQUE SCHEMATICS
	"deployables":      0,
	"watertools":       1,
	"bloodtools":       2,
	"cutteray":         3,
	"staticcompactor":  4,
	"cartographytools": 5,
	"shield":           6,
	"suspensor":        7,
	"powerpack":        8,

	// AUGMENTATIONS/UNIQUE SCHEMATICS
	"armor":  0,
	"melee":  1,
	"ranged": 2,
	"misc":   3,
}

// UniqueSchematicsMask computes the category mask for Unique/Memento items,
// routing them into the UNIQUE SCHEMATICS subcategory instead of their standard
// type category. Returns ok=false if this category doesn't have a known UNIQUE
// SCHEMATICS section (vehicles, augments, misc — those stay in standard categories).
func UniqueSchematicsMask(category string) (mask int32, depth int16, ok bool) {
	parts := strings.Split(category, "/")
	if len(parts) < 3 {
		return 0, 0, false
	}
	d1seg := parts[1]
	d2code, hasUS := uniqueSchematicsD2[d1seg]
	if !hasUS {
		return 0, 0, false
	}
	d1code := knownCodes[1][d1seg]

	// The item's standard depth-2 (or depth-3 for nested paths) becomes depth-3
	// within UNIQUE SCHEMATICS.
	d3seg := parts[2]
	d3code, hasD3 := uniqueSchematicsD3[d3seg]
	if !hasD3 && len(parts) >= 4 {
		d3seg = parts[3] // e.g. items/weapons/sidearms/pistol → "pistol"
		d3code, hasD3 = uniqueSchematicsD3[d3seg]
	}
	if !hasD3 {
		// Unknown sub-type: don't reroute — fall back to standard category.
		return 0, 0, false
	}

	mask = int32(d1code)<<24 | int32(d2code)<<16 | int32(d3code)<<8
	return mask, 3, true
}

// CategoryMask computes category_mask and category_depth from an item's category path.
// Uses confirmed codes from player orders; falls back to alphabetical for unknowns.
func CategoryMask(category string, idx [4][]string) (mask int32, depth int16) {
	if category == "" {
		return 0, 0
	}
	parts := strings.Split(category, "/")
	n := len(parts)
	if n > 4 {
		n = 4
	}
	depth = int16(n - 1)
	if depth < 0 {
		depth = 0
	}

	// Melee weapons: item-data.json has items/weapons/shortblades (depth-2) but
	// the game uses items/weapons/melee/shortblades (depth-3). Remap the mask.
	if n >= 3 && parts[1] == "weapons" {
		if remap, ok := weaponPathRemap[parts[2]]; ok {
			depth = 3
			mask = int32(knownCodes[1]["weapons"])<<24 | int32(remap[0])<<16 | int32(remap[1])<<8
			return mask, depth
		}
	}

	for i := 1; i < n; i++ { // skip i=0 (root "items", always 0)
		seg := parts[i]
		code := byte(0)
		found := false
		// depth-3: check parent context first to resolve same-name conflicts
		if i == 3 && len(parts) >= 3 {
			if c, ok := depth3Parent[[2]string{parts[2], seg}]; ok {
				code, found = c, true
			}
		}
		if !found {
			if c, ok := knownCodes[i][seg]; ok {
				code, found = c, true
			}
		}
		if !found {
			for j, s := range idx[i] {
				if s == seg {
					code = byte(j + 1)
					break
				}
			}
		}
		// Bit layout: depth1→bits24-31, depth2→bits16-23, depth3→bits8-15 (confirmed)
		mask |= int32(code) << uint((4-i)*8)
	}
	return mask, depth
}

// computePrice returns the base listing price for an item.
// When vendor_price is available (nearly all items), market price = vendor_price * multiplier.
// The multiplier accounts for rarity and convenience over NPC vendors.
// Falls back to tier+stack-based pricing for the few items without a vendor price.
func computePrice(item CatalogItem, snap configValues) int64 {
	return roundPrice(basePrice(item, snap))
}

// minMeaningfulVendorPrice is the threshold below which vendor_price is treated as a
// placeholder (the game uses 1–2 for "can't be sold to vendor") and tier-based pricing
// is used instead.
const minMeaningfulVendorPrice = 10

// basePrice returns the unrounded base price, shared by computePrice and adjustPrice.
func basePrice(item CatalogItem, snap configValues) int64 {
	// Unique/memento equipment with a known crafting cost: price as
	// schematic_equivalent + material_cost * 0.75.
	if item.MaterialCost > 0 && item.StackMax <= 1 && !item.IsSchematic &&
		(strings.ToLower(item.Rarity) == "unique" || strings.ToLower(item.Rarity) == "memento") {
		schemPrice := float64(schematicEquipmentPrice(item.Tier)) * rarityMult(item.Rarity, snap.RarityMultipliers)
		return int64(math.Round(schemPrice + float64(materialCostForGrade(item, 0))*0.75))
	}
	if item.BasePrice >= minMeaningfulVendorPrice {
		mult := vendorMult(item.Rarity, snap.VendorMultipliers)
		return int64(math.Round(float64(item.BasePrice) * mult))
	}
	// Fallback for items without a vendor price.
	mult := rarityMult(item.Rarity, snap.RarityMultipliers)
	if item.StackMax <= 1 {
		base := equipmentPrice(item.Tier)
		if item.IsSchematic {
			base = schematicEquipmentPrice(item.Tier)
		}
		return int64(math.Round(float64(base) * mult))
	}
	p := int64(math.Round(float64(materialUnitPrice(item.Tier)) * mult))
	if p < 1 {
		p = 1
	}
	return p
}

// vendorMult returns the market price multiplier for the NPC vendor base price.
// Looks up rarity (case-insensitive) in the provided multiplier map; defaults to 1.0.
func vendorMult(rarity string, mult map[string]float64) float64 {
	lower := strings.ToLower(rarity)
	for k, v := range mult {
		if strings.ToLower(k) == lower {
			return v
		}
	}
	return 1.0
}

// equipmentPrice is the per-item price for non-stackable physical gear (StackMax=1).
func equipmentPrice(tier int) int64 {
	switch tier {
	case 1:
		return 2_000
	case 2:
		return 8_000
	case 3:
		return 30_000
	case 4:
		return 100_000
	case 5:
		return 300_000
	case 6:
		return 750_000
	default: // T0 social/cosmetic gear
		return 500
	}
}

// schematicEquipmentPrice is the base listing price for a unique schematic.
// Calibrated to be competitive with (but below) Landsraad vendor prices which
// range from ~42,500–215,000 for T6 unique schematics.
func schematicEquipmentPrice(tier int) int64 {
	switch tier {
	case 1:
		return 500
	case 2:
		return 1_500
	case 3:
		return 4_000
	case 4:
		return 12_000
	case 5:
		return 30_000
	case 6:
		return 75_000
	default:
		return 500
	}
}

// materialUnitPrice is the per-unit price for stackable crafting materials.
// Full-stack values: T1≈10k, T2≈40k, T3≈100k, T4≈300k, T5≈750k, T6≈2M
func materialUnitPrice(tier int) int64 {
	switch tier {
	case 1:
		return 20
	case 2:
		return 80
	case 3:
		return 200
	case 4:
		return 600
	case 5:
		return 1_500
	case 6:
		return 4_000
	default: // T0 raw materials
		return 5
	}
}

func rarityMult(rarity string, mult map[string]float64) float64 {
	lower := strings.ToLower(rarity)
	for k, v := range mult {
		if strings.ToLower(k) == lower {
			return v
		}
	}
	return 1.0
}

func adjustPrice(item CatalogItem, currentPrice int64, soldFraction float64, snap configValues) int64 {
	floor := roundPrice(basePrice(item, snap))
	ceiling := floor * 5

	// Hard per-item overrides from item-data.json take precedence.
	if item.MinPrice > 0 && floor < item.MinPrice {
		floor = item.MinPrice
	}
	if item.MaxPrice > 0 && ceiling > item.MaxPrice {
		ceiling = item.MaxPrice
	}

	var next int64
	switch {
	case soldFraction > 0.5:
		next = int64(math.Round(float64(currentPrice) * 1.10))
	case soldFraction == 0:
		next = int64(math.Round(float64(currentPrice) * 0.95))
	default:
		next = currentPrice
	}

	next = roundPrice(next)
	if next < floor {
		next = floor
	}
	if next > ceiling {
		next = ceiling
	}
	return next
}

// gradePriceMult returns the price multiplier for quality grades 0–5 from the config array.
func gradePriceMult(grade int64, mults [6]float64) float64 {
	if grade < 0 || grade > 5 {
		return 1.0
	}
	return mults[grade]
}

// gradedPrice returns the grade-adjusted listing price, rounded to a clean step.
func gradedPrice(basePrice int64, grade int64, mults [6]float64) int64 {
	return roundPrice(int64(math.Round(float64(basePrice) * gradePriceMult(grade, mults))))
}

// materialCostForGrade returns the recipe material cost for a specific grade.
// Falls back to MaterialCost (grade-5 / last tier) when per-grade data is absent.
func materialCostForGrade(item CatalogItem, grade int64) int64 {
	if grade >= 0 && grade <= 5 && item.MaterialCostPerGrade[grade] > 0 {
		return item.MaterialCostPerGrade[grade]
	}
	return item.MaterialCost
}

// gradeFloor returns the listing price floor for item at the given grade.
// For unique/memento equipment with crafting recipes, each grade's price is derived
// from that grade's actual material cost rather than a flat multiplier.
func gradeFloor(item CatalogItem, grade int64, snap configValues) int64 {
	if item.MaterialCost > 0 && item.StackMax <= 1 && !item.IsSchematic &&
		(strings.ToLower(item.Rarity) == "unique" || strings.ToLower(item.Rarity) == "memento") {
		mc := materialCostForGrade(item, grade)
		schemPrice := float64(schematicEquipmentPrice(item.Tier)) * rarityMult(item.Rarity, snap.RarityMultipliers)
		return roundPrice(int64(math.Round(schemPrice + float64(mc)*0.75)))
	}
	return gradedPrice(roundPrice(basePrice(item, snap)), grade, snap.GradeMultipliers)
}

// roundPrice rounds to a magnitude-appropriate step so prices look clean.
func roundPrice(v int64) int64 {
	var step int64
	switch {
	case v >= 1_000_000:
		step = 100_000
	case v >= 100_000:
		step = 10_000
	case v >= 10_000:
		step = 1_000
	case v >= 1_000:
		step = 100
	default:
		step = 10
	}
	return int64(math.Round(float64(v)/float64(step))) * step
}
