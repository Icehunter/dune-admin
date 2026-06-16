package main

import (
	"encoding/json"
	"testing"
)

// openMemGivePacksStore opens an in-memory give-packs store for testing.
func openMemGivePacksStore(t *testing.T) *givePacksStore {
	t.Helper()
	s, err := openGivePacksStore(":memory:")
	if err != nil {
		t.Fatalf("openGivePacksStore: %v", err)
	}
	t.Cleanup(func() { _ = s.close() })
	return s
}

func TestGivePacksStore_LoadMissingReturnsNotOK(t *testing.T) {
	t.Parallel()
	s := openMemGivePacksStore(t)

	loaded, packsJSON, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false on empty store, got true")
	}
	if loaded {
		t.Error("expected base_packs_loaded=false on empty store")
	}
	if packsJSON != "" {
		t.Errorf("expected empty packsJSON, got %q", packsJSON)
	}
}

func TestGivePacksStore_SaveAndLoad(t *testing.T) {
	t.Parallel()
	s := openMemGivePacksStore(t)

	const testJSON = `[{"id":"starter-t1","name":"T1","category":"Starter","tier":1,"items":[]}]`
	if err := s.saveConfig(testJSON, true); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	loaded, packsJSON, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true after save")
	}
	if !loaded {
		t.Error("expected base_packs_loaded=true after saveConfig(..., true)")
	}
	if packsJSON != testJSON {
		t.Errorf("packsJSON mismatch:\nwant: %s\ngot:  %s", testJSON, packsJSON)
	}
}

func TestGivePacksStore_SavedUnloaded(t *testing.T) {
	t.Parallel()
	s := openMemGivePacksStore(t)

	if err := s.saveConfig(`[]`, false); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	loaded, _, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if loaded {
		t.Error("expected base_packs_loaded=false when saved with false")
	}
}

func TestGivePacksStore_OverwriteWithSave(t *testing.T) {
	t.Parallel()
	s := openMemGivePacksStore(t)

	if err := s.saveConfig(`[{"id":"a"}]`, false); err != nil {
		t.Fatalf("first save: %v", err)
	}
	const second = `[{"id":"b"},{"id":"c"}]`
	if err := s.saveConfig(second, true); err != nil {
		t.Fatalf("second save: %v", err)
	}

	loaded, packsJSON, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !loaded {
		t.Error("expected base_packs_loaded=true after second save")
	}
	// loadConfig now re-marshals fully-populated structs from the typed tables,
	// so byte equality against the sparse input no longer holds; assert the
	// second save fully replaced the first and preserved order (b, then c).
	var gotPacks []givePack
	if err := json.Unmarshal([]byte(packsJSON), &gotPacks); err != nil {
		t.Fatalf("unmarshal packsJSON %q: %v", packsJSON, err)
	}
	if len(gotPacks) != 2 || gotPacks[0].ID != "b" || gotPacks[1].ID != "c" {
		t.Errorf("expected packs [b c], got %q", packsJSON)
	}
}

func TestGivePacksStore_EmptyPacksRoundTrip(t *testing.T) {
	t.Parallel()
	s := openMemGivePacksStore(t)

	// Saving empty packs with loaded=true (user deleted all) must not re-seed.
	if err := s.saveConfig(`[]`, true); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	loaded, packsJSON, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !loaded {
		t.Error("base_packs_loaded should remain true even when packs are empty")
	}
	if packsJSON != `[]` {
		t.Errorf("expected empty array, got %q", packsJSON)
	}
}
