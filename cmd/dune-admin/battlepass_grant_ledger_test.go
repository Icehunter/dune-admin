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
	for i := 0; i < deferredGrantMaxAttempts; i++ {
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
