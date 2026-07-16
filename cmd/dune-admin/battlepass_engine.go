package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// globalBattlepassCancel stops the running battlepass engine goroutine.
// Protected by globalBattlepassMu; nil when the engine is not running.
var globalBattlepassCancel context.CancelFunc
var globalBattlepassMu sync.Mutex

// battlepassPlayer is the per-character snapshot the battlepass engine needs.
// Level is derived in the bulk fetch so evaluation makes no per-player level
// queries.
type battlepassPlayer struct {
	AccountID int64
	PawnID    int64
	Name      string
	Online    bool
	Level     int
}

// battlepassDeps holds injectable functions so the engine can be unit-tested
// without a live DB (pattern: eventDeps).
type battlepassDeps struct {
	fetchPlayers               func(ctx context.Context) ([]battlepassPlayer, error)
	fetchCompletedJourneyNodes func(ctx context.Context, accountID int64) ([]string, error)
	fetchPlayerTags            func(ctx context.Context, accountID int64) ([]string, error)
	// pace is a context-aware delay injected between players in the evaluation
	// loop so a scan does not burst Postgres. Production uses a timer-based
	// sleep; tests inject a no-op or recording stub.
	pace func(ctx context.Context, d time.Duration) error
}

// globalBattlepassStore is set once at startup, guarded in every handler.
var globalBattlepassStore *battlepassStore

// battlepassTierSatisfied reports whether the player state meets the tier's
// signal condition.
func battlepassTierSatisfied(t battlepassTier, level int, journey, tags map[string]bool) bool {
	switch t.Signal {
	case battlepassSignalLevel:
		return int64(level) >= t.Threshold
	case battlepassSignalJourneyNode:
		return journey[t.SignalKey]
	case battlepassSignalPlayerTag:
		return tags[t.SignalKey]
	default:
		return false
	}
}

// battlepassUnclaimed filters tiers to enabled ones the account has no claim
// for, and reports which signals the remaining tiers need.
func battlepassUnclaimed(tiers []battlepassTier, claimed map[string]string) (unclaimed []battlepassTier, needsJourney, needsTags bool) {
	for _, t := range tiers {
		if !t.Enabled {
			continue
		}
		if _, ok := claimed[t.TierKey]; ok {
			continue
		}
		unclaimed = append(unclaimed, t)
		switch t.Signal {
		case battlepassSignalJourneyNode:
			needsJourney = true
		case battlepassSignalPlayerTag:
			needsTags = true
		}
	}
	return unclaimed, needsJourney, needsTags
}

// battlepassRetroactiveGrantTiers returns tier keys already recorded as
// earned (not baseline, not already granted) that should be (re-)enqueued on
// the grant ledger. Needed because enabling auto-grant is not retroactive on
// its own: battlepassUnclaimed skips any tier already present in `claimed`,
// so a tier earned before auto-grant was turned on would otherwise never get
// a ledger row and sit in Pending forever, grantable only by hand (#259/#280).
func battlepassRetroactiveGrantTiers(claimed map[string]string) []string {
	var out []string
	for tierKey, status := range claimed {
		if status == battlepassClaimEarned {
			out = append(out, tierKey)
		}
	}
	return out
}

// enqueueRetroactiveBattlepassGrants (re-)enqueues a pending grant-ledger row
// for every already-earned tier in `claimed`, when auto-grant is on.
// recordPendingGrant is idempotent (ON CONFLICT DO NOTHING), so calling this
// every tick is safe and self-healing — it only ever inserts once per
// tier/account, and covers tiers earned before auto-grant was turned on
// (#259/#280).
func enqueueRetroactiveBattlepassGrants(store *battlepassStore, claimed map[string]string, accountID int64, autoGrant bool) error {
	if !autoGrant {
		return nil
	}
	for _, tierKey := range battlepassRetroactiveGrantTiers(claimed) {
		if err := store.recordPendingGrant(tierKey, accountID); err != nil {
			return fmt.Errorf("record pending grant %s (retroactive): %w", tierKey, err)
		}
	}
	return nil
}

// evaluateBattlepassPlayer records claims for every newly-satisfied tier.
// During the account's first complete pass (and unless awardPast is set),
// satisfied tiers are recorded as baseline — pre-existing progress is not
// rewarded; the pass pays for new unlocks only.
func evaluateBattlepassPlayer(ctx context.Context, deps battlepassDeps, store *battlepassStore, tiers []battlepassTier, p battlepassPlayer, awardPast, autoGrant bool) error {
	claimed, err := store.claimedKeys(p.AccountID)
	if err != nil {
		return fmt.Errorf("claimed keys: %w", err)
	}
	unclaimed, needsJourney, needsTags := battlepassUnclaimed(tiers, claimed)

	// Auto-grant may have just been turned on: tiers earned earlier under
	// manual-only mode were never enqueued and never will be via the
	// unclaimed path above, since they're already in `claimed`.
	if err := enqueueRetroactiveBattlepassGrants(store, claimed, p.AccountID, autoGrant); err != nil {
		return err
	}

	baselined := true
	if !awardPast {
		baselined, err = store.isBaselined(p.AccountID)
		if err != nil {
			return fmt.Errorf("baselined check: %w", err)
		}
	}

	if len(unclaimed) > 0 {
		status := battlepassClaimEarned
		if !baselined {
			status = battlepassClaimBaseline
		}
		// Only enqueue auto-grant for genuinely earned tiers — baseline rows are
		// pre-existing progress and never grantable.
		enqueue := autoGrant && status == battlepassClaimEarned
		if err := recordSatisfiedBattlepassTiers(ctx, deps, store, unclaimed, p, status, needsJourney, needsTags, enqueue); err != nil {
			return err
		}
	}

	// Only mark baselined after a fully successful pass — failing earlier
	// keeps old progress eligible for baselining, never for rewards.
	if !baselined {
		if err := store.markBaselined(p.AccountID); err != nil {
			return fmt.Errorf("mark baselined: %w", err)
		}
	}
	return nil
}

// recordSatisfiedBattlepassTiers fetches the needed signal sets and records a
// claim with the given status for every satisfied tier.
func recordSatisfiedBattlepassTiers(ctx context.Context, deps battlepassDeps, store *battlepassStore, tiers []battlepassTier, p battlepassPlayer, status string, needsJourney, needsTags, enqueueGrant bool) error {
	journey, tags, err := fetchBattlepassSignals(ctx, deps, p.AccountID, needsJourney, needsTags)
	if err != nil {
		return err
	}
	for _, t := range tiers {
		if !battlepassTierSatisfied(t, p.Level, journey, tags) {
			continue
		}
		if err := store.recordClaim(t.TierKey, p.AccountID, t.Intel, status); err != nil {
			return fmt.Errorf("record claim %s: %w", t.TierKey, err)
		}
		if enqueueGrant {
			if err := store.recordPendingGrant(t.TierKey, p.AccountID); err != nil {
				return fmt.Errorf("record pending grant %s: %w", t.TierKey, err)
			}
		}
	}
	return nil
}

// fetchBattlepassSignals loads the per-player signal sets that are actually
// needed by unclaimed tiers.
func fetchBattlepassSignals(ctx context.Context, deps battlepassDeps, accountID int64, needsJourney, needsTags bool) (journey, tags map[string]bool, err error) {
	journey, tags = map[string]bool{}, map[string]bool{}
	if needsJourney {
		nodes, err := deps.fetchCompletedJourneyNodes(ctx, accountID)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch journey nodes: %w", err)
		}
		for _, n := range nodes {
			journey[n] = true
		}
	}
	if needsTags {
		tagList, err := deps.fetchPlayerTags(ctx, accountID)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch player tags: %w", err)
		}
		for _, tag := range tagList {
			tags[tag] = true
		}
	}
	return journey, tags, nil
}

// evaluateBattlepassTick evaluates every tracked player against the enabled
// tiers. Per-player failures are logged and skipped so one bad row cannot
// stall the whole pass. paceEvery > 0 inserts a context-aware pause between
// players; a ctx cancellation mid-pace returns the ctx error immediately.
func evaluateBattlepassTick(ctx context.Context, deps battlepassDeps, store *battlepassStore, awardPast, autoGrant bool, paceEvery time.Duration) error {
	tiers, err := store.listTiers()
	if err != nil {
		return fmt.Errorf("battlepass list tiers: %w", err)
	}
	if len(tiers) == 0 {
		return nil
	}
	players, err := deps.fetchPlayers(ctx)
	if err != nil {
		return fmt.Errorf("battlepass fetch players: %w", err)
	}
	for i, p := range players {
		if i > 0 && paceEvery > 0 {
			if err := deps.pace(ctx, paceEvery); err != nil {
				return err // ctx cancelled mid-scan: stop cleanly
			}
		}
		if err := evaluateBattlepassPlayer(ctx, deps, store, tiers, p, awardPast, autoGrant); err != nil {
			componentLog("battlepass").Warn().Int64("account_id", p.AccountID).Err(err).Msg("evaluate player failed")
		}
	}
	return nil
}

// ── polling loop ──────────────────────────────────────────────────────────────

// clampBattlepassInterval converts BattlepassPollSeconds to a Duration,
// defaulting to 60s and clamped [10s, 600s].
func clampBattlepassInterval(secs int) time.Duration {
	if secs < 1 {
		secs = 60
	}
	if secs < 10 {
		secs = 10
	}
	if secs > 600 {
		secs = 600
	}
	return time.Duration(secs) * time.Second
}

// clampBattlepassPaceMs converts BattlepassScanPaceMs to a Duration.
// 0 is preserved (explicit opt-out of pacing); negative → default 75ms; max 5000ms.
func clampBattlepassPaceMs(ms int) time.Duration {
	if ms < 0 {
		ms = 75
	}
	if ms > 5000 {
		ms = 5000
	}
	return time.Duration(ms) * time.Millisecond
}

// clampBattlepassStartDelayMs converts BattlepassScanStartDelayMs to a Duration.
// 0 is preserved (immediate boot scan); negative → default 3000ms; max 30000ms.
func clampBattlepassStartDelayMs(ms int) time.Duration {
	if ms < 0 {
		ms = 3000
	}
	if ms > 30000 {
		ms = 30000
	}
	return time.Duration(ms) * time.Millisecond
}

// bootBattlepassScan runs a single evaluation tick after an optional ctx-aware
// start delay. Called once at engine startup before the ticker loop begins.
// Returns early (without scanning) if ctx is cancelled during the delay.
func bootBattlepassScan(ctx context.Context, deps battlepassDeps, store *battlepassStore, awardPast, autoGrant bool, paceEvery, startDelay time.Duration) {
	if startDelay > 0 {
		if err := deps.pace(ctx, startDelay); err != nil {
			return // cancelled during warm-up
		}
	}
	if err := evaluateBattlepassTick(ctx, deps, store, awardPast, autoGrant, paceEvery); err != nil {
		componentLog("battlepass").Warn().Err(err).Msg("boot scan failed")
	}
}

// runBattlepassEngine runs the evaluation loop until ctx is cancelled.
// It performs an immediate boot scan (after an optional start delay) before
// entering the ticker loop, so the dashboard populates quickly on startup
// without waiting a full poll interval.
func runBattlepassEngine(ctx context.Context, deps battlepassDeps, store *battlepassStore, interval, paceEvery, startDelay time.Duration, awardPast, autoGrant bool) {
	bootBattlepassScan(ctx, deps, store, awardPast, autoGrant, paceEvery, startDelay)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := evaluateBattlepassTick(ctx, deps, store, awardPast, autoGrant, paceEvery); err != nil {
				componentLog("battlepass").Warn().Err(err).Msg("tick failed")
			}
		}
	}
}

// productionBattlepassDeps builds the deps from the given pool. Called from
// applyBattlepassEngine only; tests inject mocks directly.
func productionBattlepassDeps(pool *pgxpool.Pool) battlepassDeps {
	return battlepassDeps{
		fetchPlayers: func(ctx context.Context) ([]battlepassPlayer, error) {
			if pool == nil {
				return nil, fmt.Errorf("database not connected")
			}
			return cmdFetchBattlepassPlayers(ctx, pool)
		},
		fetchCompletedJourneyNodes: func(ctx context.Context, accountID int64) ([]string, error) {
			if pool == nil {
				return nil, fmt.Errorf("database not connected")
			}
			return cmdFetchCompletedJourneyNodeIDs(ctx, pool, accountID)
		},
		fetchPlayerTags: func(ctx context.Context, accountID int64) ([]string, error) {
			if pool == nil {
				return nil, fmt.Errorf("database not connected")
			}
			return cmdFetchPlayerTagsForAccount(ctx, pool, accountID)
		},
		pace: func(ctx context.Context, d time.Duration) error {
			if d <= 0 {
				return nil
			}
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
	}
}

// battlepassEnabled returns the effective enabled flag (default off).
func battlepassEnabled(cfg appConfig) bool {
	if cfg.BattlepassEnabled == nil {
		return false
	}
	return *cfg.BattlepassEnabled
}

// battlepassAwardPast returns the award-past flag (default false: the pass
// rewards new unlocks only; pre-existing progress is baselined unrewarded).
func battlepassAwardPast(cfg appConfig) bool {
	if cfg.BattlepassAwardPast == nil {
		return false
	}
	return *cfg.BattlepassAwardPast
}

// applyBattlepassEngine syncs the catalog, then stops any running battlepass
// engine goroutine and starts a new one if battlepass_enabled is true.
// Safe to call from config save handlers.
func applyBattlepassEngine(cfg appConfig) {
	if globalBattlepassStore == nil {
		return
	}

	// Seed the baked-in catalog ONLY when this server has no tiers yet. The DB is
	// the source of truth: operator edits (via the import/reset API) must survive
	// restarts and config saves, so we must NOT clobber existing tiers here. An
	// explicit "reset to defaults" / import still uses reseedTiers.
	// Skip on a fresh install (no server row) — the FK constraint would reject
	// the INSERT; seeding runs on the next call once a server has been added.
	if !noServerConfigured() {
		catalog := defaultBattlepassCatalog()
		if n, err := globalBattlepassStore.seedTiersIfEmpty(catalog); err != nil {
			componentLog("battlepass").Error().Err(err).Msg("seed catalog failed")
		} else if n > 0 {
			componentLog("battlepass").Info().Int("tier_count", n).Msg("catalog seeded")
		}
	}

	globalBattlepassMu.Lock()
	defer globalBattlepassMu.Unlock()

	if globalBattlepassCancel != nil {
		globalBattlepassCancel()
		globalBattlepassCancel = nil
		componentLog("battlepass").Info().Msg("engine stopped")
	}

	if !battlepassEnabled(cfg) {
		return
	}

	interval := clampBattlepassInterval(cfg.BattlepassPollSeconds)
	paceEvery := clampBattlepassPaceMs(cfg.BattlepassScanPaceMs)
	startDelay := clampBattlepassStartDelayMs(cfg.BattlepassScanStartDelayMs)
	autoGrant := battlepassAutoGrant(cfg)
	componentLog("battlepass").Info().
		Str("interval", interval.String()).
		Str("pace", paceEvery.String()).
		Str("start_delay", startDelay.String()).
		Bool("award_past", battlepassAwardPast(cfg)).
		Bool("auto_grant", autoGrant).
		Msg("engine started")

	ctx, cancel := context.WithCancel(context.Background())
	globalBattlepassCancel = cancel
	for _, sc := range globalRegistry.All() {
		if sc.DB == nil {
			continue
		}
		// Each server's engine writes claims/grants under that server's scope so
		// the same account_id on different servers never collides.
		scoped := globalBattlepassStore.withScope(sc.StoreScope)
		go runBattlepassEngine(ctx, productionBattlepassDeps(sc.DB),
			scoped, interval, paceEvery, startDelay, battlepassAwardPast(cfg), autoGrant)
		if autoGrant {
			go runBattlepassGrantLoop(ctx, scoped, productionBattlepassGrantDeps(sc.DB))
		}
	}
}

// stopBattlepassEngine cancels the running battlepass engine goroutine if any.
func stopBattlepassEngine() {
	globalBattlepassMu.Lock()
	defer globalBattlepassMu.Unlock()
	if globalBattlepassCancel != nil {
		globalBattlepassCancel()
		globalBattlepassCancel = nil
	}
}
