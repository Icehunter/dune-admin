package main

import (
	"database/sql"
	"reflect"
	"testing"
)

// openScheduleStore opens an in-memory unified store with one parent server row.
func openScheduleStore(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openUnifiedStore(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO servers (id, name, position) VALUES (1, 'Default', 0)`); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	return db
}

func TestDaysMaskRoundTrip(t *testing.T) {
	t.Parallel()
	cases := [][]int{
		{},
		{0},
		{6},
		{0, 1, 2, 3, 4, 5, 6},
		{1, 3, 5},
		{2, 4},
	}
	for _, days := range cases {
		mask := daysToMask(days)
		got := maskToDays(mask)
		want := days
		if len(want) == 0 {
			want = nil
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("round-trip days %v -> mask %d -> %v", days, mask, got)
		}
	}
	// Out-of-range days are dropped.
	if daysToMask([]int{-1, 7, 3}) != daysToMask([]int{3}) {
		t.Error("out-of-range days not dropped")
	}
	// Duplicates collapse.
	if daysToMask([]int{2, 2, 2}) != daysToMask([]int{2}) {
		t.Error("duplicate days not collapsed")
	}
}

func TestBackupScheduleRoundTrip(t *testing.T) {
	store := openScheduleStore(t)
	if _, ok, err := loadBackupSchedule(store, 1); err != nil || ok {
		t.Fatalf("empty store: ok=%v err=%v, want false/nil", ok, err)
	}
	cfg := scheduledBackupConfig{
		Enabled:   true,
		Timezone:  "America/New_York",
		KeepN:     7,
		LastFired: 12345,
		Rules: []backupRule{
			{Days: []int{1, 3, 5}, Time: "04:00"},
			{Days: []int{0}, Time: "23:30"},
		},
	}
	if err := saveBackupSchedule(store, 1, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := loadBackupSchedule(store, 1)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, cfg)
	}
	// Save again with fewer rules — replace-all must not leave stale rows.
	cfg.Rules = []backupRule{{Days: []int{2}, Time: "06:00"}}
	if err := saveBackupSchedule(store, 1, cfg); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	got, _, _ = loadBackupSchedule(store, 1)
	if len(got.Rules) != 1 {
		t.Errorf("replace-all left %d rules, want 1", len(got.Rules))
	}
}

func TestRestartScheduleRoundTrip(t *testing.T) {
	store := openScheduleStore(t)
	cfg := scheduledRestartConfig{
		Enabled:     true,
		Timezone:    "UTC",
		WarnMinutes: 15,
		LastFired:   999,
		Rules:       []restartRule{{Days: []int{6}, Time: "03:00"}},
	}
	if err := saveRestartSchedule(store, 1, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := loadRestartSchedule(store, 1)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, cfg)
	}
}

func TestScheduleCascadeOnServerDelete(t *testing.T) {
	store := openScheduleStore(t)
	// Add a second server so we can prove the delete is scoped.
	if _, err := store.Exec(`INSERT INTO servers (id, name, position) VALUES (2, 'Other', 1)`); err != nil {
		t.Fatalf("seed server 2: %v", err)
	}
	bcfg := scheduledBackupConfig{Enabled: true, KeepN: 3, Rules: []backupRule{{Days: []int{1}, Time: "01:00"}}}
	rcfg := scheduledRestartConfig{Enabled: true, WarnMinutes: 10, Rules: []restartRule{{Days: []int{2}, Time: "02:00"}}}
	for _, id := range []int{1, 2} {
		if err := saveBackupSchedule(store, id, bcfg); err != nil {
			t.Fatal(err)
		}
		if err := saveRestartSchedule(store, id, rcfg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Exec(`DELETE FROM servers WHERE id = 1`); err != nil {
		t.Fatalf("delete server 1: %v", err)
	}
	// Server 1's schedule + rules gone.
	for _, q := range []string{
		`SELECT COUNT(*) FROM server_backup_schedule WHERE server_id = 1`,
		`SELECT COUNT(*) FROM server_backup_rule WHERE server_id = 1`,
		`SELECT COUNT(*) FROM server_restart_schedule WHERE server_id = 1`,
		`SELECT COUNT(*) FROM server_restart_rule WHERE server_id = 1`,
	} {
		var n int
		if err := store.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%q = %d, want 0 after cascade", q, n)
		}
	}
	// Server 2 untouched.
	if _, ok, _ := loadBackupSchedule(store, 2); !ok {
		t.Error("server 2 backup schedule wrongly cascaded")
	}
	if _, ok, _ := loadRestartSchedule(store, 2); !ok {
		t.Error("server 2 restart schedule wrongly cascaded")
	}
}
