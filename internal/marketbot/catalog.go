package marketbot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type CatalogItem struct {
	TemplateID           string
	DisplayName          string
	StackMax             int64
	Volume               float64
	Tier                 int
	Rarity               string
	BasePrice            int64
	Category             string // e.g. "items/misc/refinedresources"
	ListPrice            int64
	IsSchematic          bool
	MaterialCost         int64
	MaterialCostPerGrade [6]int64
	IsGradeable          bool
	MinQualityLevel      int
	MinPrice             int64 // hard floor override (0 = no override)
	MaxPrice             int64 // hard ceiling override (0 = no override)
	Buyable              bool  // if false, skip buying player listings of this item
}

type itemDataEntry struct {
	Name                 string   `json:"name"`
	StackMax             int64    `json:"stack_max"`
	Volume               float64  `json:"volume"`
	Tier                 int      `json:"tier"`
	Rarity               string   `json:"rarity"`
	BasePrice            int64    `json:"vendor_price"`
	Category             string   `json:"category"`
	Tradeable            *bool    `json:"tradeable"`
	IsSchematic          bool     `json:"is_schematic"`
	MaterialCost         int64    `json:"material_cost"`
	MaterialCostPerGrade [6]int64 `json:"material_cost_per_grade"`
	IsGradeable          bool     `json:"is_gradeable"`
	MinQualityLevel      int      `json:"min_quality_level"`
	MinPrice             int64    `json:"min_price"`
	MaxPrice             int64    `json:"max_price"`
	Buyable              *bool    `json:"buyable"`
}

type itemDataFile struct {
	Items map[string]itemDataEntry `json:"items"`
}

// normalizeRarity maps a blank rarity string (JSON null → Go "") to "common"
// so config multiplier lookups always have a meaningful key to match against.
func normalizeRarity(r string) string {
	if r == "" {
		return "common"
	}
	return r
}

func loadCatalog(itemDataPath string) ([]CatalogItem, error) {
	if itemDataPath == "" {
		itemDataPath = "item-data.json"
	}
	// A configured path may point at the directory that holds item-data.json
	// (e.g. the install/working dir) rather than the file itself. Reading a
	// directory as a file fails cryptically — "is a directory" on Linux,
	// "Incorrect function" on Windows (#116) — so resolve to the file inside.
	if info, statErr := os.Stat(itemDataPath); statErr == nil && info.IsDir() {
		itemDataPath = filepath.Join(itemDataPath, "item-data.json")
	}
	dataRaw, err := os.ReadFile(itemDataPath)
	if err != nil {
		return nil, err
	}
	var dataFile itemDataFile
	if err := json.Unmarshal(dataRaw, &dataFile); err != nil {
		return nil, err
	}

	var catalog []CatalogItem
	for id, d := range dataFile.Items {
		if strings.HasPrefix(id, "Emote_") {
			continue
		}
		// Skip items explicitly excluded from the exchange.
		if d.Tradeable != nil && !*d.Tradeable {
			continue
		}
		// Skip items with no market category (e.g. reputation tokens).
		if d.Category == "" {
			continue
		}
		// Skip categories that are non-tradeable on the exchange
		// despite not having ExcludeFromExchange tags in raw data.
		if strings.HasPrefix(d.Category, "items/customization/") ||
			strings.HasPrefix(d.Category, "items/construction/") {
			continue
		}

		item := CatalogItem{
			TemplateID:           id,
			DisplayName:          d.Name,
			StackMax:             d.StackMax,
			Volume:               d.Volume,
			Tier:                 d.Tier,
			Rarity:               normalizeRarity(d.Rarity),
			BasePrice:            d.BasePrice,
			Category:             d.Category,
			IsSchematic:          d.IsSchematic,
			MaterialCost:         d.MaterialCost,
			MaterialCostPerGrade: d.MaterialCostPerGrade,
			IsGradeable:          d.IsGradeable,
			MinQualityLevel:      d.MinQualityLevel,
			MinPrice:             d.MinPrice,
			MaxPrice:             d.MaxPrice,
			Buyable:              d.Buyable == nil || *d.Buyable, // default true
		}
		item.ListPrice = computePrice(item, defaultConfig())
		catalog = append(catalog, item)
	}

	return catalog, nil
}
