// config_test.go
package marketbot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigDefaults(t *testing.T) {
	cfg := defaultConfig()
	if cfg.ListInterval != 30*time.Minute {
		t.Errorf("ListInterval want 30m got %v", cfg.ListInterval)
	}
	if cfg.BuyInterval != 5*time.Minute {
		t.Errorf("BuyInterval want 5m got %v", cfg.BuyInterval)
	}
	if cfg.ListingsPerGrade != 5 {
		t.Errorf("ListingsPerGrade want 5 got %d", cfg.ListingsPerGrade)
	}
	if cfg.BuyThreshold != 1.05 {
		t.Errorf("BuyThreshold want 1.05 got %f", cfg.BuyThreshold)
	}
	if cfg.MaxBuys != 50 {
		t.Errorf("MaxBuys want 50 got %d", cfg.MaxBuys)
	}
	if !cfg.Enabled {
		t.Error("Enabled should default to true")
	}
	want := [6]float64{1.0, 1.0, 1.25, 1.5, 1.75, 2.0}
	if cfg.GradeMultipliers != want {
		t.Errorf("GradeMultipliers want %v got %v", want, cfg.GradeMultipliers)
	}
}

func TestConfigSnapshot(t *testing.T) {
	c := &Config{}
	c.config = defaultConfig()
	snap := c.Snapshot()
	snap.MaxBuys = 999
	if c.config.MaxBuys == 999 {
		t.Error("Snapshot should be a copy, not a reference")
	}

	// Test map isolation
	snap2 := c.Snapshot()
	snap2.RarityMultipliers["common"] = 99.0
	if c.Snapshot().RarityMultipliers["common"] == 99.0 {
		t.Error("Snapshot RarityMultipliers should be an independent copy")
	}

	// Test slice isolation
	snap3 := c.Snapshot()
	snap3.DisabledItems = append(snap3.DisabledItems, "extra")
	if len(c.Snapshot().DisabledItems) != 0 {
		t.Error("Snapshot DisabledItems should be an independent copy")
	}
}

func TestConfigUpdate(t *testing.T) {
	c := &Config{}
	c.config = defaultConfig()

	patch := map[string]json.RawMessage{}
	b, _ := json.Marshal(25)
	patch["max_buys"] = b
	if err := c.Apply(patch); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if c.Snapshot().MaxBuys != 25 {
		t.Error("MaxBuys not updated")
	}
}

func TestConfigValidation(t *testing.T) {
	c := &Config{}
	c.config = defaultConfig()

	patch := map[string]json.RawMessage{}
	b, _ := json.Marshal(-1)
	patch["max_buys"] = b
	if err := c.Apply(patch); err == nil {
		t.Error("expected error for negative MaxBuys")
	}
}

func TestConfigMultiplierValidation(t *testing.T) {
	c := &Config{}
	c.config = defaultConfig()

	// Zero grade multiplier should be rejected
	patch := map[string]json.RawMessage{}
	b, _ := json.Marshal([6]float64{1.0, 0.0, 1.25, 1.5, 1.75, 2.0})
	patch["grade_multipliers"] = b
	if err := c.Apply(patch); err == nil {
		t.Error("expected error for zero grade multiplier")
	}

	// Negative rarity multiplier should be rejected
	patch2 := map[string]json.RawMessage{}
	b2, _ := json.Marshal(map[string]float64{"common": -1.0})
	patch2["rarity_multipliers"] = b2
	if err := c.Apply(patch2); err == nil {
		t.Error("expected error for negative rarity multiplier")
	}
}

func TestConfigValidationAtomicity(t *testing.T) {
	c := &Config{}
	c.config = defaultConfig()

	patch := map[string]json.RawMessage{}
	validVal, _ := json.Marshal(25)
	patch["max_buys"] = validVal
	invalidVal, _ := json.Marshal(-1)
	patch["buy_threshold"] = invalidVal // invalid: negative

	err := c.Apply(patch)
	if err == nil {
		t.Error("expected error for invalid buy_threshold")
	}
	// max_buys should NOT have been updated (atomicity)
	if c.Snapshot().MaxBuys != 50 {
		t.Errorf("Apply should be atomic: MaxBuys should still be 50, got %d", c.Snapshot().MaxBuys)
	}
}

func TestConfigOnChangeFiresAfterApply(t *testing.T) {
	c := &Config{}
	c.config = defaultConfig()

	var called int
	var lastSeen configValues
	c.OnChange(func(v configValues) {
		called++
		lastSeen = v
	})

	patch := map[string]json.RawMessage{"max_buys": json.RawMessage("17")}
	if err := c.Apply(patch); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if called != 1 {
		t.Errorf("onChange should fire once on successful Apply, got %d", called)
	}
	if lastSeen.MaxBuys != 17 {
		t.Errorf("onChange got MaxBuys=%d want 17", lastSeen.MaxBuys)
	}

	// Failed validation must NOT fire the callback.
	bad := map[string]json.RawMessage{"max_buys": json.RawMessage("-5")}
	if err := c.Apply(bad); err == nil {
		t.Fatal("expected Apply to fail")
	}
	if called != 1 {
		t.Errorf("onChange should not fire on validation failure, got %d", called)
	}
}

func TestSaveAndLoadStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "market-bot-state.json")

	want := defaultConfig()
	want.MaxBuys = 123
	want.BuyThreshold = 0.42
	want.Enabled = false
	want.DisabledItems = []string{"item.a", "item.b"}
	want.RarityMultipliers = map[string]float64{"common": 2.5, "unique": 4.0}
	want.GradeMultipliers = [6]float64{1, 2, 3, 4, 5, 6}

	if err := SaveState(path, want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got.MaxBuys != want.MaxBuys || got.BuyThreshold != want.BuyThreshold ||
		got.Enabled != want.Enabled || got.GradeMultipliers != want.GradeMultipliers {
		t.Errorf("round-trip mismatch:\n want=%+v\n got=%+v", want, got)
	}
	if len(got.DisabledItems) != len(want.DisabledItems) {
		t.Errorf("DisabledItems len: got %d want %d", len(got.DisabledItems), len(want.DisabledItems))
	}
	if got.RarityMultipliers["common"] != 2.5 {
		t.Errorf("RarityMultipliers not restored: %v", got.RarityMultipliers)
	}
	if got.BuyInterval != want.BuyInterval {
		t.Errorf("BuyInterval: got %v want %v", got.BuyInterval, want.BuyInterval)
	}
}

func TestLoadStateMissingFile(t *testing.T) {
	got, err := LoadState(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("LoadState on missing file should return zero+nil err, got %v", err)
	}
	// Caller uses (zero == empty) as a "no state, use defaults" signal.
	if got.MaxBuys != 0 || got.Enabled || len(got.DisabledItems) != 0 {
		t.Errorf("missing-file load should return zero value, got %+v", got)
	}
}

func TestSaveStateAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "market-bot-state.json")

	first := defaultConfig()
	first.MaxBuys = 1
	if err := SaveState(path, first); err != nil {
		t.Fatalf("SaveState first: %v", err)
	}

	second := defaultConfig()
	second.MaxBuys = 2
	if err := SaveState(path, second); err != nil {
		t.Fatalf("SaveState second: %v", err)
	}

	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got.MaxBuys != 2 {
		t.Errorf("second save not visible: MaxBuys=%d", got.MaxBuys)
	}

	// No leftover tmp files.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("atomic write left stale tmp file: %s", e.Name())
		}
	}
}
