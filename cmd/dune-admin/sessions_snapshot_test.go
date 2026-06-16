package main

import (
	"context"
	"database/sql"
	"testing"
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

// Note: the in-place initSessionSchema column-add migration was removed. The
// legacy 0.39.5 → unified (text/missing server_id → int-FK, plus solaris_balance)
// conversion now runs in migrateUnifiedRemodel (covered by store_migration_test.go),
// so a standalone initSessionSchema no longer mutates a pre-existing legacy table.
