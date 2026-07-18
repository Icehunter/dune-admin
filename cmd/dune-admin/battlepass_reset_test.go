package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// resetTestDeps returns battlepassDeps with one offline player (account 77,
// level 10) that satisfies any level<=10 tier.
func resetTestDeps() battlepassDeps {
	return battlepassDeps{
		fetchPlayers: func(_ context.Context) ([]battlepassPlayer, error) {
			return []battlepassPlayer{{AccountID: 77, PawnID: 1, Level: 10, Online: false}}, nil
		},
		fetchCompletedJourneyNodes: func(_ context.Context, _ int64) ([]string, error) { return nil, nil },
		fetchPlayerTags:            func(_ context.Context, _ int64) ([]string, error) { return nil, nil },
		pace:                       func(_ context.Context, _ time.Duration) error { return nil },
	}
}

func seedResetTier(t *testing.T, s *battlepassStore) {
	t.Helper()
	if _, err := s.createTier(battlepassTier{
		TierKey: "lvl5", Category: "level", Label: "Level 5",
		Signal: battlepassSignalLevel, Threshold: 5, Intel: 100, Enabled: true,
	}); err != nil {
		t.Fatalf("createTier: %v", err)
	}
}

// TestBattlepassResetDemote_NoRegrantAfterReenable is the core safety property
// of the #293 cleanup: after a demote reset, re-enabling the engine with
// auto-grant on must NOT re-enqueue or re-earn anything — the demoted claims
// stay baseline and block re-earning.
func TestBattlepassResetDemote_NoRegrantAfterReenable(t *testing.T) {
	s := openMemBattlepassStore(t)
	seedResetTier(t, s)
	if err := s.recordClaim("lvl5", 77, 100, battlepassClaimEarned); err != nil {
		t.Fatalf("recordClaim: %v", err)
	}
	if err := s.recordPendingGrant("lvl5", 77); err != nil {
		t.Fatalf("recordPendingGrant: %v", err)
	}

	if _, err := s.deleteUnsettledGrantLedger(0); err != nil {
		t.Fatalf("deleteUnsettledGrantLedger: %v", err)
	}
	demoted, err := s.demoteEarnedClaims(0)
	if err != nil {
		t.Fatalf("demoteEarnedClaims: %v", err)
	}
	if demoted != 1 {
		t.Fatalf("demoted = %d, want 1", demoted)
	}

	// Re-enable: evaluate with auto-grant on. Nothing may become earned or
	// enqueued — the tier is claimed (baseline) and retroactive enqueue only
	// touches earned claims.
	if err := evaluateBattlepassTick(context.Background(), resetTestDeps(), s, false, true, 0); err != nil {
		t.Fatalf("evaluateBattlepassTick: %v", err)
	}
	if due, _ := s.listRetryableGrantLedger(time.Now().Add(time.Hour)); len(due) != 0 {
		t.Fatalf("want 0 pending grants after demote+re-enable, got %d", len(due))
	}
	claims, _ := s.listClaims(77)
	if len(claims) != 1 || claims[0].Status != battlepassClaimBaseline {
		t.Fatalf("claims = %+v, want single baseline claim", claims)
	}
}

// TestBattlepassResetDemote_KeepsGrantedHistory: granted claims are delivery
// history and must survive a demote untouched.
func TestBattlepassResetDemote_KeepsGrantedHistory(t *testing.T) {
	s := openMemBattlepassStore(t)
	seedResetTier(t, s)
	if err := s.recordClaim("lvl5", 77, 100, battlepassClaimGranted); err != nil {
		t.Fatalf("recordClaim: %v", err)
	}

	demoted, err := s.demoteEarnedClaims(0)
	if err != nil {
		t.Fatalf("demoteEarnedClaims: %v", err)
	}
	if demoted != 0 {
		t.Fatalf("demoted = %d, want 0 (granted is history)", demoted)
	}
	claims, _ := s.listClaims(77)
	if len(claims) != 1 || claims[0].Status != battlepassClaimGranted {
		t.Fatalf("claims = %+v, want granted claim preserved", claims)
	}
}

// TestBattlepassResetDemote_ScopedToAccount: a per-account demote must not
// touch other accounts' earned claims.
func TestBattlepassResetDemote_ScopedToAccount(t *testing.T) {
	s := openMemBattlepassStore(t)
	seedResetTier(t, s)
	for _, acct := range []int64{77, 88} {
		if err := s.recordClaim("lvl5", acct, 100, battlepassClaimEarned); err != nil {
			t.Fatalf("recordClaim %d: %v", acct, err)
		}
	}

	demoted, err := s.demoteEarnedClaims(77)
	if err != nil {
		t.Fatalf("demoteEarnedClaims: %v", err)
	}
	if demoted != 1 {
		t.Fatalf("demoted = %d, want 1", demoted)
	}
	if claims, _ := s.listClaims(77); claims[0].Status != battlepassClaimBaseline {
		t.Errorf("account 77 claim = %s, want baseline", claims[0].Status)
	}
	if claims, _ := s.listClaims(88); claims[0].Status != battlepassClaimEarned {
		t.Errorf("account 88 claim = %s, want earned (untouched)", claims[0].Status)
	}
}

// TestBattlepassResetPurge_RebaselinesNotEarns: a purge deletes claims AND the
// tier_seen markers together, so a still-satisfied tier re-baselines on the
// next scan instead of re-earning (which would be a re-grant storm).
func TestBattlepassResetPurge_RebaselinesNotEarns(t *testing.T) {
	s := openMemBattlepassStore(t)
	seedResetTier(t, s)
	if err := s.recordClaim("lvl5", 77, 100, battlepassClaimEarned); err != nil {
		t.Fatalf("recordClaim: %v", err)
	}
	if err := s.recordPendingGrant("lvl5", 77); err != nil {
		t.Fatalf("recordPendingGrant: %v", err)
	}
	// The engine has previously watched this tier unsatisfied — without the
	// paired tier_seen deletion this would make the tier re-earn after purge.
	if err := s.markSeenUnsatisfied("lvl5", 77); err != nil {
		t.Fatalf("markSeenUnsatisfied: %v", err)
	}

	if _, err := s.deleteUnsettledGrantLedger(0); err != nil {
		t.Fatalf("deleteUnsettledGrantLedger: %v", err)
	}
	if _, err := s.purgeClaims(0); err != nil {
		t.Fatalf("purgeClaims: %v", err)
	}
	if _, err := s.purgeTierSeen(0); err != nil {
		t.Fatalf("purgeTierSeen: %v", err)
	}

	if err := evaluateBattlepassTick(context.Background(), resetTestDeps(), s, false, true, 0); err != nil {
		t.Fatalf("evaluateBattlepassTick: %v", err)
	}
	claims, _ := s.listClaims(77)
	if len(claims) != 1 || claims[0].Status != battlepassClaimBaseline {
		t.Fatalf("claims = %+v, want single baseline claim after purge", claims)
	}
	if due, _ := s.listRetryableGrantLedger(time.Now().Add(time.Hour)); len(due) != 0 {
		t.Fatalf("want 0 pending grants after purge+re-scan, got %d", len(due))
	}
}

// TestBattlepassPurgeClaimsAloneReearns documents WHY the reset handler must
// always pair purgeClaims with purgeTierSeen: deleting claims while keeping
// the seen-unsatisfied markers makes every still-satisfied tier re-earn (and
// auto-grant re-deliver) on the very next scan.
func TestBattlepassPurgeClaimsAloneReearns(t *testing.T) {
	s := openMemBattlepassStore(t)
	seedResetTier(t, s)
	if err := s.recordClaim("lvl5", 77, 100, battlepassClaimEarned); err != nil {
		t.Fatalf("recordClaim: %v", err)
	}
	if err := s.markSeenUnsatisfied("lvl5", 77); err != nil {
		t.Fatalf("markSeenUnsatisfied: %v", err)
	}

	if _, err := s.purgeClaims(0); err != nil {
		t.Fatalf("purgeClaims: %v", err)
	}
	// tier_seen deliberately NOT purged — this is the hazard.

	if err := evaluateBattlepassTick(context.Background(), resetTestDeps(), s, false, true, 0); err != nil {
		t.Fatalf("evaluateBattlepassTick: %v", err)
	}
	claims, _ := s.listClaims(77)
	if len(claims) != 1 || claims[0].Status != battlepassClaimEarned {
		t.Fatalf("claims = %+v — expected the documented re-earn hazard (earned)", claims)
	}
}

// TestDeleteUnsettledGrantLedger_KeepsGranted: only pending/exhausted rows are
// deleted; granted rows are delivery history.
func TestDeleteUnsettledGrantLedger_KeepsGranted(t *testing.T) {
	s := openMemBattlepassStore(t)
	seedResetTier(t, s)
	if err := s.recordPendingGrant("lvl5", 77); err != nil {
		t.Fatalf("recordPendingGrant: %v", err)
	}
	if err := s.recordGrantLedgerSuccess("lvl5", 88); err != nil {
		t.Fatalf("recordGrantLedgerSuccess: %v", err)
	}

	deleted, err := s.deleteUnsettledGrantLedger(0)
	if err != nil {
		t.Fatalf("deleteUnsettledGrantLedger: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if status, _ := s.grantLedgerStatus("lvl5", 88); status != "granted" {
		t.Errorf("granted row status = %q, want granted (kept)", status)
	}
	if status, _ := s.grantLedgerStatus("lvl5", 77); status != "" {
		t.Errorf("pending row status = %q, want deleted", status)
	}
}

// ── handler ───────────────────────────────────────────────────────────────────

func TestHandleBattlepassResetClaims_Validation(t *testing.T) {
	setupBattlepassStore(t)
	cases := []struct {
		name string
		body string
	}{
		{"missing mode", `{}`},
		{"unknown mode", `{"mode": "nuke"}`},
		{"malformed json", `{`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/claims/reset", bytes.NewReader([]byte(c.body)))
			rec := httptest.NewRecorder()
			handleBattlepassResetClaims(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleBattlepassResetClaims_NilStore(t *testing.T) {
	globalBattlepassStore = nil
	req := httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/claims/reset",
		bytes.NewReader([]byte(`{"mode":"demote"}`)))
	rec := httptest.NewRecorder()
	handleBattlepassResetClaims(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
}

func TestHandleBattlepassResetClaims_DemoteFlow(t *testing.T) {
	s := setupBattlepassStore(t)
	seedResetTier(t, s)
	if err := s.recordClaim("lvl5", 77, 100, battlepassClaimEarned); err != nil {
		t.Fatalf("recordClaim: %v", err)
	}
	if err := s.recordPendingGrant("lvl5", 77); err != nil {
		t.Fatalf("recordPendingGrant: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/claims/reset",
		bytes.NewReader([]byte(`{"mode":"demote"}`)))
	rec := httptest.NewRecorder()
	handleBattlepassResetClaims(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	claims, _ := s.listClaims(77)
	if len(claims) != 1 || claims[0].Status != battlepassClaimBaseline {
		t.Fatalf("claims = %+v, want baseline after demote", claims)
	}
	if due, _ := s.listRetryableGrantLedger(time.Now().Add(time.Hour)); len(due) != 0 {
		t.Fatalf("want empty ledger after demote, got %d", len(due))
	}
}

func TestHandleBattlepassResetClaims_PurgeFlow(t *testing.T) {
	s := setupBattlepassStore(t)
	seedResetTier(t, s)
	if err := s.recordClaim("lvl5", 77, 100, battlepassClaimGranted); err != nil {
		t.Fatalf("recordClaim: %v", err)
	}
	if err := s.markSeenUnsatisfied("lvl5", 77); err != nil {
		t.Fatalf("markSeenUnsatisfied: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/claims/reset",
		bytes.NewReader([]byte(`{"mode":"purge","account_id":77}`)))
	rec := httptest.NewRecorder()
	handleBattlepassResetClaims(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if claims, _ := s.listClaims(77); len(claims) != 0 {
		t.Fatalf("claims = %+v, want none after purge", claims)
	}
	seen, _ := s.seenUnsatisfiedKeys(77)
	if len(seen) != 0 {
		t.Fatalf("tier_seen = %v, want purged with claims", seen)
	}
}
