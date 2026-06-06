package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// handleGetPlayerSummary guards on globalDB before doing any work, so with no
// database connection (globalDB is nil in unit tests — connectAll is never
// called) it must surface 503. Not parallel: it reads the globalDB package
// global. Mirrors TestHandleGetMapMarkers_Input.
func TestHandleGetPlayerSummary_Guard(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/players/summary", nil)
	rec := httptest.NewRecorder()

	handleGetPlayerSummary(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

// fillActivityTrend is the pure, testable core of the dashboard's activity
// trend: given a sparse day->count map it returns a contiguous, ascending daily
// series of length `days` ending on `today` (UTC), zero-filling inactive days
// so the chart shows gaps as 0 rather than collapsing them. `today` is injected
// so the test is deterministic (no time.Now).
func TestFillActivityTrend(t *testing.T) {
	t.Parallel()
	today := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	t.Run("zero-fills missing days, ascending, ends today", func(t *testing.T) {
		t.Parallel()
		counts := map[string]int64{"2026-06-04": 3, "2026-06-06": 5}
		got := fillActivityTrend(3, today, counts)
		want := []activityPoint{
			{Day: "2026-06-04", Count: 3},
			{Day: "2026-06-05", Count: 0},
			{Day: "2026-06-06", Count: 5},
		}
		if len(got) != len(want) {
			t.Fatalf("len = %d, want %d (%+v)", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("point[%d] = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("ignores counts outside the window", func(t *testing.T) {
		t.Parallel()
		counts := map[string]int64{"2026-05-01": 99, "2026-06-06": 1}
		got := fillActivityTrend(2, today, counts)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2 (%+v)", len(got), got)
		}
		if got[0] != (activityPoint{Day: "2026-06-05", Count: 0}) || got[1] != (activityPoint{Day: "2026-06-06", Count: 1}) {
			t.Fatalf("window = %+v, want [05=0, 06=1]", got)
		}
	})

	t.Run("days < 1 is coerced to a single day (today)", func(t *testing.T) {
		t.Parallel()
		got := fillActivityTrend(0, today, nil)
		if len(got) != 1 || got[0].Day != "2026-06-06" || got[0].Count != 0 {
			t.Fatalf("got %+v, want a single zero point for today", got)
		}
	})
}

// averageLevel is the dashboard's "avg character level" (#130) — the mean of
// per-character levels via xpToLevel (NOT the level of the mean XP, since the
// XP→level curve is non-linear). 344440 XP = level 200 (the cap).
func TestAverageLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		xps  []int64
		want float64
	}{
		{name: "empty is zero", xps: nil, want: 0},
		{name: "single max-level char", xps: []int64{344440}, want: 200},
		{name: "averages levels not raw xp", xps: []int64{0, 344440}, want: 100},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := averageLevel(tt.xps); got != tt.want {
				t.Fatalf("averageLevel(%v) = %v, want %v", tt.xps, got, tt.want)
			}
		})
	}
}

// avgLevelsByFaction (#130 ext v2) — mean character level per faction via
// xpToLevel; averages levels within each faction bucket, empty input → {}.
func TestAvgLevelsByFaction(t *testing.T) {
	t.Parallel()
	got := avgLevelsByFaction([]factionXP{
		{Faction: "Atreides", XP: 344440},  // level 200
		{Faction: "Atreides", XP: 0},       // level 0
		{Faction: "Unaligned", XP: 344440}, // level 200
	})
	if got["Atreides"] != 100 {
		t.Errorf("Atreides avg = %v, want 100", got["Atreides"])
	}
	if got["Unaligned"] != 200 {
		t.Errorf("Unaligned avg = %v, want 200", got["Unaligned"])
	}
	if len(avgLevelsByFaction(nil)) != 0 {
		t.Errorf("nil input: want empty map, got %v", avgLevelsByFaction(nil))
	}
}
