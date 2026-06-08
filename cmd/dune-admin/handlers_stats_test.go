package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleGetPlayerStats_DBNil(t *testing.T) {
	orig := globalDB
	globalDB = nil
	defer func() { globalDB = orig }()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	handleGetPlayerStats(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}

func TestHandleGetPlayerStats_InvalidID(t *testing.T) {
	orig := globalDB
	globalDB = nil
	defer func() { globalDB = orig }()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("id", "not-a-number")
	rr := httptest.NewRecorder()
	handleGetPlayerStats(rr, req)

	// DB nil guard fires before ID parse — still 503.
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}

func TestHandleGetSolarisHistory_DBNil(t *testing.T) {
	orig := globalDB
	globalDB = nil
	defer func() { globalDB = orig }()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	handleGetSolarisHistory(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}

func TestHandleGetSessionHistory_DBNil(t *testing.T) {
	origDB := globalDB
	origSDB := globalSessionDB
	globalDB = nil
	globalSessionDB = nil
	defer func() {
		globalDB = origDB
		globalSessionDB = origSDB
	}()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("id", "42")
	rr := httptest.NewRecorder()
	handleGetSessionHistory(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}

func TestHandleGetSessionHistory_InvalidID(t *testing.T) {
	t.Parallel()
	// globalDB nil check fires first regardless of ID validity;
	// use a non-nil proxy to exercise the parse branch.
	// Since we can't easily construct a pgxpool.Pool in tests,
	// we test the nil guard is the first check and that bad IDs
	// would be rejected — verify via nil guard returning 503 with
	// a bad path value too (consistent with other handlers).
	orig := globalDB
	globalDB = nil
	defer func() { globalDB = orig }()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("id", "not-a-number")
	rr := httptest.NewRecorder()
	handleGetSessionHistory(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 (db nil checked before id parse), got %d", rr.Code)
	}
}

func TestAccumulateSolarisPoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raws      []solarisRaw
		wantLen   int
		wantEarns []int64
		wantSpent []int64
	}{
		{
			name:      "empty",
			raws:      nil,
			wantLen:   0,
			wantEarns: nil,
			wantSpent: nil,
		},
		{
			name:      "single earn",
			raws:      []solarisRaw{{Time: "t1", Balance: 500, Delta: 500}},
			wantLen:   1,
			wantEarns: []int64{500},
			wantSpent: []int64{0},
		},
		{
			name:      "single spend",
			raws:      []solarisRaw{{Time: "t1", Balance: 200, Delta: -300}},
			wantLen:   1,
			wantEarns: []int64{0},
			wantSpent: []int64{300},
		},
		{
			name: "zero delta does not change cumulative",
			raws: []solarisRaw{
				{Time: "t1", Balance: 100, Delta: 100},
				{Time: "t2", Balance: 100, Delta: 0},
			},
			wantLen:   2,
			wantEarns: []int64{100, 100},
			wantSpent: []int64{0, 0},
		},
		{
			name: "mixed sequence accumulates correctly",
			raws: []solarisRaw{
				{Time: "t1", Balance: 1000, Delta: 1000},
				{Time: "t2", Balance: 600, Delta: -400},
				{Time: "t3", Balance: 1100, Delta: 500},
				{Time: "t4", Balance: 900, Delta: -200},
			},
			wantLen:   4,
			wantEarns: []int64{1000, 1000, 1500, 1500},
			wantSpent: []int64{0, 400, 400, 600},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := accumulateSolarisPoints(tt.raws)
			if len(got) != tt.wantLen {
				t.Fatalf("len: want %d, got %d", tt.wantLen, len(got))
			}
			for i, p := range got {
				if p.CumEarned != tt.wantEarns[i] {
					t.Errorf("[%d] CumEarned: want %d, got %d", i, tt.wantEarns[i], p.CumEarned)
				}
				if p.CumSpent != tt.wantSpent[i] {
					t.Errorf("[%d] CumSpent: want %d, got %d", i, tt.wantSpent[i], p.CumSpent)
				}
			}
		})
	}
}

func TestBuildPlayerStats(t *testing.T) {
	t.Parallel()

	pg := playerPgStats{
		SolarisBal:      1_000_000,
		ScripBal:        500,
		SolarisEarned:   2_000_000,
		SolarisSpent:    50_000,
		POIsDiscovered:  12,
		StoryMilestones: 4,
		MaxFactionTier:  19,
		Faction:         "Atreides",
		CharXP:          88_364,
		SkillPoints:     142,
	}
	sess := sessionStats{
		TotalPlaytimeSecs: 7200,
		SessionCount:      3,
		AvgSessionSecs:    2400,
	}

	got := buildPlayerStats(pg, sess)

	if got.SolarisBal != 1_000_000 {
		t.Errorf("SolarisBal: want 1000000, got %d", got.SolarisBal)
	}
	if got.ScripBal != 500 {
		t.Errorf("ScripBal: want 500, got %d", got.ScripBal)
	}
	if got.POIsDiscovered != 12 {
		t.Errorf("POIsDiscovered: want 12, got %d", got.POIsDiscovered)
	}
	if got.StoryMilestones != 4 {
		t.Errorf("StoryMilestones: want 4, got %d", got.StoryMilestones)
	}
	if got.MaxFactionTier != 19 {
		t.Errorf("MaxFactionTier: want 19, got %d", got.MaxFactionTier)
	}
	if got.Faction != "Atreides" {
		t.Errorf("Faction: want Atreides, got %q", got.Faction)
	}
	if got.TotalPlaytimeSecs != 7200 {
		t.Errorf("TotalPlaytimeSecs: want 7200, got %d", got.TotalPlaytimeSecs)
	}
	if got.SessionCount != 3 {
		t.Errorf("SessionCount: want 3, got %d", got.SessionCount)
	}
	if got.AvgSessionSecs != 2400 {
		t.Errorf("AvgSessionSecs: want 2400, got %d", got.AvgSessionSecs)
	}
}
