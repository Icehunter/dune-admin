package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// seedEarnedTier inserts a tier and an earned claim + pending ledger row for an
// account, mimicking what evaluation does when auto-grant is on.
func seedEarnedTier(t *testing.T, s *battlepassStore, tierKey string, accountID, intel int64) {
	t.Helper()
	if _, err := s.createTier(battlepassTier{
		TierKey: tierKey, Category: "level", Label: tierKey,
		Signal: battlepassSignalLevel, Threshold: 1, Intel: intel, Enabled: true,
	}); err != nil {
		t.Fatalf("createTier: %v", err)
	}
	if err := s.recordClaim(tierKey, accountID, intel, battlepassClaimEarned); err != nil {
		t.Fatalf("recordClaim: %v", err)
	}
	if err := s.recordPendingGrant(tierKey, accountID); err != nil {
		t.Fatalf("recordPendingGrant: %v", err)
	}
}

func noopBattlepassGrantDeps() battlepassGrantDeps {
	return battlepassGrantDeps{
		fetchPlayers: func(_ context.Context) ([]battlepassPlayer, error) { return nil, nil },
		awardIntel:   func(_ context.Context, _, _ int64) error { return nil },
		giveItem:     func(_ context.Context, _ int64, _ string, _, _ int64) error { return nil },
		resolveGrantTarget: func(_ context.Context, _ int64) (int64, error) {
			return 0, errNotFound
		},
	}
}

func TestAttemptBattlepassGrant_OfflineSuccess(t *testing.T) {
	s := openMemBattlepassStore(t)
	seedEarnedTier(t, s, "tier_x", 100, 50)

	var intelGranted int64
	deps := noopBattlepassGrantDeps()
	// resolveGrantTarget works for an OFFLINE player (no online filter).
	deps.resolveGrantTarget = func(_ context.Context, accountID int64) (int64, error) {
		if accountID != 100 {
			t.Errorf("resolve accountID = %d, want 100", accountID)
		}
		return 999, nil // pawn id
	}
	deps.awardIntel = func(_ context.Context, pawnID, amount int64) error {
		if pawnID != 999 {
			t.Errorf("awardIntel pawn = %d, want 999", pawnID)
		}
		intelGranted += amount
		return nil
	}

	if err := attemptBattlepassGrant(context.Background(), s, deps, "tier_x", 100); err != nil {
		t.Fatalf("attemptBattlepassGrant: %v", err)
	}
	if intelGranted != 50 {
		t.Errorf("intel granted = %d, want 50", intelGranted)
	}

	// Ledger row is granted (terminal) and claim flipped to granted.
	due, _ := s.listRetryableGrantLedger(time.Now().Add(100 * deferredGrantRetryBackoff))
	if len(due) != 0 {
		t.Errorf("want 0 retryable after success, got %d", len(due))
	}
	claims, _ := s.listClaims(100)
	if len(claims) != 1 || claims[0].Status != battlepassClaimGranted {
		t.Errorf("claim status = %+v, want granted", claims)
	}
}

func TestAttemptBattlepassGrant_FailThenSucceed(t *testing.T) {
	s := openMemBattlepassStore(t)
	seedEarnedTier(t, s, "tier_y", 200, 30)

	calls := 0
	deps := noopBattlepassGrantDeps()
	deps.resolveGrantTarget = func(_ context.Context, _ int64) (int64, error) { return 1, nil }
	deps.awardIntel = func(_ context.Context, _, _ int64) error {
		calls++
		if calls == 1 {
			return errors.New("db hiccup")
		}
		return nil
	}

	// First attempt fails: ledger goes pending with backoff, error surfaced.
	if err := attemptBattlepassGrant(context.Background(), s, deps, "tier_y", 200); err == nil {
		t.Fatalf("want error on first attempt")
	}
	// Not due immediately (backoff in the future).
	if due, _ := s.listRetryableGrantLedger(time.Now()); len(due) != 0 {
		t.Fatalf("want 0 due right after failure, got %d", len(due))
	}
	// Due after backoff; second attempt succeeds.
	due, _ := s.listRetryableGrantLedger(time.Now().Add(deferredGrantRetryBackoff + time.Hour))
	if len(due) != 1 {
		t.Fatalf("want 1 due after backoff, got %d", len(due))
	}
	if err := attemptBattlepassGrant(context.Background(), s, deps, "tier_y", 200); err != nil {
		t.Fatalf("second attempt: %v", err)
	}
	if due, _ := s.listRetryableGrantLedger(time.Now().Add(100 * deferredGrantRetryBackoff)); len(due) != 0 {
		t.Errorf("want 0 retryable after success, got %d", len(due))
	}
}

func TestAttemptBattlepassGrant_ExhaustsAfterMaxAttempts(t *testing.T) {
	s := openMemBattlepassStore(t)
	seedEarnedTier(t, s, "tier_z", 300, 10)

	deps := noopBattlepassGrantDeps()
	deps.resolveGrantTarget = func(_ context.Context, _ int64) (int64, error) { return 1, nil }
	deps.awardIntel = func(_ context.Context, _, _ int64) error { return errors.New("always fails") }

	for i := 0; i < deferredGrantMaxAttempts; i++ {
		if err := attemptBattlepassGrant(context.Background(), s, deps, "tier_z", 300); err == nil {
			t.Fatalf("attempt %d: want error", i)
		}
	}
	due, _ := s.listRetryableGrantLedger(time.Now().Add(100 * deferredGrantRetryBackoff))
	if len(due) != 0 {
		t.Errorf("want 0 retryable after exhaustion, got %d", len(due))
	}
}

// TestAttemptBattlepassGrant_OnlineFailureRetriesWithoutExhausting covers the
// #259/#280 fix: when awardIntel fails specifically because the player is
// online (errPlayerOnline), the ledger must retry on a short backoff and
// never exhaust — many consecutive online-failures (far more than
// deferredGrantMaxAttempts) must still leave the row retryable.
func TestAttemptBattlepassGrant_OnlineFailureRetriesWithoutExhausting(t *testing.T) {
	s := openMemBattlepassStore(t)
	seedEarnedTier(t, s, "tier_online", 500, 40)

	deps := noopBattlepassGrantDeps()
	deps.resolveGrantTarget = func(_ context.Context, _ int64) (int64, error) { return 1, nil }
	deps.awardIntel = func(_ context.Context, _, _ int64) error { return errPlayerOnline }

	for i := 0; i < deferredGrantMaxAttempts+5; i++ {
		if err := attemptBattlepassGrant(context.Background(), s, deps, "tier_online", 500); err == nil {
			t.Fatalf("attempt %d: want error while player is online", i)
		}
	}

	// Still retryable after far more "failures" than deferredGrantMaxAttempts
	// would normally tolerate — online is not a real failure.
	due, err := s.listRetryableGrantLedger(time.Now().Add(24 * time.Hour))
	if err != nil {
		t.Fatalf("listRetryableGrantLedger: %v", err)
	}
	if len(due) != 1 || due[0].TierKey != "tier_online" {
		t.Fatalf("want tier_online still retryable, got %+v", due)
	}
	if due[0].Attempts != 0 {
		t.Errorf("attempts = %d, want 0 — online failures must not consume attempts", due[0].Attempts)
	}

	// The player finally logs out: the very next attempt succeeds.
	deps.awardIntel = func(_ context.Context, _, amount int64) error { return nil }
	if err := attemptBattlepassGrant(context.Background(), s, deps, "tier_online", 500); err != nil {
		t.Fatalf("attempt after logout: %v", err)
	}
	if due, _ := s.listRetryableGrantLedger(time.Now().Add(100 * deferredGrantRetryBackoff)); len(due) != 0 {
		t.Errorf("want 0 retryable after success, got %d", len(due))
	}
}

func TestBattlepassGrantSource_DrivesRetryLoop(t *testing.T) {
	s := openMemBattlepassStore(t)
	seedEarnedTier(t, s, "tier_loop", 400, 25)

	deps := noopBattlepassGrantDeps()
	deps.resolveGrantTarget = func(_ context.Context, _ int64) (int64, error) { return 1, nil }
	granted := 0
	deps.awardIntel = func(_ context.Context, _, amount int64) error {
		granted += int(amount)
		return nil
	}

	src := newBattlepassGrantSource(s, deps)
	processDeferredGrantTick(context.Background(), src, src.attempt, time.Now())
	if granted != 25 {
		t.Errorf("granted intel via loop = %d, want 25", granted)
	}
}

func TestEvaluateBattlepass_AutoGrantEnqueuesPending(t *testing.T) {
	s := openMemBattlepassStore(t)
	if _, err := s.createTier(battlepassTier{
		TierKey: "lvl5", Category: "level", Label: "Level 5",
		Signal: battlepassSignalLevel, Threshold: 5, Intel: 100, Enabled: true,
	}); err != nil {
		t.Fatalf("createTier: %v", err)
	}

	deps := battlepassDeps{
		fetchPlayers: func(_ context.Context) ([]battlepassPlayer, error) {
			return []battlepassPlayer{{AccountID: 77, PawnID: 1, Level: 10, Online: false}}, nil
		},
		fetchCompletedJourneyNodes: func(_ context.Context, _ int64) ([]string, error) { return nil, nil },
		fetchPlayerTags:            func(_ context.Context, _ int64) ([]string, error) { return nil, nil },
		pace:                       func(_ context.Context, _ time.Duration) error { return nil },
	}

	// awardPast=true so the satisfied tier is earned (not baseline), autoGrant=true.
	if err := evaluateBattlepassTick(context.Background(), deps, s, true, true, 0); err != nil {
		t.Fatalf("evaluateBattlepassTick: %v", err)
	}

	due, err := s.listRetryableGrantLedger(time.Now())
	if err != nil {
		t.Fatalf("listRetryableGrantLedger: %v", err)
	}
	if len(due) != 1 || due[0].TierKey != "lvl5" || due[0].AccountID != 77 {
		t.Fatalf("want 1 pending grant for lvl5/77, got %+v", due)
	}
}

func TestEvaluateBattlepass_AutoGrantOff_NoLedger(t *testing.T) {
	s := openMemBattlepassStore(t)
	if _, err := s.createTier(battlepassTier{
		TierKey: "lvl5", Category: "level", Label: "Level 5",
		Signal: battlepassSignalLevel, Threshold: 5, Intel: 100, Enabled: true,
	}); err != nil {
		t.Fatalf("createTier: %v", err)
	}
	deps := battlepassDeps{
		fetchPlayers: func(_ context.Context) ([]battlepassPlayer, error) {
			return []battlepassPlayer{{AccountID: 77, PawnID: 1, Level: 10}}, nil
		},
		fetchCompletedJourneyNodes: func(_ context.Context, _ int64) ([]string, error) { return nil, nil },
		fetchPlayerTags:            func(_ context.Context, _ int64) ([]string, error) { return nil, nil },
		pace:                       func(_ context.Context, _ time.Duration) error { return nil },
	}
	if err := evaluateBattlepassTick(context.Background(), deps, s, true, false, 0); err != nil {
		t.Fatalf("evaluateBattlepassTick: %v", err)
	}
	due, _ := s.listRetryableGrantLedger(time.Now())
	if len(due) != 0 {
		t.Fatalf("want 0 pending grants with auto-grant off, got %d", len(due))
	}
}

func TestApplyBattlepassEngine_NilStoreNoPanic(t *testing.T) {
	orig := globalBattlepassStore
	t.Cleanup(func() { globalBattlepassStore = orig })
	globalBattlepassStore = nil
	// Must return cleanly with no store.
	applyBattlepassEngine(appConfig{})
}

func TestBattlepassAutoGrant_DefaultOff(t *testing.T) {
	if battlepassAutoGrant(appConfig{}) {
		t.Errorf("battlepassAutoGrant default = true, want false")
	}
	if !battlepassAutoGrant(appConfig{BattlepassAutoGrant: boolPtr(true)}) {
		t.Errorf("battlepassAutoGrant(true) = false, want true")
	}
}
