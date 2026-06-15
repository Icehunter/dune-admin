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

	if err := saveWelcomePackagesColumns(db, "default", packages, activeVersions); err != nil {
		t.Fatalf("saveWelcomePackagesColumns: %v", err)
	}

	gotPkgs, gotActive, err := loadWelcomePackagesColumns(db, "default")
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
	if err := saveWelcomePackagesColumns(db, "srv-a", pkgsA, []string{"a1"}); err != nil {
		t.Fatalf("save srv-a: %v", err)
	}

	gotB, activeB, err := loadWelcomePackagesColumns(db, "srv-b")
	if err != nil {
		t.Fatalf("load srv-b: %v", err)
	}
	if len(gotB) != 0 || len(activeB) != 0 {
		t.Fatalf("srv-b should see nothing, got %d pkgs, %d active", len(gotB), len(activeB))
	}

	gotA, activeA, err := loadWelcomePackagesColumns(db, "srv-a")
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

// TestMigrateWelcomeColumns seeds a legacy welcome_config row with JSON blobs and
// verifies the migration decomposes them into the typed child tables once.
func TestMigrateWelcomeColumns(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)

	const packagesJSON = `[{"version":"v1","items":[{"template":"Tmpl_X","qty":3,"quality":2}]}]`
	const activeVersionsJSON = `["v1"]`
	if _, err := db.Exec(`INSERT INTO welcome_config
		(server_id, enabled, scan_secs, active_version, active_versions_json, packages_json, updated_at)
		VALUES ('default', 1, 30, 'v1', ?, ?, '')`, activeVersionsJSON, packagesJSON); err != nil {
		t.Fatalf("seed welcome_config: %v", err)
	}

	if err := migrateWelcomeColumns(db); err != nil {
		t.Fatalf("migrateWelcomeColumns: %v", err)
	}

	pkgs, active, err := loadWelcomePackagesColumns(db, "default")
	if err != nil {
		t.Fatalf("loadWelcomePackagesColumns: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].Version != "v1" {
		t.Fatalf("migrated packages = %+v", pkgs)
	}
	if len(pkgs[0].Items) != 1 || pkgs[0].Items[0] != (welcomePackageItem{Template: "Tmpl_X", Qty: 3, Quality: 2}) {
		t.Fatalf("migrated items = %+v", pkgs[0].Items)
	}
	if len(active) != 1 || active[0] != "v1" {
		t.Fatalf("migrated active = %v", active)
	}

	marker, err := metaGet(db, "migrated:welcome_columns")
	if err != nil {
		t.Fatalf("metaGet: %v", err)
	}
	if marker == "" {
		t.Error("migration marker migrated:welcome_columns not set")
	}
}

// TestWelcomeStore_PackagesRoundTripThroughAPI verifies the store's public
// saveConfig/loadConfig API still round-trips packages now that they are stored
// in typed columns rather than the packages_json blob.
func TestWelcomeStore_PackagesRoundTripThroughAPI(t *testing.T) {
	t.Parallel()
	db := openSharedScopeDB(t)
	s := newWelcomeStore(db, "default")

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
