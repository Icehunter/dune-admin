package marketbot

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// configValues holds all runtime-tunable parameters.
// It is safe to copy (no pointers to shared state).
type configValues struct {
	BuyInterval       time.Duration      `json:"buy_interval"`
	ListInterval      time.Duration      `json:"list_interval"`
	BuyThreshold      float64            `json:"buy_threshold"`
	MaxBuys           int                `json:"max_buys"`
	ListingsPerGrade  int                `json:"listings_per_grade"`
	Enabled           bool               `json:"enabled"`
	GradeMultipliers  [6]float64         `json:"grade_multipliers"`
	RarityMultipliers map[string]float64 `json:"rarity_multipliers"`
	VendorMultipliers map[string]float64 `json:"vendor_multipliers"`
	DisabledItems     []string           `json:"disabled_items"`
}

// Config is the thread-safe runtime config. Tickers call Snapshot() at tick start.
type Config struct {
	mu     sync.RWMutex
	config configValues
}

func defaultConfig() configValues {
	return configValues{
		BuyInterval:      5 * time.Minute,
		ListInterval:     30 * time.Minute,
		BuyThreshold:     1.05,
		MaxBuys:          50,
		ListingsPerGrade: 5,
		Enabled:          true,
		GradeMultipliers: [6]float64{1.0, 1.0, 1.25, 1.5, 1.75, 2.0},
		RarityMultipliers: map[string]float64{
			"common":  1.0,
			"unique":  3.0,
			"memento": 5.0,
		},
		VendorMultipliers: map[string]float64{
			"common":  3.0,
			"unique":  5.0,
			"memento": 5.0,
		},
		DisabledItems: nil,
	}
}

// Snapshot returns a copy of the current config values under read lock.
func (c *Config) Snapshot() configValues {
	c.mu.RLock()
	defer c.mu.RUnlock()
	snap := c.config
	// Deep copy slices/maps to prevent races.
	snap.DisabledItems = append([]string(nil), c.config.DisabledItems...)
	snap.RarityMultipliers = make(map[string]float64, len(c.config.RarityMultipliers))
	for k, v := range c.config.RarityMultipliers {
		snap.RarityMultipliers[k] = v
	}
	snap.VendorMultipliers = make(map[string]float64, len(c.config.VendorMultipliers))
	for k, v := range c.config.VendorMultipliers {
		snap.VendorMultipliers[k] = v
	}
	return snap
}

// configJSON is the JSON wire format — durations as strings for round-trip compatibility with Apply.
type configJSON struct {
	BuyInterval       string             `json:"buy_interval"`
	ListInterval      string             `json:"list_interval"`
	BuyThreshold      float64            `json:"buy_threshold"`
	MaxBuys           int                `json:"max_buys"`
	ListingsPerGrade  int                `json:"listings_per_grade"`
	Enabled           bool               `json:"enabled"`
	GradeMultipliers  [6]float64         `json:"grade_multipliers"`
	RarityMultipliers map[string]float64 `json:"rarity_multipliers"`
	VendorMultipliers map[string]float64 `json:"vendor_multipliers"`
	DisabledItems     []string           `json:"disabled_items"`
}

// MarshalJSON returns the config as JSON with durations as strings (e.g. "5m0s"),
// making the output round-trip compatible with Apply.
func (c *Config) MarshalJSON() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return json.Marshal(configJSON{
		BuyInterval:       c.config.BuyInterval.String(),
		ListInterval:      c.config.ListInterval.String(),
		BuyThreshold:      c.config.BuyThreshold,
		MaxBuys:           c.config.MaxBuys,
		ListingsPerGrade:  c.config.ListingsPerGrade,
		Enabled:           c.config.Enabled,
		GradeMultipliers:  c.config.GradeMultipliers,
		RarityMultipliers: c.config.RarityMultipliers,
		VendorMultipliers: c.config.VendorMultipliers,
		DisabledItems:     c.config.DisabledItems,
	})
}

// Apply updates config fields from a partial JSON patch map.
// Only listed keys are changed; unknown keys are ignored.
// Returns an error if validation fails; no partial updates are applied.
func (c *Config) Apply(patch map[string]json.RawMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Work on a copy; only commit if all validation passes.
	next := c.config
	next.DisabledItems = append([]string(nil), c.config.DisabledItems...)
	next.RarityMultipliers = make(map[string]float64, len(c.config.RarityMultipliers))
	for k, v := range c.config.RarityMultipliers {
		next.RarityMultipliers[k] = v
	}
	next.VendorMultipliers = make(map[string]float64, len(c.config.VendorMultipliers))
	for k, v := range c.config.VendorMultipliers {
		next.VendorMultipliers[k] = v
	}

	for key, raw := range patch {
		switch key {
		case "buy_interval":
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				return fmt.Errorf("buy_interval: %w", err)
			}
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("buy_interval: %w", err)
			}
			if d < time.Minute {
				return fmt.Errorf("buy_interval: minimum 1m")
			}
			next.BuyInterval = d
		case "list_interval":
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				return fmt.Errorf("list_interval: %w", err)
			}
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("list_interval: %w", err)
			}
			if d < time.Minute {
				return fmt.Errorf("list_interval: minimum 1m")
			}
			next.ListInterval = d
		case "buy_threshold":
			if err := json.Unmarshal(raw, &next.BuyThreshold); err != nil {
				return fmt.Errorf("buy_threshold: %w", err)
			}
			if next.BuyThreshold < 0 {
				return fmt.Errorf("buy_threshold: must be >= 0")
			}
		case "max_buys":
			if err := json.Unmarshal(raw, &next.MaxBuys); err != nil {
				return fmt.Errorf("max_buys: %w", err)
			}
			if next.MaxBuys < 0 {
				return fmt.Errorf("max_buys: must be >= 0")
			}
		case "listings_per_grade":
			if err := json.Unmarshal(raw, &next.ListingsPerGrade); err != nil {
				return fmt.Errorf("listings_per_grade: %w", err)
			}
			if next.ListingsPerGrade < 1 {
				return fmt.Errorf("listings_per_grade: must be >= 1")
			}
		case "enabled":
			if err := json.Unmarshal(raw, &next.Enabled); err != nil {
				return fmt.Errorf("enabled: %w", err)
			}
		case "grade_multipliers":
			var tmp [6]float64
			if err := json.Unmarshal(raw, &tmp); err != nil {
				return fmt.Errorf("grade_multipliers: %w", err)
			}
			for i, v := range tmp {
				if v <= 0 {
					return fmt.Errorf("grade_multipliers[%d]: must be > 0", i)
				}
			}
			next.GradeMultipliers = tmp
		case "rarity_multipliers":
			var tmp map[string]float64
			if err := json.Unmarshal(raw, &tmp); err != nil {
				return fmt.Errorf("rarity_multipliers: %w", err)
			}
			for k, v := range tmp {
				if v <= 0 {
					return fmt.Errorf("rarity_multipliers[%q]: must be > 0", k)
				}
			}
			next.RarityMultipliers = tmp
		case "vendor_multipliers":
			var tmp map[string]float64
			if err := json.Unmarshal(raw, &tmp); err != nil {
				return fmt.Errorf("vendor_multipliers: %w", err)
			}
			for k, v := range tmp {
				if v <= 0 {
					return fmt.Errorf("vendor_multipliers[%q]: must be > 0", k)
				}
			}
			next.VendorMultipliers = tmp
		case "disabled_items":
			if err := json.Unmarshal(raw, &next.DisabledItems); err != nil {
				return fmt.Errorf("disabled_items: %w", err)
			}
		}
	}

	c.config = next
	return nil
}
