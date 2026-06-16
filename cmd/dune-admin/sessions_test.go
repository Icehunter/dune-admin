package main

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestSessionDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openSessionDB(":memory:")
	if err != nil {
		t.Fatalf("openSessionDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpenSessionDB_CreatesSchema(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)

	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='play_sessions'`).Scan(&name)
	if err != nil {
		t.Fatalf("play_sessions table not created: %v", err)
	}
	if name != "play_sessions" {
		t.Fatalf("expected table name 'play_sessions', got %q", name)
	}
}

func TestRecordSessions_StartsNewSession(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)

	if err := recordSessions(context.Background(), []int64{42}, db, defaultServerID); err != nil {
		t.Fatalf("recordSessions: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM play_sessions WHERE account_id = 42 AND ended_at IS NULL`).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 open session for account 42, got %d", count)
	}
}

func TestRecordSessions_ClosesSession(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	ctx := context.Background()

	if err := recordSessions(ctx, []int64{42}, db, defaultServerID); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if err := recordSessions(ctx, []int64{}, db, defaultServerID); err != nil {
		t.Fatalf("second record (offline): %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM play_sessions WHERE account_id = 42 AND ended_at IS NOT NULL AND duration_secs >= 0`).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 closed session for account 42, got %d", count)
	}
}

func TestRecordSessions_ContinuesActiveSession(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	ctx := context.Background()

	if err := recordSessions(ctx, []int64{42}, db, defaultServerID); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if err := recordSessions(ctx, []int64{42}, db, defaultServerID); err != nil {
		t.Fatalf("second record: %v", err)
	}

	var total, open int
	if err := db.QueryRow(`SELECT COUNT(*) FROM play_sessions WHERE account_id = 42`).Scan(&total); err != nil {
		t.Fatalf("total query: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM play_sessions WHERE account_id = 42 AND ended_at IS NULL`).Scan(&open); err != nil {
		t.Fatalf("open query: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 total session, got %d", total)
	}
	if open != 1 {
		t.Fatalf("expected 1 open session, got %d", open)
	}
}

func TestGetSessionStats(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx,
		`INSERT INTO play_sessions(server_id, account_id, started_at, ended_at, duration_secs) VALUES(1, 7, '2026-01-01T10:00:00Z', '2026-01-01T11:00:00Z', 3600)`,
	); err != nil {
		t.Fatalf("insert session 1: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO play_sessions(server_id, account_id, started_at, ended_at, duration_secs) VALUES(1, 7, '2026-01-02T10:00:00Z', '2026-01-02T10:30:00Z', 1800)`,
	); err != nil {
		t.Fatalf("insert session 2: %v", err)
	}
	// Open session should not count toward totals.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO play_sessions(server_id, account_id, started_at) VALUES(1, 7, '2026-01-03T10:00:00Z')`,
	); err != nil {
		t.Fatalf("insert open session: %v", err)
	}

	stats, err := getSessionStats(ctx, db, defaultServerID, 7)
	if err != nil {
		t.Fatalf("getSessionStats: %v", err)
	}
	if stats.TotalPlaytimeSecs != 5400 {
		t.Fatalf("expected 5400 total secs, got %d", stats.TotalPlaytimeSecs)
	}
	if stats.SessionCount != 2 {
		t.Fatalf("expected 2 sessions, got %d", stats.SessionCount)
	}
	if stats.AvgSessionSecs != 2700 {
		t.Fatalf("expected 2700 avg secs, got %d", stats.AvgSessionSecs)
	}
}

func TestGetSessionStats_NoSessions(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)

	stats, err := getSessionStats(context.Background(), db, defaultServerID, 999)
	if err != nil {
		t.Fatalf("getSessionStats for unknown account: %v", err)
	}
	if stats.TotalPlaytimeSecs != 0 || stats.SessionCount != 0 || stats.AvgSessionSecs != 0 {
		t.Fatalf("expected zero stats for unknown account, got %+v", stats)
	}
}

func TestCloseOrphanedSessions(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx,
		`INSERT INTO play_sessions(server_id, account_id, started_at) VALUES(1, 99, '2026-01-01T10:00:00Z')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := closeOrphanedSessions(db, defaultServerID); err != nil {
		t.Fatalf("closeOrphanedSessions: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM play_sessions WHERE account_id = 99 AND ended_at IS NOT NULL AND duration_secs = 0`).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected orphaned session closed with 0 duration, got count=%d", count)
	}
}

func TestRecordSessions_ServerIDIsolation(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	ctx := context.Background()

	// server A sees account 1; server B sees account 2 — they must not bleed.
	if err := recordSessions(ctx, []int64{1}, db, 1); err != nil {
		t.Fatalf("recordSessions srvA: %v", err)
	}
	if err := recordSessions(ctx, []int64{2}, db, 2); err != nil {
		t.Fatalf("recordSessions srvB: %v", err)
	}

	var countA, countB int
	if err := db.QueryRow(`SELECT COUNT(*) FROM play_sessions WHERE server_id=1`).Scan(&countA); err != nil {
		t.Fatalf("count srvA: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM play_sessions WHERE server_id=2`).Scan(&countB); err != nil {
		t.Fatalf("count srvB: %v", err)
	}
	if countA != 1 {
		t.Errorf("srvA: want 1 session, got %d", countA)
	}
	if countB != 1 {
		t.Errorf("srvB: want 1 session, got %d", countB)
	}

	// srvA stats for account 2 (a srvB account) must be zero.
	statsA, err := getSessionStats(ctx, db, 1, 2)
	if err != nil {
		t.Fatalf("getSessionStats srvA/acct2: %v", err)
	}
	if statsA.SessionCount != 0 {
		t.Errorf("srvA should not see srvB account 2: got %d sessions", statsA.SessionCount)
	}
}
