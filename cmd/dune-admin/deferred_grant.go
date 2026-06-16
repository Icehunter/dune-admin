package main

import (
	"context"
	"time"
)

// deferred_grant.go holds the feature-agnostic retry-scheduling core shared by
// the events engine (#198) and the battlepass auto-grant engine (#197). Each
// feature keeps its own reward-specific delivery and ledger SQL; only the loop,
// the backoff policy, and the small "list due claims → attempt each" contract
// are shared here.

const (
	// deferredGrantMaxAttempts is the number of grant attempts allowed before a
	// claim is considered exhausted (manual-grant-only).
	deferredGrantMaxAttempts = 3

	// deferredGrantRetryBackoff is the delay added before the next automatic
	// retry after a failed attempt.
	deferredGrantRetryBackoff = 24 * time.Hour

	// deferredGrantRetryInterval is how often the loop scans for due claims.
	// Coarse on purpose: backoff is measured in days, so a 1-minute tick is
	// plenty.
	deferredGrantRetryInterval = time.Minute
)

// deferredClaim is the feature-agnostic view of one retryable claim that the
// shared loop carries to the per-feature attempt closure for logging context.
// Feature attempt closures hold their own typed claim, so only the owner ID and
// attempt count are needed here.
type deferredClaim struct {
	// OwnerID identifies the grant recipient (account ID for both events and
	// battlepass).
	OwnerID int64
	// Attempts is how many grant attempts have already been made.
	Attempts int
}

// deferredGrantSource exposes the due-claim query the shared loop needs. Each
// feature's store implements it over its own ledger table.
type deferredGrantSource interface {
	// listRetryableDeferredClaims returns the claims whose backoff window has
	// elapsed (next_attempt_at <= now) and which still have attempts remaining.
	listRetryableDeferredClaims(now time.Time) ([]deferredClaim, error)
}

// deferredAttemptFunc delivers one claim's reward and records success/failure on
// the feature's ledger. It returns an error only when the attempt fails.
type deferredAttemptFunc func(ctx context.Context, claim deferredClaim) error

// runDeferredGrantLoop periodically retries due claims. It is ctx-cancellable
// and a no-op when the source is nil (feature disabled).
func runDeferredGrantLoop(ctx context.Context, src deferredGrantSource, attempt deferredAttemptFunc) {
	if src == nil {
		return
	}
	ticker := time.NewTicker(deferredGrantRetryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			processDeferredGrantTick(ctx, src, attempt, now)
		}
	}
}

// processDeferredGrantTick attempts every claim currently due for retry. A
// listing error skips the tick; per-claim failures are logged and do not stop
// the remaining claims.
func processDeferredGrantTick(ctx context.Context, src deferredGrantSource, attempt deferredAttemptFunc, now time.Time) {
	claims, err := src.listRetryableDeferredClaims(now)
	if err != nil {
		componentLog("deferred_grant").Error().Err(err).Msg("list retryable claims failed")
		return
	}
	for _, claim := range claims {
		if err := attempt(ctx, claim); err != nil {
			componentLog("deferred_grant").Warn().Err(err).Int64("account_id", claim.OwnerID).Int("attempt", claim.Attempts).Msg("retry failed")
		}
	}
}
