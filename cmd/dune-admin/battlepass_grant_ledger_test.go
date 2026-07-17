package main

import (
	"testing"
	"time"
)

func openMemBattlepassStore(t *testing.T) *battlepassStore {
	t.Helper()
	s, err := openBattlepassStore(":memory:")
	if err != nil {
		t.Fatalf("openBattlepassStore: %v", err)
	}
	t.Cleanup(func() { _ = s.db.Close() })
	return s
}

// exhaustLedgerRow drives a row to exhausted with the given error message.
func exhaustLedgerRow(t *testing.T, s *battlepassStore, tierKey string, accountID int64, errMsg string) {
	t.Helper()
	for range deferredGrantMaxAttempts {
		if err := s.recordGrantLedgerFailure(tierKey, accountID, errMsg); err != nil {
			t.Fatalf("recordGrantLedgerFailure: %v", err)
		}
	}
}

// TestHealExhaustedOnlineGrantLedger covers the #259/#280 self-heal: rows
// exhausted by the PRE-fix policy (online failures counted as attempts) must
// be reset to retryable, while genuinely-failed exhausted rows, granted rows,
// and pending rows stay untouched. Post-fix, online failures never reach the
// exhaustion path, so the heal is naturally idempotent.
func TestHealExhaustedOnlineGrantLedger(t *testing.T) {
	s := openMemBattlepassStore(t)
	onlineMsg := "award intel: " + (&playerOnlineError{status: "Online"}).Error()

	exhaustLedgerRow(t, s, "tier_online", 1, onlineMsg)
	exhaustLedgerRow(t, s, "tier_real", 2, "give item: connection refused")
	if err := s.recordPendingGrant("tier_pending", 3); err != nil {
		t.Fatalf("recordPendingGrant: %v", err)
	}
	if err := s.recordGrantLedgerSuccess("tier_granted", 4); err != nil {
		t.Fatalf("recordGrantLedgerSuccess: %v", err)
	}

	healed, err := s.healExhaustedOnlineGrantLedger()
	if err != nil {
		t.Fatalf("heal: %v", err)
	}
	if healed != 1 {
		t.Fatalf("healed = %d, want 1 (only the online-exhausted row)", healed)
	}

	due, err := s.listRetryableGrantLedger(time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("listRetryableGrantLedger: %v", err)
	}
	keys := map[string]battlepassGrantLedgerRow{}
	for _, r := range due {
		keys[r.TierKey] = r
	}
	healedRow, ok := keys["tier_online"]
	if !ok {
		t.Fatalf("healed row not retryable; due = %v", keys)
	}
	if healedRow.Attempts != 0 || healedRow.Status != battlepassGrantPending {
		t.Fatalf("healed row = %+v, want pending/0 attempts", healedRow)
	}
	if _, ok := keys["tier_real"]; ok {
		t.Fatal("genuinely-exhausted row must stay exhausted")
	}

	// Idempotent: a second heal touches nothing.
	healed, err = s.healExhaustedOnlineGrantLedger()
	if err != nil {
		t.Fatalf("second heal: %v", err)
	}
	if healed != 0 {
		t.Fatalf("second heal healed = %d, want 0", healed)
	}
}

// TestHealExhaustedOnlineGrantLedger_AllServerScopes — the heal runs once at
// startup on the shared handle and must repair every server's rows, not just
// the default scope.
func TestHealExhaustedOnlineGrantLedger_AllServerScopes(t *testing.T) {
	s := openMemBattlepassStore(t)
	onlineMsg := (&playerOnlineError{status: "Online"}).Error()

	other := s.withScope(2)
	exhaustLedgerRow(t, other, "tier_online", 9, onlineMsg)

	healed, err := s.healExhaustedOnlineGrantLedger()
	if err != nil {
		t.Fatalf("heal: %v", err)
	}
	if healed != 1 {
		t.Fatalf("healed = %d, want 1 (row on scope 2)", healed)
	}
	due, err := other.listRetryableGrantLedger(time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("list scope 2: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("scope 2 due = %d, want 1", len(due))
	}
}

func TestBattlepassGrantLedger_RecordPending(t *testing.T) {
	s := openMemBattlepassStore(t)
	if err := s.recordPendingGrant("tier_a", 100); err != nil {
		t.Fatalf("recordPendingGrant: %v", err)
	}
	// Idempotent: a second record for the same key must not duplicate or reset.
	if err := s.recordPendingGrant("tier_a", 100); err != nil {
		t.Fatalf("recordPendingGrant again: %v", err)
	}

	due, err := s.listRetryableGrantLedger(time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("listRetryableGrantLedger: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("want 1 pending ledger row, got %d", len(due))
	}
	if due[0].TierKey != "tier_a" || due[0].AccountID != 100 {
		t.Fatalf("ledger row = %+v, want tier_a/100", due[0])
	}
	if due[0].Attempts != 0 {
		t.Errorf("attempts = %d, want 0 on fresh pending", due[0].Attempts)
	}
}

func TestBattlepassGrantLedger_FailureIncrementsAndBacksOff(t *testing.T) {
	s := openMemBattlepassStore(t)
	if err := s.recordPendingGrant("tier_b", 200); err != nil {
		t.Fatalf("recordPendingGrant: %v", err)
	}
	if err := s.recordGrantLedgerFailure("tier_b", 200, "inventory full"); err != nil {
		t.Fatalf("recordGrantLedgerFailure: %v", err)
	}

	// Not due immediately after a failure (next_attempt_at is in the future).
	dueNow, err := s.listRetryableGrantLedger(time.Now())
	if err != nil {
		t.Fatalf("listRetryableGrantLedger now: %v", err)
	}
	if len(dueNow) != 0 {
		t.Fatalf("want 0 due immediately after failure, got %d", len(dueNow))
	}

	// Due once the backoff window elapses.
	dueLater, err := s.listRetryableGrantLedger(time.Now().Add(deferredGrantRetryBackoff + time.Hour))
	if err != nil {
		t.Fatalf("listRetryableGrantLedger later: %v", err)
	}
	if len(dueLater) != 1 {
		t.Fatalf("want 1 due after backoff, got %d", len(dueLater))
	}
	if dueLater[0].Attempts != 1 {
		t.Errorf("attempts = %d, want 1", dueLater[0].Attempts)
	}
	if dueLater[0].LastError != "inventory full" {
		t.Errorf("last_error = %q, want inventory full", dueLater[0].LastError)
	}
}

func TestBattlepassGrantLedger_ExhaustsAfterMaxAttempts(t *testing.T) {
	s := openMemBattlepassStore(t)
	if err := s.recordPendingGrant("tier_c", 300); err != nil {
		t.Fatalf("recordPendingGrant: %v", err)
	}
	for i := range deferredGrantMaxAttempts {
		if err := s.recordGrantLedgerFailure("tier_c", 300, "boom"); err != nil {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
	// After max attempts the row is exhausted and never retryable again.
	due, err := s.listRetryableGrantLedger(time.Now().Add(100 * deferredGrantRetryBackoff))
	if err != nil {
		t.Fatalf("listRetryableGrantLedger: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("want 0 retryable after exhaustion, got %d", len(due))
	}
}

func TestBattlepassGrantLedger_SuccessTerminal(t *testing.T) {
	s := openMemBattlepassStore(t)
	if err := s.recordPendingGrant("tier_d", 400); err != nil {
		t.Fatalf("recordPendingGrant: %v", err)
	}
	if err := s.recordGrantLedgerSuccess("tier_d", 400); err != nil {
		t.Fatalf("recordGrantLedgerSuccess: %v", err)
	}
	due, err := s.listRetryableGrantLedger(time.Now().Add(100 * deferredGrantRetryBackoff))
	if err != nil {
		t.Fatalf("listRetryableGrantLedger: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("want 0 retryable after grant, got %d", len(due))
	}
}

// TestBattlepassGrantLedger_RetryLaterDoesNotConsumeAttempt covers the #259/
// #280 fix: a "player is online" delivery failure is an expected, frequent
// condition (not a real failure), so it must retry on a short backoff without
// ever counting toward deferredGrantMaxAttempts. Before this fix, an online
// failure used the same 24h/3-attempt policy as a genuine error, so a player
// who stayed online for hours (explicitly reported) would exhaust the ledger
// row long before they ever logged out, leaving the tier grantable only by
// hand.
func TestBattlepassGrantLedger_RetryLaterDoesNotConsumeAttempt(t *testing.T) {
	s := openMemBattlepassStore(t)
	if err := s.recordPendingGrant("tier_online", 600); err != nil {
		t.Fatalf("recordPendingGrant: %v", err)
	}

	shortBackoff := 5 * time.Minute
	// Retry-later many more times than deferredGrantMaxAttempts would allow —
	// the row must never exhaust from this alone.
	for i := range deferredGrantMaxAttempts + 5 {
		if err := s.recordGrantLedgerRetryLater("tier_online", 600, "player is online", shortBackoff); err != nil {
			t.Fatalf("recordGrantLedgerRetryLater %d: %v", i, err)
		}
	}

	// Not due immediately — the short backoff is still in effect.
	dueNow, err := s.listRetryableGrantLedger(time.Now())
	if err != nil {
		t.Fatalf("listRetryableGrantLedger now: %v", err)
	}
	if len(dueNow) != 0 {
		t.Fatalf("want 0 due immediately, got %d", len(dueNow))
	}

	// Due once the short backoff elapses — and crucially still pending with
	// attempts untouched, unlike a real failure which would have exhausted by now.
	dueLater, err := s.listRetryableGrantLedger(time.Now().Add(shortBackoff + time.Minute))
	if err != nil {
		t.Fatalf("listRetryableGrantLedger later: %v", err)
	}
	if len(dueLater) != 1 {
		t.Fatalf("want 1 due after short backoff (never exhausted), got %d", len(dueLater))
	}
	if dueLater[0].Attempts != 0 {
		t.Errorf("attempts = %d, want 0 — retry-later must not consume an attempt", dueLater[0].Attempts)
	}
	if dueLater[0].Status != battlepassGrantPending {
		t.Errorf("status = %q, want pending", dueLater[0].Status)
	}
	if dueLater[0].LastError != "player is online" {
		t.Errorf("last_error = %q, want %q", dueLater[0].LastError, "player is online")
	}
}

func TestBattlepassGrantLedger_RetryLaterPreservesExistingAttemptsOnConflict(t *testing.T) {
	s := openMemBattlepassStore(t)
	if err := s.recordPendingGrant("tier_mixed", 700); err != nil {
		t.Fatalf("recordPendingGrant: %v", err)
	}
	// One genuine failure first (attempts -> 1)...
	if err := s.recordGrantLedgerFailure("tier_mixed", 700, "db hiccup"); err != nil {
		t.Fatalf("recordGrantLedgerFailure: %v", err)
	}
	// ...then an online retry-later must not touch that attempt count.
	if err := s.recordGrantLedgerRetryLater("tier_mixed", 700, "player is online", time.Minute); err != nil {
		t.Fatalf("recordGrantLedgerRetryLater: %v", err)
	}
	due, err := s.listRetryableGrantLedger(time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("listRetryableGrantLedger: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("want 1 due row, got %d", len(due))
	}
	if due[0].Attempts != 1 {
		t.Errorf("attempts = %d, want 1 (preserved from the earlier genuine failure)", due[0].Attempts)
	}
}

func TestBattlepassGrantLedger_OnlyDueClaimsListed(t *testing.T) {
	s := openMemBattlepassStore(t)
	// Two pending rows; one fresh (due now), one just-failed (not yet due).
	if err := s.recordPendingGrant("tier_due", 500); err != nil {
		t.Fatalf("recordPendingGrant due: %v", err)
	}
	if err := s.recordPendingGrant("tier_backoff", 500); err != nil {
		t.Fatalf("recordPendingGrant backoff: %v", err)
	}
	if err := s.recordGrantLedgerFailure("tier_backoff", 500, "later"); err != nil {
		t.Fatalf("recordGrantLedgerFailure: %v", err)
	}
	due, err := s.listRetryableGrantLedger(time.Now())
	if err != nil {
		t.Fatalf("listRetryableGrantLedger: %v", err)
	}
	if len(due) != 1 || due[0].TierKey != "tier_due" {
		t.Fatalf("want only tier_due, got %+v", due)
	}
}
