package main

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func TestStatSnapshot_SchemaHasSolarisBalance(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('stat_snapshots') WHERE name='solaris_balance'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("pragma query: %v", err)
	}
	if count != 1 {
		t.Fatal("stat_snapshots missing solaris_balance column")
	}
}

func TestWriteStatSnapshot_StoresSolarisBalance(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	ctx := context.Background()

	snap := statSnapshot{
		AccountID:      42,
		SnappedAt:      "2026-01-01T12:00:00Z",
		SolarisBalance: ptr(int64(313_183_207)),
	}
	if err := writeStatSnapshot(ctx, db, snap, defaultServerID); err != nil {
		t.Fatalf("writeStatSnapshot: %v", err)
	}

	var got sql.NullInt64
	if err := db.QueryRow(
		`SELECT solaris_balance FROM stat_snapshots WHERE account_id = 42`,
	).Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !got.Valid || got.Int64 != 313_183_207 {
		t.Fatalf("want 313183207, got valid=%v val=%d", got.Valid, got.Int64)
	}
}

func TestGetStatSnapshotHistory_ReturnsSolarisBalance(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	ctx := context.Background()

	snaps := []statSnapshot{
		{AccountID: 7, SnappedAt: "2026-01-01T10:00:00Z", SolarisBalance: ptr(int64(1000))},
		{AccountID: 7, SnappedAt: "2026-01-01T10:05:00Z", SolarisBalance: ptr(int64(1500))},
		{AccountID: 7, SnappedAt: "2026-01-01T10:10:00Z", SolarisBalance: ptr(int64(1200))},
	}
	for _, s := range snaps {
		if err := writeStatSnapshot(ctx, db, s, defaultServerID); err != nil {
			t.Fatalf("writeStatSnapshot: %v", err)
		}
	}

	got, err := getStatSnapshotHistory(ctx, db, defaultServerID, 7, 500)
	if err != nil {
		t.Fatalf("getStatSnapshotHistory: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 snapshots, got %d", len(got))
	}
	wantBals := []int64{1000, 1500, 1200}
	for i, s := range got {
		if s.SolarisBalance == nil {
			t.Errorf("[%d] SolarisBalance is nil, want %d", i, wantBals[i])
			continue
		}
		if *s.SolarisBalance != wantBals[i] {
			t.Errorf("[%d] SolarisBalance: want %d, got %d", i, wantBals[i], *s.SolarisBalance)
		}
	}
}

func TestGetStatSnapshotHistory_NilSolarisBalanceWhenNotSet(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	ctx := context.Background()

	snap := statSnapshot{AccountID: 5, SnappedAt: "2026-01-01T10:00:00Z"}
	if err := writeStatSnapshot(ctx, db, snap, defaultServerID); err != nil {
		t.Fatalf("writeStatSnapshot: %v", err)
	}

	got, err := getStatSnapshotHistory(ctx, db, defaultServerID, 5, 1)
	if err != nil {
		t.Fatalf("getStatSnapshotHistory: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	if got[0].SolarisBalance != nil {
		t.Fatalf("want nil SolarisBalance, got %d", *got[0].SolarisBalance)
	}
}

// TestGetStatSnapshotHistory_ReturnsNewestOverLimit is the #294 regression
// test: snapshots accrue one row per 5 minutes online, so an account passes
// any fixed cap within days. The cap must window the NEWEST rows (in ascending
// order for the charts) — the old ASC+LIMIT query returned the oldest rows
// forever, freezing the Solari/XP graphs on the first few days of data.
func TestGetStatSnapshotHistory_ReturnsNewestOverLimit(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	ctx := context.Background()

	for i := range 10 {
		snap := statSnapshot{
			AccountID:      9,
			SnappedAt:      fmt.Sprintf("2026-01-01T10:%02d:00Z", i),
			SolarisBalance: ptr(int64(1000 + i)),
		}
		if err := writeStatSnapshot(ctx, db, snap, defaultServerID); err != nil {
			t.Fatalf("writeStatSnapshot: %v", err)
		}
	}

	got, err := getStatSnapshotHistory(ctx, db, defaultServerID, 9, 5)
	if err != nil {
		t.Fatalf("getStatSnapshotHistory: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("want 5 snapshots, got %d", len(got))
	}
	for i, s := range got {
		wantAt := fmt.Sprintf("2026-01-01T10:%02d:00Z", 5+i)
		if s.SnappedAt != wantAt {
			t.Errorf("[%d] SnappedAt = %s, want %s (newest window, ascending)", i, s.SnappedAt, wantAt)
		}
	}
}

// TestPruneStatSnapshots: rows older than the cutoff are removed for the given
// server scope only; newer rows and other servers' rows survive.
func TestPruneStatSnapshots(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	ctx := context.Background()

	rows := []struct {
		serverID  int
		snappedAt string
	}{
		{defaultServerID, "2026-01-01T10:00:00Z"}, // old — pruned
		{defaultServerID, "2026-05-01T10:00:00Z"}, // new — kept
		{2, "2026-01-01T10:00:00Z"},               // old but different server — kept
	}
	for _, r := range rows {
		snap := statSnapshot{AccountID: 4, SnappedAt: r.snappedAt}
		if err := writeStatSnapshot(ctx, db, snap, r.serverID); err != nil {
			t.Fatalf("writeStatSnapshot: %v", err)
		}
	}

	cutoff := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	pruned, err := pruneStatSnapshots(ctx, db, defaultServerID, cutoff)
	if err != nil {
		t.Fatalf("pruneStatSnapshots: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("pruned = %d, want 1", pruned)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM stat_snapshots`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("remaining rows = %d, want 2 (new row + other server)", count)
	}
}

// Note: the in-place initSessionSchema column-add migration was removed. The
// legacy 0.39.5 → unified (text/missing server_id → int-FK, plus solaris_balance)
// conversion now runs in migrateUnifiedRemodel (covered by store_migration_test.go),
// so a standalone initSessionSchema no longer mutates a pre-existing legacy table.
