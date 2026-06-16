package main

import (
	"encoding/json"
	"testing"
)

// samplePacks returns two packs with items spanning varied tier/qty/quality.
func samplePacks() []givePack {
	return []givePack{
		{
			ID: "starter-t1", Name: "T1", Category: "Starter", Tier: 1,
			Items: []welcomePackageItem{
				{Template: "Boots", Qty: 1, Quality: 0},
				{Template: "Ammo", Qty: 500, Quality: 3},
			},
		},
		{
			ID: "buggy-t6", Name: "T6", Category: "Buggy", Tier: 6,
			Items: []welcomePackageItem{
				{Template: "BuggyChassis_6", Qty: 1, Quality: 5},
			},
		},
	}
}

func TestGivePacksColumnsRoundTrip(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)

	want := samplePacks()
	if err := saveGivePacksColumns(db, scopeA, want); err != nil {
		t.Fatalf("saveGivePacksColumns: %v", err)
	}

	got, err := loadGivePacksColumns(db, scopeA)
	if err != nil {
		t.Fatalf("loadGivePacksColumns: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("pack count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID || got[i].Name != want[i].Name ||
			got[i].Category != want[i].Category || got[i].Tier != want[i].Tier {
			t.Fatalf("pack[%d] = %+v, want %+v", i, got[i], want[i])
		}
		if len(got[i].Items) != len(want[i].Items) {
			t.Fatalf("pack[%d] item count = %d, want %d", i, len(got[i].Items), len(want[i].Items))
		}
		for j := range want[i].Items {
			if got[i].Items[j] != want[i].Items[j] {
				t.Fatalf("pack[%d].item[%d] = %+v, want %+v", i, j, got[i].Items[j], want[i].Items[j])
			}
		}
	}
}

func TestGivePacksColumns_ServerScoped(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)

	packsA := []givePack{{ID: "a-pack", Name: "A", Category: "CatA", Tier: 2,
		Items: []welcomePackageItem{{Template: "ItemA", Qty: 1, Quality: 0}}}}
	packsB := []givePack{{ID: "b-pack", Name: "B", Category: "CatB", Tier: 4,
		Items: []welcomePackageItem{{Template: "ItemB", Qty: 2, Quality: 1}}}}

	if err := saveGivePacksColumns(db, scopeA, packsA); err != nil {
		t.Fatalf("save srv-a: %v", err)
	}
	if err := saveGivePacksColumns(db, scopeB, packsB); err != nil {
		t.Fatalf("save srv-b: %v", err)
	}

	gotA, err := loadGivePacksColumns(db, scopeA)
	if err != nil {
		t.Fatalf("load srv-a: %v", err)
	}
	gotB, err := loadGivePacksColumns(db, scopeB)
	if err != nil {
		t.Fatalf("load srv-b: %v", err)
	}
	if len(gotA) != 1 || gotA[0].ID != "a-pack" {
		t.Fatalf("srv-a = %+v, want only a-pack", gotA)
	}
	if len(gotB) != 1 || gotB[0].ID != "b-pack" {
		t.Fatalf("srv-b = %+v, want only b-pack", gotB)
	}
}

func TestGivePacksStore_SaveLoadRoundTrip(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)
	store := newGivePacksStore(db, scopeA)

	want := samplePacks()
	blob, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal packs: %v", err)
	}
	if err := store.saveConfig(string(blob), true); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	basePacksLoaded, packsJSON, ok, err := store.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !basePacksLoaded {
		t.Fatalf("basePacksLoaded = false, want true")
	}
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	var got []givePack
	if err := json.Unmarshal([]byte(packsJSON), &got); err != nil {
		t.Fatalf("unmarshal returned packsJSON %q: %v", packsJSON, err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d packs, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID || len(got[i].Items) != len(want[i].Items) {
			t.Fatalf("pack[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
