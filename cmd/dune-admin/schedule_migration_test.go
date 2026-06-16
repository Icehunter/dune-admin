package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// withLegacySchedulePaths redirects the legacy JSON file paths into dir for the
// test's lifetime.
func withLegacySchedulePaths(t *testing.T, dir string) (backupPath, restartPath string) {
	t.Helper()
	backupPath = filepath.Join(dir, "scheduled-backups.json")
	restartPath = filepath.Join(dir, "scheduled-restarts.json")
	oldB, oldR := backupCfgPath, restartCfgPath
	backupCfgPath, restartCfgPath = backupPath, restartPath
	t.Cleanup(func() { backupCfgPath, restartCfgPath = oldB, oldR })
	return backupPath, restartPath
}

func TestMigrateLegacyBackupSchedule(t *testing.T) {
	store := openScheduleStore(t)
	dir := t.TempDir()
	backupPath, _ := withLegacySchedulePaths(t, dir)

	// 0=Sun..6=Sat encoded directly in the legacy JSON.
	json := `{"enabled":true,"timezone":"UTC","keep_n":5,"last_fired":4242,` +
		`"rules":[{"days":[1,3,5],"time":"04:00"}]}`
	if err := os.WriteFile(backupPath, []byte(json), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := migrateLegacyBackupSchedule(store, 1); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	got, ok, err := loadBackupSchedule(store, 1)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	want := scheduledBackupConfig{
		Enabled: true, Timezone: "UTC", KeepN: 5, LastFired: 4242,
		Rules: []backupRule{{Days: []int{1, 3, 5}, Time: "04:00"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("migrated cfg:\n got %+v\nwant %+v", got, want)
	}
	// File must be left untouched.
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("legacy file removed: %v", err)
	}
	// Idempotent: second run is a no-op (marker set).
	if err := migrateLegacyBackupSchedule(store, 1); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestMigrateLegacyRestartSchedule(t *testing.T) {
	store := openScheduleStore(t)
	dir := t.TempDir()
	_, restartPath := withLegacySchedulePaths(t, dir)

	json := `{"enabled":true,"timezone":"America/New_York","warn_minutes":20,"last_fired":77,` +
		`"rules":[{"days":[6],"time":"03:30"}]}`
	if err := os.WriteFile(restartPath, []byte(json), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyRestartSchedule(store, 1); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	got, ok, err := loadRestartSchedule(store, 1)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	want := scheduledRestartConfig{
		Enabled: true, Timezone: "America/New_York", WarnMinutes: 20, LastFired: 77,
		Rules: []restartRule{{Days: []int{6}, Time: "03:30"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("migrated cfg:\n got %+v\nwant %+v", got, want)
	}
	if _, err := os.Stat(restartPath); err != nil {
		t.Errorf("legacy file removed: %v", err)
	}
}

func TestMigrateLegacyScheduleNoFile(t *testing.T) {
	store := openScheduleStore(t)
	withLegacySchedulePaths(t, t.TempDir()) // empty dir → no files
	if err := migrateLegacyBackupSchedule(store, 1); err != nil {
		t.Fatalf("backup migrate: %v", err)
	}
	if err := migrateLegacyRestartSchedule(store, 1); err != nil {
		t.Fatalf("restart migrate: %v", err)
	}
	if _, ok, _ := loadBackupSchedule(store, 1); ok {
		t.Error("backup schedule created from nothing")
	}
	if _, ok, _ := loadRestartSchedule(store, 1); ok {
		t.Error("restart schedule created from nothing")
	}
}

func TestMigrateLegacyScheduleSkipsWhenPopulated(t *testing.T) {
	store := openScheduleStore(t)
	dir := t.TempDir()
	backupPath, _ := withLegacySchedulePaths(t, dir)
	// Pre-existing DB row.
	existing := scheduledBackupConfig{Enabled: true, KeepN: 9, Rules: []backupRule{{Days: []int{0}, Time: "12:00"}}}
	if err := saveBackupSchedule(store, 1, existing); err != nil {
		t.Fatal(err)
	}
	// A legacy file that must be ignored.
	if err := os.WriteFile(backupPath, []byte(`{"enabled":false,"keep_n":1,"rules":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyBackupSchedule(store, 1); err != nil {
		t.Fatal(err)
	}
	got, _, _ := loadBackupSchedule(store, 1)
	if got.KeepN != 9 {
		t.Errorf("existing DB row clobbered by migration: %+v", got)
	}
}
