package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockBattlepassDeps returns deps backed by simple in-memory fixtures.
// The pace field is set to a no-op so existing tests compile unchanged after
// the pace param was added to battlepassDeps.
func mockBattlepassDeps(players []battlepassPlayer, journey map[int64][]string, tags map[int64][]string) battlepassDeps {
	return battlepassDeps{
		fetchPlayers: func(ctx context.Context) ([]battlepassPlayer, error) {
			return players, nil
		},
		fetchCompletedJourneyNodes: func(ctx context.Context, accountID int64) ([]string, error) {
			return journey[accountID], nil
		},
		fetchPlayerTags: func(ctx context.Context, accountID int64) ([]string, error) {
			return tags[accountID], nil
		},
		pace: func(_ context.Context, _ time.Duration) error { return nil },
	}
}

// recordingPace returns a pace func that appends each call's duration to *calls
// and returns nil. Use in tests that verify pacing behaviour without real sleeps.
func recordingPace(calls *[]time.Duration) func(context.Context, time.Duration) error {
	return func(_ context.Context, d time.Duration) error {
		*calls = append(*calls, d)
		return nil
	}
}

// cancellingPace returns a pace func that cancels the supplied CancelFunc and
// returns context.Canceled on its first call, simulating mid-scan cancellation.
func cancellingPace(cancel context.CancelFunc) func(context.Context, time.Duration) error {
	return func(_ context.Context, _ time.Duration) error {
		cancel()
		return context.Canceled
	}
}

func engineTestTiers() []battlepassTier {
	return []battlepassTier{
		{TierKey: "level:5", Category: "level", Label: "Level 5", Signal: battlepassSignalLevel, Threshold: 5, Intel: 10, Enabled: true},
		{TierKey: "level:10", Category: "level", Label: "Level 10", Signal: battlepassSignalLevel, Threshold: 10, Intel: 10, Enabled: true},
		{TierKey: "journey:DA_MQ_FindTheFremen", Category: "story", Label: "Find the Fremen", Signal: battlepassSignalJourneyNode, SignalKey: "DA_MQ_FindTheFremen", Intel: 40, Enabled: true},
		{TierKey: "tag:Exploration.Cave.Large.Altar1", Category: "exploration", Label: "Altar 1", Signal: battlepassSignalPlayerTag, SignalKey: "Exploration.Cave.Large.Altar1", Intel: 5, Enabled: true},
		{TierKey: "level:15", Category: "level", Label: "Level 15", Signal: battlepassSignalLevel, Threshold: 15, Intel: 10, Enabled: false},
	}
}

func seededEngineStore(t *testing.T) *battlepassStore {
	t.Helper()
	s := testBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(engineTestTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return s
}

func TestBattlepassTierSatisfied(t *testing.T) {
	journey := map[string]bool{"DA_MQ_FindTheFremen": true}
	tags := map[string]bool{"Exploration.Cave.Large.Altar1": true}
	tiers := engineTestTiers()

	if !battlepassTierSatisfied(tiers[0], 7, journey, tags) {
		t.Error("level 7 must satisfy level:5")
	}
	if battlepassTierSatisfied(tiers[1], 7, journey, tags) {
		t.Error("level 7 must not satisfy level:10")
	}
	if !battlepassTierSatisfied(tiers[2], 1, journey, tags) {
		t.Error("completed journey node must satisfy journey tier")
	}
	if !battlepassTierSatisfied(tiers[3], 1, journey, tags) {
		t.Error("player tag must satisfy tag tier")
	}
	if battlepassTierSatisfied(tiers[2], 1, map[string]bool{}, tags) {
		t.Error("missing journey node must not satisfy journey tier")
	}
}

func TestBattlepassFirstEvaluationBaselines(t *testing.T) {
	s := seededEngineStore(t)
	players := []battlepassPlayer{{AccountID: 1, PawnID: 100, Name: "Paul", Level: 7}}
	deps := mockBattlepassDeps(players,
		map[int64][]string{1: {"DA_MQ_FindTheFremen"}},
		map[int64][]string{})

	if err := evaluateBattlepassTick(context.Background(), deps, s, false, false, 0); err != nil {
		t.Fatalf("tick: %v", err)
	}

	keys, _ := s.claimedKeys(1)
	if keys["level:5"] != battlepassClaimBaseline {
		t.Errorf("level:5 = %q, want baseline (pre-existing progress)", keys["level:5"])
	}
	if keys["journey:DA_MQ_FindTheFremen"] != battlepassClaimBaseline {
		t.Errorf("journey claim = %q, want baseline", keys["journey:DA_MQ_FindTheFremen"])
	}
	if _, ok := keys["level:10"]; ok {
		t.Error("unsatisfied tier must not be claimed")
	}
	if _, ok := keys["level:15"]; ok {
		t.Error("disabled tier must not be claimed")
	}
	totals, _ := s.earnedTotals()
	if len(totals) != 0 {
		t.Errorf("baseline run must not create earned intel: %v", totals)
	}
}

func TestBattlepassNewUnlocksEarnAfterBaseline(t *testing.T) {
	s := seededEngineStore(t)
	players := []battlepassPlayer{{AccountID: 1, PawnID: 100, Name: "Paul", Level: 7}}
	journey := map[int64][]string{}
	tags := map[int64][]string{}
	deps := mockBattlepassDeps(players, journey, tags)

	// First tick baselines level:5.
	if err := evaluateBattlepassTick(context.Background(), deps, s, false, false, 0); err != nil {
		t.Fatalf("first tick: %v", err)
	}

	// Player progresses: level 12, completes the chapter, discovers the altar.
	players[0].Level = 12
	journey[1] = []string{"DA_MQ_FindTheFremen"}
	tags[1] = []string{"Exploration.Cave.Large.Altar1"}

	if err := evaluateBattlepassTick(context.Background(), deps, s, false, false, 0); err != nil {
		t.Fatalf("second tick: %v", err)
	}

	keys, _ := s.claimedKeys(1)
	if keys["level:5"] != battlepassClaimBaseline {
		t.Errorf("level:5 must stay baseline, got %q", keys["level:5"])
	}
	for _, k := range []string{"level:10", "journey:DA_MQ_FindTheFremen", "tag:Exploration.Cave.Large.Altar1"} {
		if keys[k] != battlepassClaimEarned {
			t.Errorf("%s = %q, want earned", k, keys[k])
		}
	}
	totals, _ := s.earnedTotals()
	if totals[1] != 10+40+5 {
		t.Errorf("earned total = %d, want 55", totals[1])
	}
}

func TestBattlepassAwardPastSkipsBaseline(t *testing.T) {
	s := seededEngineStore(t)
	players := []battlepassPlayer{{AccountID: 1, PawnID: 100, Name: "Paul", Level: 7}}
	deps := mockBattlepassDeps(players, map[int64][]string{}, map[int64][]string{})

	if err := evaluateBattlepassTick(context.Background(), deps, s, true, false, 0); err != nil {
		t.Fatalf("tick: %v", err)
	}

	keys, _ := s.claimedKeys(1)
	if keys["level:5"] != battlepassClaimEarned {
		t.Errorf("award-past mode: level:5 = %q, want earned", keys["level:5"])
	}
}

func TestBattlepassFetchErrorSkipsBaseline(t *testing.T) {
	s := seededEngineStore(t)
	players := []battlepassPlayer{{AccountID: 1, PawnID: 100, Name: "Paul", Level: 7}}
	deps := mockBattlepassDeps(players, map[int64][]string{}, map[int64][]string{})
	deps.fetchCompletedJourneyNodes = func(ctx context.Context, accountID int64) ([]string, error) {
		return nil, fmt.Errorf("db down")
	}

	// Evaluation fails mid-pass: the account must NOT be marked baselined,
	// so the next successful pass still baselines instead of over-rewarding.
	if err := evaluateBattlepassTick(context.Background(), deps, s, false, false, 0); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if baselined, _ := s.isBaselined(1); baselined {
		t.Fatal("account must not be baselined after a failed evaluation pass")
	}

	deps.fetchCompletedJourneyNodes = func(ctx context.Context, accountID int64) ([]string, error) {
		return []string{"DA_MQ_FindTheFremen"}, nil
	}
	if err := evaluateBattlepassTick(context.Background(), deps, s, false, false, 0); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	keys, _ := s.claimedKeys(1)
	if keys["journey:DA_MQ_FindTheFremen"] != battlepassClaimBaseline {
		t.Errorf("journey claim = %q, want baseline", keys["journey:DA_MQ_FindTheFremen"])
	}
}

func TestBattlepassSkipsSignalFetchWhenAllClaimed(t *testing.T) {
	s := seededEngineStore(t)
	players := []battlepassPlayer{{AccountID: 1, PawnID: 100, Name: "Paul", Level: 200}}
	journeyCalls, tagCalls := 0, 0
	deps := battlepassDeps{
		fetchPlayers: func(ctx context.Context) ([]battlepassPlayer, error) { return players, nil },
		fetchCompletedJourneyNodes: func(ctx context.Context, accountID int64) ([]string, error) {
			journeyCalls++
			return []string{"DA_MQ_FindTheFremen"}, nil
		},
		fetchPlayerTags: func(ctx context.Context, accountID int64) ([]string, error) {
			tagCalls++
			return []string{"Exploration.Cave.Large.Altar1"}, nil
		},
		pace: func(_ context.Context, _ time.Duration) error { return nil },
	}

	if err := evaluateBattlepassTick(context.Background(), deps, s, false, false, 0); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if journeyCalls != 1 || tagCalls != 1 {
		t.Fatalf("first tick fetches = %d/%d, want 1/1", journeyCalls, tagCalls)
	}

	// Everything is claimed — second tick must not re-fetch per-player signals.
	if err := evaluateBattlepassTick(context.Background(), deps, s, false, false, 0); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if journeyCalls != 1 || tagCalls != 1 {
		t.Fatalf("second tick re-fetched signals (%d/%d), want no new fetches", journeyCalls, tagCalls)
	}
}

func TestClampBattlepassInterval(t *testing.T) {
	if got := clampBattlepassInterval(0); got != 60*time.Second {
		t.Errorf("default interval = %v, want 60s", got)
	}
	if got := clampBattlepassInterval(5); got != 10*time.Second {
		t.Errorf("clamped low = %v, want 10s", got)
	}
	if got := clampBattlepassInterval(10000); got != 600*time.Second {
		t.Errorf("clamped high = %v, want 600s", got)
	}
	if got := clampBattlepassInterval(120); got != 120*time.Second {
		t.Errorf("interval = %v, want 120s", got)
	}
}

func TestClampBattlepassPaceMs(t *testing.T) {
	// negative → default (75ms)
	if got := clampBattlepassPaceMs(-5); got != 75*time.Millisecond {
		t.Errorf("negative pace = %v, want 75ms default", got)
	}
	// 0 → preserved (explicit opt-out of pacing)
	if got := clampBattlepassPaceMs(0); got != 0 {
		t.Errorf("zero pace = %v, want 0 (no pacing)", got)
	}
	// above max → clamped to 5000ms
	if got := clampBattlepassPaceMs(99999); got != 5000*time.Millisecond {
		t.Errorf("large pace = %v, want 5000ms max", got)
	}
	// in-range passthrough
	if got := clampBattlepassPaceMs(75); got != 75*time.Millisecond {
		t.Errorf("pace 75ms = %v, want 75ms", got)
	}
}

func TestClampBattlepassStartDelayMs(t *testing.T) {
	// negative → default (3000ms)
	if got := clampBattlepassStartDelayMs(-1); got != 3000*time.Millisecond {
		t.Errorf("negative start delay = %v, want 3000ms default", got)
	}
	// 0 → immediate (preserved)
	if got := clampBattlepassStartDelayMs(0); got != 0 {
		t.Errorf("zero start delay = %v, want 0 (immediate)", got)
	}
	// above max → clamped to 30000ms
	if got := clampBattlepassStartDelayMs(99999); got != 30000*time.Millisecond {
		t.Errorf("large start delay = %v, want 30000ms max", got)
	}
	// in-range passthrough
	if got := clampBattlepassStartDelayMs(2000); got != 2000*time.Millisecond {
		t.Errorf("start delay 2000ms = %v, want 2000ms", got)
	}
}

func TestBattlepassPaceCalledBetweenPlayers(t *testing.T) {
	s := seededEngineStore(t)
	players := []battlepassPlayer{
		{AccountID: 1, PawnID: 100, Name: "A", Level: 7},
		{AccountID: 2, PawnID: 200, Name: "B", Level: 7},
		{AccountID: 3, PawnID: 300, Name: "C", Level: 7},
	}
	deps := mockBattlepassDeps(players, map[int64][]string{}, map[int64][]string{})

	var paced []time.Duration
	deps.pace = recordingPace(&paced)

	const paceEvery = 50 * time.Millisecond
	if err := evaluateBattlepassTick(context.Background(), deps, s, false, false, paceEvery); err != nil {
		t.Fatalf("tick: %v", err)
	}

	// pace is called BETWEEN players: for 3 players expect 2 calls.
	if len(paced) != 2 {
		t.Errorf("pace called %d times, want 2 (between 3 players)", len(paced))
	}
	for i, d := range paced {
		if d != paceEvery {
			t.Errorf("pace[%d] = %v, want %v", i, d, paceEvery)
		}
	}
}

func TestBattlepassPaceCancellationStopsLoop(t *testing.T) {
	s := seededEngineStore(t)
	players := []battlepassPlayer{
		{AccountID: 1, PawnID: 100, Name: "A", Level: 7},
		{AccountID: 2, PawnID: 200, Name: "B", Level: 7},
		{AccountID: 3, PawnID: 300, Name: "C", Level: 7},
	}
	deps := mockBattlepassDeps(players, map[int64][]string{}, map[int64][]string{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	deps.pace = cancellingPace(cancel)

	err := evaluateBattlepassTick(ctx, deps, s, false, false, 10*time.Millisecond)
	if err == nil {
		t.Fatal("tick must return an error when ctx is cancelled during pacing")
	}

	// Only the first player should have a claim row; the loop must have stopped.
	claimed, _ := s.claimedKeys(1)
	if len(claimed) == 0 {
		t.Error("first player must have claim rows before cancellation")
	}
	claimed2, _ := s.claimedKeys(2)
	if len(claimed2) != 0 {
		t.Errorf("second player must not be processed after pace cancellation, got %v", claimed2)
	}
}

func TestRunBattlepassEngineBootScanRunsBeforeFirstInterval(t *testing.T) {
	s := seededEngineStore(t)
	players := []battlepassPlayer{{AccountID: 1, PawnID: 100, Name: "Paul", Level: 7}}
	deps := mockBattlepassDeps(players, map[int64][]string{}, map[int64][]string{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Interval is 1 hour — the ticker never fires during this test.
	// startDelay=0 so the boot scan fires immediately.
	go runBattlepassEngine(ctx, deps, s, time.Hour, 0, 0, true, false)

	// Poll claimedKeys: with awardPast=true the engine records claims as "earned"
	// but does NOT call markBaselined (that only runs when !baselined). Waiting for
	// a non-empty claim set is the correct completion signal for award-past mode.
	deadline := time.After(2 * time.Second)
	for {
		keys, _ := s.claimedKeys(1)
		if len(keys) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("boot scan did not complete within 2s; ticker fires after full interval, not at t=0")
		case <-time.After(5 * time.Millisecond):
		}
	}

	// Verify the scan recorded claims with the expected status.
	keys, _ := s.claimedKeys(1)
	if keys["level:5"] != battlepassClaimEarned {
		t.Errorf("boot scan: level:5 = %q, want earned (award_past=true)", keys["level:5"])
	}
}

func TestRunBattlepassEngineBootScanHonorsCancelDuringStartDelay(t *testing.T) {
	s := seededEngineStore(t)

	fetchCalled := make(chan struct{}, 1)
	players := []battlepassPlayer{{AccountID: 1, PawnID: 100, Name: "Paul", Level: 7}}
	deps := mockBattlepassDeps(players, map[int64][]string{}, map[int64][]string{})
	deps.fetchPlayers = func(ctx context.Context) ([]battlepassPlayer, error) {
		fetchCalled <- struct{}{}
		return players, nil
	}
	// Start delay: reuse pace as the seam (see bootBattlepassScan design).
	// We make pace block until ctx is cancelled, simulating a long start delay.
	paceBlocked := make(chan struct{})
	deps.pace = func(ctx context.Context, _ time.Duration) error {
		close(paceBlocked)
		<-ctx.Done()
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		runBattlepassEngine(ctx, deps, s, time.Hour, 0, 5*time.Second, true, false)
	}()

	// Wait until the goroutine enters the start-delay pace, then cancel.
	<-paceBlocked
	cancel()

	select {
	case <-done:
		// goroutine exited cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("runBattlepassEngine did not exit after ctx cancel during start delay")
	}

	// fetchPlayers must never have been called.
	select {
	case <-fetchCalled:
		t.Fatal("fetchPlayers was called despite ctx being cancelled during start delay")
	default:
	}
}

func TestBattlepassAwardPastPopulatesEarnedTotals(t *testing.T) {
	s := seededEngineStore(t)
	players := []battlepassPlayer{{AccountID: 1, PawnID: 100, Name: "Paul", Level: 7}}
	deps := mockBattlepassDeps(players, map[int64][]string{}, map[int64][]string{})

	if err := evaluateBattlepassTick(context.Background(), deps, s, true, false, 0); err != nil {
		t.Fatalf("tick: %v", err)
	}

	totals, err := s.earnedTotals()
	if err != nil {
		t.Fatalf("earnedTotals: %v", err)
	}
	if len(totals) == 0 {
		t.Error("award_past=true must produce non-empty earned totals (Pending dashboard source)")
	}
}
