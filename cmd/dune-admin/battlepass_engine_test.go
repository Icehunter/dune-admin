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

// TestBattlepassTierAction covers the pure per-(account,tier) baseline
// decision that replaced the per-account isBaselined/markBaselined gate
// (#297): a tier only earns once the engine has actually watched it
// transition from unsatisfied to satisfied (seen=true), or when the operator
// opts in via awardPast. An unsatisfied tier is never recorded — it's only
// (idempotently) marked as watched, regardless of seen/awardPast.
func TestBattlepassTierAction(t *testing.T) {
	tests := []struct {
		name       string
		satisfied  bool
		seen       bool
		awardPast  bool
		wantRecord bool
		wantStatus string
		wantMark   bool
	}{
		{"unsatisfied marks seen, never records", false, false, false, false, "", true},
		{"unsatisfied ignores seen and awardPast", false, true, true, false, "", true},
		{"satisfied on first sight baselines", true, false, false, true, battlepassClaimBaseline, false},
		{"satisfied after being watched unsatisfied earns", true, true, false, true, battlepassClaimEarned, false},
		{"satisfied with awardPast earns even on first sight", true, false, true, true, battlepassClaimEarned, false},
		{"satisfied with awardPast and seen still earns", true, true, true, true, battlepassClaimEarned, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, status, mark := battlepassTierAction(tt.satisfied, tt.seen, tt.awardPast)
			if record != tt.wantRecord || status != tt.wantStatus || mark != tt.wantMark {
				t.Errorf("battlepassTierAction(satisfied=%v, seen=%v, awardPast=%v) = (%v, %q, %v), want (%v, %q, %v)",
					tt.satisfied, tt.seen, tt.awardPast, record, status, mark,
					tt.wantRecord, tt.wantStatus, tt.wantMark)
			}
		})
	}
}

// TestBattlepassNewTierAddedAfterAccountActive_BaselinesInsteadOfEarning is
// the direct regression test for the reported bug: a game update introduced
// achievement tiers (Steam-achievement journey nodes that are satisfied by
// default for every character, independent of real progress) after existing
// accounts already had claim history. The old per-ACCOUNT baseline flag only
// protected an account's very first evaluation pass ever; any tier added
// later that happened to already be satisfied was recorded as a fresh
// "earned" claim and auto-granted real in-game intel to players who never did
// anything to earn it (e.g. a 32-XP character showing 56 granted
// achievements). The fix: a tier only earns once the engine has actually
// watched THAT TIER transition from unsatisfied to satisfied for THIS
// account — a default-true tier the engine never saw unsatisfied baselines
// instead, exactly like first-run progress does.
func TestBattlepassNewTierAddedAfterAccountActive_BaselinesInsteadOfEarning(t *testing.T) {
	s := seededEngineStore(t)
	players := []battlepassPlayer{{AccountID: 1, PawnID: 100, Name: "LowLevelPlayer", Level: 1}}
	deps := mockBattlepassDeps(players, map[int64][]string{}, map[int64][]string{})

	// Account is already active: level:5 was watched unsatisfied and earned in
	// an earlier era, long before the new tier below ever existed.
	if err := s.recordClaim("level:5", 1, 10, battlepassClaimEarned); err != nil {
		t.Fatalf("seed prior claim: %v", err)
	}

	// A battlepass update adds a new achievement tier that is satisfied by
	// default — exactly like DA_ACH_SteamAchievements.sb-ach-exploration10.
	newTier := battlepassTier{
		TierKey: "journey:DA_ACH_SteamAchievements.sb-ach-exploration10", Category: "achievement",
		Label: "Achievement: Exploration 10", Signal: battlepassSignalJourneyNode,
		SignalKey: "DA_ACH_SteamAchievements.sb-ach-exploration10", Intel: 2, Enabled: true,
	}
	if _, err := s.createTier(newTier); err != nil {
		t.Fatalf("add new tier: %v", err)
	}
	// The node is satisfied for this account from the very first observation —
	// the engine never watched it unsatisfied.
	journey := map[int64][]string{1: {"DA_ACH_SteamAchievements.sb-ach-exploration10"}}
	deps.fetchCompletedJourneyNodes = func(ctx context.Context, accountID int64) ([]string, error) {
		return journey[accountID], nil
	}

	if err := evaluateBattlepassTick(context.Background(), deps, s, false, true, 0); err != nil {
		t.Fatalf("tick: %v", err)
	}

	keys, err := s.claimedKeys(1)
	if err != nil {
		t.Fatalf("claimedKeys: %v", err)
	}
	const newKey = "journey:DA_ACH_SteamAchievements.sb-ach-exploration10"
	if got := keys[newKey]; got != battlepassClaimBaseline {
		t.Fatalf("new default-satisfied tier = %q, want baseline (not earned)", got)
	}

	due, err := s.listRetryableGrantLedger(time.Now())
	if err != nil {
		t.Fatalf("listRetryableGrantLedger: %v", err)
	}
	for _, d := range due {
		if d.TierKey == newKey {
			t.Fatalf("bogus achievement tier must not be auto-granted, got pending grant %+v", d)
		}
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

	// Evaluation fails mid-pass: no claims and no seen-unsatisfied markers may
	// be recorded, so the next successful pass still baselines pre-existing
	// progress instead of over-rewarding it.
	if err := evaluateBattlepassTick(context.Background(), deps, s, false, false, 0); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if keys, _ := s.claimedKeys(1); len(keys) != 0 {
		t.Fatalf("claims recorded despite fetch failure: %v", keys)
	}
	if seen, _ := s.seenUnsatisfiedKeys(1); len(seen) != 0 {
		t.Fatalf("seen-unsatisfied markers recorded despite fetch failure: %v", seen)
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

// TestBattlepassRetroactiveGrantTiers covers the pure filter used to fix
// #259/#280: enabling auto-grant is not retroactive on its own, because
// battlepassUnclaimed skips any tier already present in `claimed` — a tier
// earned before auto-grant was turned on would otherwise never be
// (re-)enqueued. Only already-earned (not baseline, not already-granted)
// tiers should come back.
func TestBattlepassRetroactiveGrantTiers(t *testing.T) {
	claimed := map[string]string{
		"already_granted":      battlepassClaimGranted,
		"still_baseline":       battlepassClaimBaseline,
		"earned_no_ledger_yet": battlepassClaimEarned,
	}
	got := battlepassRetroactiveGrantTiers(claimed)
	if len(got) != 1 || got[0] != "earned_no_ledger_yet" {
		t.Fatalf("battlepassRetroactiveGrantTiers = %v, want only [earned_no_ledger_yet]", got)
	}
}

// TestBattlepassAutoGrantEnabledLater_RetroactivelyEnqueuesAlreadyEarnedTier
// covers the actual reported bug: a tier earned while auto-grant was off (so
// it was never enqueued on the grant ledger) must get enqueued once
// auto-grant turns on, even though the tick that discovers this doesn't
// newly satisfy any tier — level:5 stays at the same level, so it never
// re-enters `unclaimed`.
func TestBattlepassAutoGrantEnabledLater_RetroactivelyEnqueuesAlreadyEarnedTier(t *testing.T) {
	s := seededEngineStore(t)
	// Simulate a tier earned before auto-grant was ever enabled: recorded
	// directly on battlepass_claims with no matching grant-ledger row, exactly
	// what evaluateBattlepassPlayer produced while autoGrant was false.
	if err := s.recordClaim("level:5", 1, 10, battlepassClaimEarned); err != nil {
		t.Fatalf("recordClaim: %v", err)
	}
	if err := s.markBaselined(1); err != nil {
		t.Fatalf("markBaselined: %v", err)
	}

	players := []battlepassPlayer{{AccountID: 1, PawnID: 100, Name: "Paul", Level: 5, Online: false}}
	deps := mockBattlepassDeps(players, map[int64][]string{}, map[int64][]string{})

	// Auto-grant is now on. Nothing newly unlocks this pass — the pre-existing
	// earned claim must still be enqueued.
	if err := evaluateBattlepassTick(context.Background(), deps, s, true, true, 0); err != nil {
		t.Fatalf("evaluateBattlepassTick: %v", err)
	}

	due, err := s.listRetryableGrantLedger(time.Now())
	if err != nil {
		t.Fatalf("listRetryableGrantLedger: %v", err)
	}
	if len(due) != 1 || due[0].TierKey != "level:5" || due[0].AccountID != 1 {
		t.Fatalf("want 1 retroactively-enqueued pending grant for level:5/1, got %+v", due)
	}
}

// TestBattlepassAutoGrantOff_DoesNotRetroactivelyEnqueue guards against the
// fix over-firing: with auto-grant off, an already-earned tier must not gain
// a ledger row (it stays manual-only, matching pre-#259/#280 behaviour).
func TestBattlepassAutoGrantOff_DoesNotRetroactivelyEnqueue(t *testing.T) {
	s := seededEngineStore(t)
	if err := s.recordClaim("level:5", 1, 10, battlepassClaimEarned); err != nil {
		t.Fatalf("recordClaim: %v", err)
	}
	if err := s.markBaselined(1); err != nil {
		t.Fatalf("markBaselined: %v", err)
	}

	players := []battlepassPlayer{{AccountID: 1, PawnID: 100, Name: "Paul", Level: 5}}
	deps := mockBattlepassDeps(players, map[int64][]string{}, map[int64][]string{})

	if err := evaluateBattlepassTick(context.Background(), deps, s, true, false, 0); err != nil {
		t.Fatalf("evaluateBattlepassTick: %v", err)
	}

	due, err := s.listRetryableGrantLedger(time.Now())
	if err != nil {
		t.Fatalf("listRetryableGrantLedger: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("want 0 pending grants with auto-grant off, got %+v", due)
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

	// Poll claimedKeys: with awardPast=true every satisfied tier earns
	// immediately (battlepassTierAction always returns earned when awardPast
	// is set). Waiting for a non-empty claim set is the correct completion
	// signal for award-past mode.
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
