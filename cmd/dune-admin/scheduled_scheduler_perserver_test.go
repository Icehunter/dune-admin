package main

import (
	"testing"
	"time"
)

// TestBackupSchedulerSingleServerFires proves the per-server scheduler loads a
// server's DB schedule, fires a due backup using that server's control plane,
// and advances that server's watermark — the single-server regression case.
func TestBackupSchedulerSingleServerFires(t *testing.T) {
	t.Setenv("DUNE_ADMIN_CONFIG_DIR", t.TempDir()) // backups land here
	store := openScheduleStore(t)

	origStore, origReg := globalStore, globalRegistry
	globalStore = store
	reg := newServerRegistry(store)
	ctrl := &dbProviderControl{}
	reg.Register(&ServerContext{ID: "1", Name: "Default", StoreScope: 1, Control: ctrl, Executor: &localExecutor{}})
	globalRegistry = reg
	t.Cleanup(func() { globalStore, globalRegistry = origStore, origReg })

	loc := time.UTC
	// All-days rule at 04:00 so the test is weekday-agnostic; "now" is 2 min past.
	cfg := scheduledBackupConfig{
		Enabled:  true,
		Timezone: "UTC",
		Rules:    []backupRule{{Days: []int{0, 1, 2, 3, 4, 5, 6}, Time: "04:00"}},
	}
	if err := saveBackupSchedule(store, 1, cfg); err != nil {
		t.Fatal(err)
	}
	at0402 := time.Date(2026, 6, 8, 4, 2, 0, 0, loc)

	backupSchedulerTickOnce(at0402)

	if !ctrl.backupCalled {
		t.Fatal("scheduled backup did not fire for the single server")
	}
	// Watermark advanced to the 04:00 occurrence, so a second tick is a no-op.
	got, _, _ := loadBackupSchedule(store, 1)
	want := time.Date(2026, 6, 8, 4, 0, 0, 0, loc).Unix()
	if got.LastFired != want {
		t.Errorf("last_fired = %d, want %d", got.LastFired, want)
	}
	ctrl.backupCalled = false
	backupSchedulerTickOnce(at0402)
	if ctrl.backupCalled {
		t.Error("backup re-fired despite watermark")
	}
}
