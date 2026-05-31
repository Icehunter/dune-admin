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
	if err := writeStatSnapshot(ctx, db, snap); err != nil {
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
		if err := writeStatSnapshot(ctx, db, s); err != nil {
			t.Fatalf("writeStatSnapshot: %v", err)
		}
	}

	got, err := getStatSnapshotHistory(ctx, db, 7, 500)
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
	if err := writeStatSnapshot(ctx, db, snap); err != nil {
		t.Fatalf("writeStatSnapshot: %v", err)
	}

	got, err := getStatSnapshotHistory(ctx, db, 5, 1)
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

func TestOpenSessionDB_MigratesExistingDBToAddSolarisBalance(t *testing.T) {
	t.Parallel()
	// Simulate a pre-existing DB without the solaris_balance column.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Create schema as it existed before the solaris column was added.
	if _, err := db.Exec(`
		CREATE TABLE stat_snapshots (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id     INTEGER NOT NULL,
			snapped_at     TEXT    NOT NULL,
			char_xp        INTEGER,
			skill_points   INTEGER,
			intel_points   INTEGER,
			combat_xp      INTEGER,
			crafting_xp    INTEGER,
			gathering_xp   INTEGER,
			exploration_xp INTEGER,
			sabotage_xp    INTEGER
		);
		CREATE TABLE play_sessions (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id    INTEGER NOT NULL,
			started_at    TEXT    NOT NULL,
			ended_at      TEXT,
			duration_secs INTEGER
		);
	`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	// Running initSessionSchema should add the missing column without error.
	if err := initSessionSchema(db); err != nil {
		t.Fatalf("initSessionSchema on existing db: %v", err)
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('stat_snapshots') WHERE name='solaris_balance'`,
	).Scan(&count); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if count != 1 {
		t.Fatal("migration did not add solaris_balance to existing stat_snapshots table")
	}
}
