package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// battlepass_grant_engine.go implements battlepass auto-grant (#197): the
// feature-specific delivery + ledger recording that the shared deferred-grant
// core (deferred_grant.go) drives. Delivery reuses the manual-grant primitives
// (awardIntel / giveItem) but resolves the target offline-capably and records
// each attempt on battlepass_grant_ledger so transient failures retry with
// backoff and eventually exhaust to manual-only.

// attemptBattlepassGrant delivers one earned tier's reward (intel + items) to an
// account — online or offline — and records the outcome on the grant ledger. On
// success it also flips the battlepass_claims row to granted so the manual
// pending view stays consistent. Returns an error only when delivery fails (the
// failure is recorded on the ledger first so backoff applies).
func attemptBattlepassGrant(ctx context.Context, store *battlepassStore, deps battlepassGrantDeps, tierKey string, accountID int64) error {
	tier, err := store.getTierByKey(tierKey)
	if err != nil {
		// A missing tier cannot be auto-granted; record the failure so it
		// backs off rather than spinning every tick.
		recordBattlepassGrantFailure(store, tierKey, accountID, err)
		return fmt.Errorf("resolve tier %q: %w", tierKey, err)
	}

	pawnID, err := deps.resolveGrantTarget(ctx, accountID)
	if err != nil {
		recordBattlepassGrantFailure(store, tierKey, accountID, err)
		return fmt.Errorf("resolve grant target %d: %w", accountID, err)
	}

	if err := deps.awardIntel(ctx, pawnID, tier.Intel); err != nil {
		recordBattlepassGrantFailure(store, tierKey, accountID, err)
		return fmt.Errorf("award intel %d/%s: %w", accountID, tierKey, err)
	}

	// Intel landed: flip the claim and seal the ledger before delivering items.
	// Items are best-effort (a failure here must not re-pay intel on retry).
	if err := store.markGrantedForTier(accountID, tierKey); err != nil {
		log.Printf("battlepass: intel granted but claim update failed %d/%s: %v", accountID, tierKey, err)
	}
	if err := store.recordGrantLedgerSuccess(tierKey, accountID); err != nil {
		log.Printf("battlepass: record grant success %d/%s: %v", accountID, tierKey, err)
	}
	deliverBattlepassTierItems(ctx, deps, tier, pawnID)
	return nil
}

// recordBattlepassGrantFailure records a failed attempt on the ledger, logging
// (but not surfacing) a ledger write error.
func recordBattlepassGrantFailure(store *battlepassStore, tierKey string, accountID int64, cause error) {
	if recErr := store.recordGrantLedgerFailure(tierKey, accountID, cause.Error()); recErr != nil {
		log.Printf("battlepass: record grant failure %d/%s: %v", accountID, tierKey, recErr)
	}
}

// deliverBattlepassTierItems grants a tier's item rewards (if any). Item
// failures are logged, not retried — the intel grant is already terminal.
func deliverBattlepassTierItems(ctx context.Context, deps battlepassGrantDeps, tier *battlepassTier, pawnID int64) {
	if tier.RewardItems == "" {
		return
	}
	var items []rewardItem
	if err := json.Unmarshal([]byte(tier.RewardItems), &items); err != nil {
		log.Printf("battlepass: tier %s reward_items: %v", tier.TierKey, err)
		return
	}
	for _, item := range items {
		if err := deps.giveItem(ctx, pawnID, item.Template, item.Qty, item.Quality); err != nil {
			log.Printf("battlepass: give item %q for tier %s: %v", item.Template, tier.TierKey, err)
		}
	}
}

// battlepassGrantSource adapts a battlepassStore to the shared
// deferredGrantSource contract for the auto-grant retry loop.
type battlepassGrantSource struct {
	store *battlepassStore
	deps  battlepassGrantDeps
	// pending maps owner (account) ID → tier_key for the current tick so the
	// attempt closure can recover the tier the generic deferredClaim drops.
	pending map[int64]string
}

func newBattlepassGrantSource(store *battlepassStore, deps battlepassGrantDeps) *battlepassGrantSource {
	return &battlepassGrantSource{store: store, deps: deps, pending: map[int64]string{}}
}

func (s *battlepassGrantSource) listRetryableDeferredClaims(now time.Time) ([]deferredClaim, error) {
	rows, err := s.store.listRetryableGrantLedger(now)
	if err != nil {
		return nil, err
	}
	out := make([]deferredClaim, 0, len(rows))
	s.pending = make(map[int64]string, len(rows))
	for _, r := range rows {
		// One account may have several due tiers; the closure resolves them in
		// order via the per-claim tier key carried below.
		out = append(out, deferredClaim{OwnerID: r.AccountID, Attempts: r.Attempts})
		s.pending[r.AccountID] = r.TierKey
	}
	return out, nil
}

func (s *battlepassGrantSource) attempt(ctx context.Context, dc deferredClaim) error {
	tierKey, ok := s.pending[dc.OwnerID]
	if !ok {
		return fmt.Errorf("battlepass grant for account %d not found in tick", dc.OwnerID)
	}
	return attemptBattlepassGrant(ctx, s.store, s.deps, tierKey, dc.OwnerID)
}

// battlepassAutoGrant returns the effective auto-grant flag (default off).
func battlepassAutoGrant(cfg appConfig) bool {
	if cfg.BattlepassAutoGrant == nil {
		return false
	}
	return *cfg.BattlepassAutoGrant
}

// runBattlepassGrantLoop drives the auto-grant retry loop via the shared core.
// Ctx-cancellable; a no-op when the store is nil.
func runBattlepassGrantLoop(ctx context.Context, store *battlepassStore, deps battlepassGrantDeps) {
	if store == nil {
		runDeferredGrantLoop(ctx, nil, nil)
		return
	}
	src := newBattlepassGrantSource(store, deps)
	runDeferredGrantLoop(ctx, src, src.attempt)
}
