package main

import (
	"encoding/json"
	"testing"
)

// TestWelcomeColumnsRoundTrip saves two packages (each with items) plus an
// ordered active-version list and verifies load reproduces the packages with
// their item details and the active-version order intact.
func TestWelcomeColumnsRoundTrip(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)

	packages := []welcomePackage{
		{Version: "v1", Items: []welcomePackageItem{
			{Template: "Item_A", Qty: 2, Quality: 0},
			{Template: "Item_B", Qty: 1, Quality: 5},
		}},
		{Version: "v2", Items: []welcomePackageItem{
			{Template: "Item_C", Qty: 10, Quality: 3},
		}},
	}
	activeVersions := []string{"v2", "v1"}

	if err := saveWelcomePackagesColumns(db, scopeA, packages, activeVersions); err != nil {
		t.Fatalf("saveWelcomePackagesColumns: %v", err)
	}

	gotPkgs, gotActive, err := loadWelcomePackagesColumns(db, scopeA)
	if err != nil {
		t.Fatalf("loadWelcomePackagesColumns: %v", err)
	}

	if len(gotPkgs) != 2 {
		t.Fatalf("packages: got %d, want 2", len(gotPkgs))
	}
	if gotPkgs[0].Version != "v1" || gotPkgs[1].Version != "v2" {
		t.Fatalf("package order wrong: %v", []string{gotPkgs[0].Version, gotPkgs[1].Version})
	}
	if len(gotPkgs[0].Items) != 2 {
		t.Fatalf("v1 items: got %d, want 2", len(gotPkgs[0].Items))
	}
	if gotPkgs[0].Items[0] != (welcomePackageItem{Template: "Item_A", Qty: 2, Quality: 0}) {
		t.Errorf("v1 item[0] = %+v", gotPkgs[0].Items[0])
	}
	if gotPkgs[0].Items[1] != (welcomePackageItem{Template: "Item_B", Qty: 1, Quality: 5}) {
		t.Errorf("v1 item[1] = %+v", gotPkgs[0].Items[1])
	}
	if len(gotPkgs[1].Items) != 1 || gotPkgs[1].Items[0] != (welcomePackageItem{Template: "Item_C", Qty: 10, Quality: 3}) {
		t.Errorf("v2 items = %+v", gotPkgs[1].Items)
	}
	if len(gotActive) != 2 || gotActive[0] != "v2" || gotActive[1] != "v1" {
		t.Fatalf("activeVersions: got %v, want [v2 v1]", gotActive)
	}
}

// TestWelcomeColumns_ServerScoped verifies rows written under one server ID are
// invisible to another.
func TestWelcomeColumns_ServerScoped(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)

	pkgsA := []welcomePackage{{Version: "a1", Items: []welcomePackageItem{{Template: "AX", Qty: 1, Quality: 0}}}}
	if err := saveWelcomePackagesColumns(db, scopeA, pkgsA, []string{"a1"}); err != nil {
		t.Fatalf("save srv-a: %v", err)
	}

	gotB, activeB, err := loadWelcomePackagesColumns(db, scopeB)
	if err != nil {
		t.Fatalf("load srv-b: %v", err)
	}
	if len(gotB) != 0 || len(activeB) != 0 {
		t.Fatalf("srv-b should see nothing, got %d pkgs, %d active", len(gotB), len(activeB))
	}

	gotA, activeA, err := loadWelcomePackagesColumns(db, scopeA)
	if err != nil {
		t.Fatalf("load srv-a: %v", err)
	}
	if len(gotA) != 1 || gotA[0].Version != "a1" || len(gotA[0].Items) != 1 {
		t.Fatalf("srv-a packages = %+v", gotA)
	}
	if len(activeA) != 1 || activeA[0] != "a1" {
		t.Fatalf("srv-a active = %v", activeA)
	}
}

// TestWelcomeStore_PackagesRoundTripThroughAPI verifies the store's public
// saveConfig/loadConfig API still round-trips packages now that they are stored
// in typed columns rather than the packages_json blob.
func TestWelcomeStore_PackagesRoundTripThroughAPI(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)
	s := newWelcomeStore(db, scopeA)

	packages := []welcomePackage{
		{Version: "v1", Items: []welcomePackageItem{{Template: "Item_A", Qty: 2, Quality: 1}}},
	}
	packagesJSON, err := json.Marshal(packages)
	if err != nil {
		t.Fatalf("marshal packages: %v", err)
	}

	if err := s.saveConfig(welcomeConfigRow{
		PackagesJSON:   string(packagesJSON),
		ActiveVersions: []string{"v1"},
		ScanSecs:       30,
	}); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	got, ok, err := s.loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !ok {
		t.Fatal("expected config present after save")
	}

	var gotPkgs []welcomePackage
	if err := json.Unmarshal([]byte(got.PackagesJSON), &gotPkgs); err != nil {
		t.Fatalf("unmarshal got.PackagesJSON %q: %v", got.PackagesJSON, err)
	}
	if len(gotPkgs) != 1 || gotPkgs[0].Version != "v1" {
		t.Fatalf("round-tripped packages = %+v", gotPkgs)
	}
	if len(gotPkgs[0].Items) != 1 || gotPkgs[0].Items[0] != (welcomePackageItem{Template: "Item_A", Qty: 2, Quality: 1}) {
		t.Fatalf("round-tripped items = %+v", gotPkgs[0].Items)
	}
	if len(got.ActiveVersions) != 1 || got.ActiveVersions[0] != "v1" {
		t.Fatalf("ActiveVersions = %v", got.ActiveVersions)
	}
}
