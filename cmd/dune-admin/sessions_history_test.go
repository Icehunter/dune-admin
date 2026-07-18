package main

import (
	"context"
	"fmt"
	"testing"
)

func TestGetSessionHistory_ReturnsSortedClosedSessions(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `INSERT INTO play_sessions(server_id,account_id,started_at,ended_at,duration_secs) VALUES(1,42,'2026-01-01T10:00:00Z','2026-01-01T11:00:00Z',3600)`); err != nil {
		t.Fatalf("insert session 1: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO play_sessions(server_id,account_id,started_at,ended_at,duration_secs) VALUES(1,42,'2026-01-02T10:00:00Z','2026-01-02T10:30:00Z',1800)`); err != nil {
		t.Fatalf("insert session 2: %v", err)
	}
	// open session — must NOT appear
	if _, err := db.ExecContext(ctx, `INSERT INTO play_sessions(server_id,account_id,started_at) VALUES(1,42,'2026-01-03T10:00:00Z')`); err != nil {
		t.Fatalf("insert open session: %v", err)
	}

	recs, err := getSessionHistory(ctx, db, defaultServerID, 42, 50)
	if err != nil {
		t.Fatalf("getSessionHistory: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 closed sessions, got %d", len(recs))
	}
	if recs[0].DurationSecs != 3600 {
		t.Errorf("first session: want 3600s, got %d", recs[0].DurationSecs)
	}
	if recs[1].DurationSecs != 1800 {
		t.Errorf("second session: want 1800s, got %d", recs[1].DurationSecs)
	}
}

func TestGetSessionHistory_Empty(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	recs, err := getSessionHistory(context.Background(), db, defaultServerID, 999, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected 0 records, got %d", len(recs))
	}
}

func TestGetSessionHistory_RespectsLimit(t *testing.T) {
	t.Parallel()
	db := openTestSessionDB(t)
	ctx := context.Background()

	for i := range 5 {
		ts := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		te := fmt.Sprintf("2026-01-%02dT11:00:00Z", i+1)
		if _, err := db.ExecContext(ctx,
			`INSERT INTO play_sessions(server_id,account_id,started_at,ended_at,duration_secs) VALUES(1,77,?,?,3600)`,
			ts, te,
		); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	recs, err := getSessionHistory(ctx, db, defaultServerID, 77, 3)
	if err != nil {
		t.Fatalf("getSessionHistory: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("expected 3 records with limit=3, got %d", len(recs))
	}
	// The cap must keep the NEWEST rows (#294): a chart that hits the limit
	// must window the most recent history, still in ascending order.
	wantStarts := []string{"2026-01-03T10:00:00Z", "2026-01-04T10:00:00Z", "2026-01-05T10:00:00Z"}
	for i, r := range recs {
		if r.StartedAt != wantStarts[i] {
			t.Errorf("[%d] StartedAt = %s, want %s (newest window, ascending)", i, r.StartedAt, wantStarts[i])
		}
	}
}
