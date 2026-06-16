package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func openPermStore(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openUnifiedStore(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestPermissionMatrixDBRoundTrip(t *testing.T) {
	s := openPermStore(t)
	if _, ok, err := loadPermissionMatrix(s); err != nil || ok {
		t.Fatalf("empty store: ok=%v err=%v, want false/nil", ok, err)
	}
	matrix := map[string][]string{
		"default":   {"players:read", "world:read"},
		"123456789": {"server:control"},
	}
	if err := savePermissionMatrix(s, matrix); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := loadPermissionMatrix(s)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	for k := range got {
		sort.Strings(got[k])
	}
	for k := range matrix {
		sort.Strings(matrix[k])
	}
	if !reflect.DeepEqual(got, matrix) {
		t.Errorf("round-trip:\n got %v\nwant %v", got, matrix)
	}
	// Replace-all: a smaller matrix must not leave stale rows.
	if err := savePermissionMatrix(s, map[string][]string{"x": {"logs:read"}}); err != nil {
		t.Fatal(err)
	}
	got, _, _ = loadPermissionMatrix(s)
	if len(got) != 1 || len(got["x"]) != 1 {
		t.Errorf("replace-all left stale rows: %v", got)
	}
}

func TestInitPermissionsMatrixSeedsDefaultInDB(t *testing.T) {
	s := openPermStore(t)
	withPermissionsStore(t, s)
	old := snapshotPermissionsMatrix()
	t.Cleanup(func() { setPermissionsMatrix(old) })

	initPermissionsMatrix()

	// Seeded into the DB.
	matrix, ok, err := loadPermissionMatrix(s)
	if err != nil || !ok {
		t.Fatalf("seed not persisted: ok=%v err=%v", ok, err)
	}
	got := matrix[pseudoRoleDefault]
	want := defaultSeedCaps()
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("default seed:\n got %v\nwant %v", got, want)
	}
	// Cached in memory too.
	if caps := capsForRoles(nil); !caps[capPlayersRead] {
		t.Error("seeded default row not applied to cascade")
	}
}

func TestMigrateLegacyPermissions(t *testing.T) {
	s := openPermStore(t)
	tmp := filepath.Join(t.TempDir(), "permissions.yaml")
	withPermissionsPath(t, tmp)

	yaml := "default:\n  - players:read\nmod-role:\n  - players:write\n  - logs:read\n"
	if err := os.WriteFile(tmp, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyPermissions(s); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	got, ok, err := loadPermissionMatrix(s)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if len(got["mod-role"]) != 2 || len(got["default"]) != 1 {
		t.Errorf("migrated matrix = %v", got)
	}
	// File left on disk.
	if _, err := os.Stat(tmp); err != nil {
		t.Errorf("permissions.yaml removed: %v", err)
	}
	// Idempotent.
	if err := migrateLegacyPermissions(s); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}
