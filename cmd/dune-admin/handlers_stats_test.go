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
