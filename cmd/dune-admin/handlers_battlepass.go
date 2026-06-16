package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// errBattlepassNothingEarned signals a grant request for an account with no
// grantable (earned) claims.
var errBattlepassNothingEarned = errors.New("no earned battlepass rewards to grant")

// battlepassGrantDeps holds the injectable pieces of the grant flow so it can
// be unit-tested without a live DB.
type battlepassGrantDeps struct {
	fetchPlayers func(ctx context.Context) ([]battlepassPlayer, error)
	awardIntel   func(ctx context.Context, pawnID, amount int64) error
	giveItem     func(ctx context.Context, actorID int64, template string, qty, quality int64) error
	// resolveGrantTarget returns a pawn ID for an account without requiring the
	// player to be online — used by the auto-grant retry loop (#197).
	resolveGrantTarget func(ctx context.Context, accountID int64) (pawnID int64, err error)
}

// grantBattlepassEarned sums the account's earned claims, awards the intel in
// one update (the player must be offline), marks the claims granted, then
// delivers any per-tier item rewards. Failed intel awards are recorded on the
// claims, which stay earned for retry; item failures after the intel grant
// are logged but do not re-open the claims (a retry would double-pay intel).
func grantBattlepassEarned(ctx context.Context, store *battlepassStore, deps battlepassGrantDeps, accountID int64) (int64, int, error) {
	earned, err := store.earnedClaims(accountID)
	if err != nil {
		return 0, 0, fmt.Errorf("earned claims: %w", err)
	}
	if len(earned) == 0 {
		return 0, 0, errBattlepassNothingEarned
	}

	players, err := deps.fetchPlayers(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch players: %w", err)
	}
	var pawnID int64
	for _, p := range players {
		if p.AccountID == accountID {
			pawnID = p.PawnID
			break
		}
	}
	if pawnID == 0 {
		return 0, 0, errNotFound
	}

	var total int64
	for _, c := range earned {
		total += c.Intel
	}
	if err := deps.awardIntel(ctx, pawnID, total); err != nil {
		if recErr := store.recordGrantFailure(accountID, err.Error()); recErr != nil {
			componentLog("battlepass").Error().Int64("account_id", accountID).Err(recErr).Msg("record grant failure failed")
		}
		return 0, 0, err
	}
	if err := store.markGrantedForAccount(accountID); err != nil {
		// Intel was applied but the ledger update failed — surface loudly so
		// the admin can reconcile before retrying (a retry would double-pay).
		return total, len(earned), fmt.Errorf("intel granted but claim update failed: %w", err)
	}
	grantBattlepassItems(ctx, store, deps, earned, pawnID)
	return total, len(earned), nil
}

// grantBattlepassItems delivers the item rewards attached to the earned
// tiers. Runs after the intel grant; failures are logged, not retried.
func grantBattlepassItems(ctx context.Context, store *battlepassStore, deps battlepassGrantDeps, earned []battlepassClaim, pawnID int64) {
	tiers, err := store.listTiers()
	if err != nil {
		componentLog("battlepass").Error().Err(err).Msg("list tiers for item rewards failed")
		return
	}
	rewardsByKey := make(map[string]string, len(tiers))
	for _, t := range tiers {
		if t.RewardItems != "" {
			rewardsByKey[t.TierKey] = t.RewardItems
		}
	}
	for _, c := range earned {
		raw, ok := rewardsByKey[c.TierKey]
		if !ok {
			continue
		}
		var items []rewardItem
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			componentLog("battlepass").Error().Str("tier_key", c.TierKey).Err(err).Msg("parse tier reward_items failed")
			continue
		}
		for _, item := range items {
			if err := deps.giveItem(ctx, pawnID, item.Template, item.Qty, item.Quality); err != nil {
				componentLog("battlepass").Error().Str("template", item.Template).Str("tier_key", c.TierKey).Int64("account_id", c.AccountID).Err(err).Msg("give item failed")
			}
		}
	}
}

// productionBattlepassGrantDeps builds grant deps from the given pool.
func productionBattlepassGrantDeps(pool *pgxpool.Pool) battlepassGrantDeps {
	return battlepassGrantDeps{
		fetchPlayers: func(ctx context.Context) ([]battlepassPlayer, error) {
			return cmdFetchBattlepassPlayers(ctx, pool)
		},
		awardIntel: func(ctx context.Context, pawnID, amount int64) error {
			return cmdAwardIntelCtx(ctx, pool, pawnID, amount)
		},
		giveItem: func(ctx context.Context, actorID int64, template string, qty, quality int64) error {
			return cmdGiveItemCtx(ctx, pool, actorID, template, qty, quality)
		},
		resolveGrantTarget: func(ctx context.Context, accountID int64) (int64, error) {
			if pool == nil {
				return 0, fmt.Errorf("database not connected")
			}
			return cmdFetchBattlepassGrantTargets(ctx, pool, accountID)
		},
	}
}

// battlepassStoreForCtx returns the battlepass store scoped to the request's
// server, so per-player claims, grants, and baselines never leak across servers.
// Returns nil when the store is unavailable. Tier-catalog handlers use
// globalBattlepassStore directly because the catalog has no server_id.
func battlepassStoreForCtx(r *http.Request) *battlepassStore {
	if globalBattlepassStore == nil {
		return nil
	}
	return globalBattlepassStore.withScope(storeScopeFromCtx(r))
}

// ── handlers ──────────────────────────────────────────────────────────────────

// handleListBattlepassTiers returns the tier catalog with per-tier claim counts.
func handleListBattlepassTiers(w http.ResponseWriter, r *http.Request) {
	store := battlepassStoreForCtx(r)
	if store == nil {
		jsonErr(w, fmt.Errorf("battlepass store not available"), http.StatusServiceUnavailable)
		return
	}
	tiers, err := store.listTiers()
	if err != nil {
		componentLog("battlepass").Error().Err(err).Msg("list tiers failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	counts, err := store.countsByTier()
	if err != nil {
		componentLog("battlepass").Error().Err(err).Msg("counts by tier failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	// playerCount lets the UI render per-tier population percentages. Best
	// effort: 0 when the game DB is unavailable.
	db := dbFromCtx(r)
	playerCount := 0
	if db != nil {
		if players, err := cmdFetchBattlepassPlayers(r.Context(), db); err == nil {
			playerCount = len(players)
		} else {
			componentLog("battlepass").Warn().Err(err).Msg("fetch players for tier counts failed")
		}
	}
	jsonOK(w, map[string]any{
		"tiers":         tiers,
		"counts":        counts,
		"player_count":  playerCount,
		"default_count": len(defaultBattlepassCatalog()),
	})
}

// handleBattlepassTiersBulk enables, disables, or deletes a set of tiers.
func handleBattlepassTiersBulk(w http.ResponseWriter, r *http.Request) {
	if globalBattlepassStore == nil {
		jsonErr(w, fmt.Errorf("battlepass store not available"), http.StatusServiceUnavailable)
		return
	}
	var req struct {
		IDs    []int64 `json:"ids"`
		Action string  `json:"action"`
	}
	if err := decode(r, &req); err != nil || len(req.IDs) == 0 {
		jsonErr(w, fmt.Errorf("ids and action are required"), http.StatusBadRequest)
		return
	}
	var err error
	switch req.Action {
	case "enable":
		err = globalBattlepassStore.setTiersEnabled(req.IDs, true)
	case "disable":
		err = globalBattlepassStore.setTiersEnabled(req.IDs, false)
	case "delete":
		err = globalBattlepassStore.deleteTiers(req.IDs)
	default:
		jsonErr(w, fmt.Errorf("action must be enable, disable, or delete"), http.StatusBadRequest)
		return
	}
	if err != nil {
		componentLog("battlepass").Error().Str("action", req.Action).Err(err).Msg("bulk tier action failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": true, "count": len(req.IDs)})
}

// mergeBattlepassTierUpdate resolves the optional label/reward_items fields
// against the existing tier: omitted fields keep their current values so
// callers like the inline intel editor don't have to send everything.
// Non-empty reward_items must parse as a []rewardItem JSON array.
func mergeBattlepassTierUpdate(existing *battlepassTier, reqLabel, reqRewardItems *string) (string, string, error) {
	label := existing.Label
	if reqLabel != nil && *reqLabel != "" {
		label = *reqLabel
	}
	rewardItems := existing.RewardItems
	if reqRewardItems != nil {
		if *reqRewardItems != "" {
			var items []rewardItem
			if err := json.Unmarshal([]byte(*reqRewardItems), &items); err != nil {
				return "", "", fmt.Errorf("reward_items must be a JSON array of {template, qty, quality}")
			}
		}
		rewardItems = *reqRewardItems
	}
	return label, rewardItems, nil
}

// battlepassTierExport is the wire type for catalog export/import. It omits the
// auto-assigned database ID so that exported files can be cleanly imported.
type battlepassTierExport struct {
	TierKey     string           `json:"tier_key"`
	Category    string           `json:"category"`
	Label       string           `json:"label"`
	Signal      battlepassSignal `json:"signal"`
	SignalKey   string           `json:"signal_key"`
	Threshold   int64            `json:"threshold"`
	Intel       int64            `json:"intel"`
	RewardItems string           `json:"reward_items"`
	Enabled     bool             `json:"enabled"`
}

// battlepassCatalogEnvelope wraps an exported catalog with a version number so
// future format changes remain backward-compatible.
type battlepassCatalogEnvelope struct {
	Version int                    `json:"version"`
	Tiers   []battlepassTierExport `json:"tiers"`
}

// validateBattlepassTier returns an error if any required field is invalid.
// Used by create, full-update, and import handlers.
func validateBattlepassTier(t battlepassTier) error {
	if t.TierKey == "" {
		return fmt.Errorf("tier_key is required")
	}
	if t.Category == "" {
		return fmt.Errorf("category is required")
	}
	if t.Label == "" {
		return fmt.Errorf("label is required")
	}
	switch t.Signal {
	case battlepassSignalLevel, battlepassSignalJourneyNode, battlepassSignalPlayerTag:
	default:
		return fmt.Errorf("signal must be level, journey_node, or player_tag")
	}
	if t.Intel < 0 {
		return fmt.Errorf("intel must be >= 0")
	}
	if t.Threshold < 0 {
		return fmt.Errorf("threshold must be >= 0")
	}
	if t.Signal == battlepassSignalLevel && t.Threshold == 0 {
		return fmt.Errorf("threshold must be > 0 for level signal")
	}
	if t.Signal != battlepassSignalLevel && t.SignalKey == "" {
		return fmt.Errorf("signal_key is required for non-level signal")
	}
	if t.RewardItems != "" {
		var items []rewardItem
		if err := json.Unmarshal([]byte(t.RewardItems), &items); err != nil {
			return fmt.Errorf("reward_items must be a JSON array of {template, qty, quality}")
		}
	}
	return nil
}

// mergeOptionalTierFields applies optional pointer fields from a request onto
// the existing tier, returning the merged category, signal, signal_key, and
// threshold. Nil pointers keep the existing value.
func mergeOptionalTierFields(ex *battlepassTier, cat *string, sig *battlepassSignal, sigKey *string, thr *int64) (string, battlepassSignal, string, int64) {
	category := ex.Category
	if cat != nil {
		category = *cat
	}
	signal := ex.Signal
	if sig != nil {
		signal = *sig
	}
	signalKey := ex.SignalKey
	if sigKey != nil {
		signalKey = *sigKey
	}
	threshold := ex.Threshold
	if thr != nil {
		threshold = *thr
	}
	return category, signal, signalKey, threshold
}

// handleUpdateBattlepassTier adjusts a tier's editable fields. tier_key is
// immutable and is silently ignored if present in the request body.
func handleUpdateBattlepassTier(w http.ResponseWriter, r *http.Request) {
	if globalBattlepassStore == nil {
		jsonErr(w, fmt.Errorf("battlepass store not available"), http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid id"), http.StatusBadRequest)
		return
	}
	var req struct {
		Label       *string           `json:"label"`
		Intel       int64             `json:"intel"`
		Enabled     bool              `json:"enabled"`
		RewardItems *string           `json:"reward_items"`
		Category    *string           `json:"category"`
		Signal      *battlepassSignal `json:"signal"`
		SignalKey   *string           `json:"signal_key"`
		Threshold   *int64            `json:"threshold"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, fmt.Errorf("invalid request body"), http.StatusBadRequest)
		return
	}
	// Validate intel early so the test for negative intel (id=1, empty store)
	// gets a 400 before the store lookup.
	if req.Intel < 0 {
		jsonErr(w, fmt.Errorf("intel must be >= 0"), http.StatusBadRequest)
		return
	}

	existing, err := globalBattlepassStore.getTier(id)
	if errors.Is(err, errNotFound) {
		jsonErr(w, fmt.Errorf("tier not found"), http.StatusNotFound)
		return
	}
	if err != nil {
		componentLog("battlepass").Error().Int64("tier_id", id).Err(err).Msg("get tier failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}

	label, rewardItems, err := mergeBattlepassTierUpdate(existing, req.Label, req.RewardItems)
	if err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	category, signal, signalKey, threshold := mergeOptionalTierFields(existing, req.Category, req.Signal, req.SignalKey, req.Threshold)

	merged := battlepassTier{
		TierKey: existing.TierKey, Category: category, Label: label,
		Signal: signal, SignalKey: signalKey, Threshold: threshold,
		Intel: req.Intel, Enabled: req.Enabled, RewardItems: rewardItems,
	}
	if err := validateBattlepassTier(merged); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}

	tier, err := globalBattlepassStore.updateTier(id, label, req.Intel, req.Enabled, rewardItems, category, signal, signalKey, threshold)
	if errors.Is(err, errNotFound) {
		jsonErr(w, fmt.Errorf("tier not found"), http.StatusNotFound)
		return
	}
	if err != nil {
		componentLog("battlepass").Error().Int64("tier_id", id).Err(err).Msg("update tier failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, tier)
}

// handleBattlepassProgress returns one account's claims and pending intel.
func handleBattlepassProgress(w http.ResponseWriter, r *http.Request) {
	store := battlepassStoreForCtx(r)
	if store == nil {
		jsonErr(w, fmt.Errorf("battlepass store not available"), http.StatusServiceUnavailable)
		return
	}
	accountID, err := strconv.ParseInt(r.PathValue("accountId"), 10, 64)
	if err != nil {
		jsonErr(w, fmt.Errorf("invalid account id"), http.StatusBadRequest)
		return
	}
	claims, err := store.listClaims(accountID)
	if err != nil {
		componentLog("battlepass").Error().Int64("account_id", accountID).Err(err).Msg("list claims failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	var pending int64
	for _, c := range claims {
		if c.Status == battlepassClaimEarned {
			pending += c.Intel
		}
	}
	jsonOK(w, map[string]any{"claims": claims, "pending_intel": pending})
}

// handleBattlepassPending lists all earned-but-ungranted claims at tier
// granularity, decorated with character name and online state when available.
func handleBattlepassPending(w http.ResponseWriter, r *http.Request) {
	store := battlepassStoreForCtx(r)
	if store == nil {
		jsonErr(w, fmt.Errorf("battlepass store not available"), http.StatusServiceUnavailable)
		return
	}
	tierRows, err := store.earnedClaimsWithTiers()
	if err != nil {
		componentLog("battlepass").Error().Err(err).Msg("earned claims with tiers failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}

	type pendingTierRow struct {
		AccountID   int64  `json:"account_id"`
		Name        string `json:"name"`
		Online      bool   `json:"online"`
		TierKey     string `json:"tier_key"`
		TierLabel   string `json:"tier_label"`
		Intel       int64  `json:"intel"`
		RewardItems string `json:"reward_items"`
	}
	db := dbFromCtx(r)
	names := map[int64]battlepassPlayer{}
	if db != nil {
		if players, err := cmdFetchBattlepassPlayers(r.Context(), db); err == nil {
			for _, p := range players {
				names[p.AccountID] = p
			}
		} else {
			componentLog("battlepass").Warn().Err(err).Msg("fetch players for pending view failed")
		}
	}
	out := make([]pendingTierRow, 0, len(tierRows))
	for _, tr := range tierRows {
		row := pendingTierRow{
			AccountID:   tr.AccountID,
			TierKey:     tr.TierKey,
			TierLabel:   tr.TierLabel,
			Intel:       tr.Intel,
			RewardItems: tr.RewardItems,
		}
		if p, ok := names[tr.AccountID]; ok {
			row.Name = p.Name
			row.Online = p.Online
		}
		out = append(out, row)
	}
	jsonOK(w, out)
}

// grantBattlepassTier grants a single earned tier for an account: awards its
// intel, marks the claim granted, then delivers any item rewards. Mirrors the
// semantics of grantBattlepassEarned but scoped to one tier.
func grantBattlepassTier(ctx context.Context, store *battlepassStore, deps battlepassGrantDeps, accountID int64, tierKey string) (int64, error) {
	claim, err := store.earnedClaim(accountID, tierKey)
	if err != nil {
		return 0, err
	}

	players, err := deps.fetchPlayers(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch players: %w", err)
	}
	var pawnID int64
	for _, p := range players {
		if p.AccountID == accountID {
			pawnID = p.PawnID
			break
		}
	}
	if pawnID == 0 {
		return 0, errNotFound
	}

	if err := deps.awardIntel(ctx, pawnID, claim.Intel); err != nil {
		if recErr := store.recordGrantFailureForTier(accountID, tierKey, err.Error()); recErr != nil {
			componentLog("battlepass").Error().Int64("account_id", accountID).Str("tier_key", tierKey).Err(recErr).Msg("record grant failure for tier failed")
		}
		return 0, err
	}
	if err := store.markGrantedForTier(accountID, tierKey); err != nil {
		return claim.Intel, fmt.Errorf("intel granted but claim update failed: %w", err)
	}
	grantBattlepassItems(ctx, store, deps, []battlepassClaim{claim}, pawnID)
	return claim.Intel, nil
}

// handleBattlepassGrantTier grants a single earned tier for an account.
func handleBattlepassGrantTier(w http.ResponseWriter, r *http.Request) {
	store := battlepassStoreForCtx(r)
	if store == nil {
		jsonErr(w, fmt.Errorf("battlepass store not available"), http.StatusServiceUnavailable)
		return
	}
	var req struct {
		AccountID int64  `json:"account_id"`
		TierKey   string `json:"tier_key"`
	}
	if err := decode(r, &req); err != nil || req.AccountID == 0 {
		jsonErr(w, fmt.Errorf("account_id is required"), http.StatusBadRequest)
		return
	}
	if req.TierKey == "" {
		jsonErr(w, fmt.Errorf("tier_key is required"), http.StatusBadRequest)
		return
	}
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}

	intel, err := grantBattlepassTier(r.Context(), store, productionBattlepassGrantDeps(db), req.AccountID, req.TierKey)
	switch {
	case errors.Is(err, errBattlepassNothingEarned):
		jsonErr(w, err, http.StatusBadRequest)
		return
	case errors.Is(err, errNotFound):
		jsonErr(w, fmt.Errorf("no character found for account %d", req.AccountID), http.StatusNotFound)
		return
	case err != nil:
		componentLog("battlepass").Error().Int64("account_id", req.AccountID).Str("tier_key", req.TierKey).Err(err).Msg("grant tier failed")
		jsonErr(w, err, http.StatusConflict)
		return
	}
	jsonOK(w, map[string]any{"granted_intel": intel})
}

// handleBattlepassReseed resets the tier catalog to the defaults. Claims are
// keyed by tier_key and survive the reseed.
func handleBattlepassReseed(w http.ResponseWriter, r *http.Request) {
	if globalBattlepassStore == nil {
		jsonErr(w, fmt.Errorf("battlepass store not available"), http.StatusServiceUnavailable)
		return
	}
	catalog := defaultBattlepassCatalog()
	if err := globalBattlepassStore.reseedTiers(catalog); err != nil {
		componentLog("battlepass").Error().Err(err).Msg("reseed tiers failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"seeded": len(catalog)})
}

// handleBattlepassGrant applies an account's earned intel to its character.
// The player must be offline — the game would clobber a live edit.
func handleBattlepassGrant(w http.ResponseWriter, r *http.Request) {
	store := battlepassStoreForCtx(r)
	if store == nil {
		jsonErr(w, fmt.Errorf("battlepass store not available"), http.StatusServiceUnavailable)
		return
	}
	db := dbFromCtx(r)
	if db == nil {
		jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
		return
	}
	var req struct {
		AccountID int64 `json:"account_id"`
	}
	if err := decode(r, &req); err != nil || req.AccountID == 0 {
		jsonErr(w, fmt.Errorf("account_id is required"), http.StatusBadRequest)
		return
	}

	total, tiers, err := grantBattlepassEarned(r.Context(), store, productionBattlepassGrantDeps(db), req.AccountID)
	switch {
	case errors.Is(err, errBattlepassNothingEarned):
		jsonErr(w, err, http.StatusBadRequest)
		return
	case errors.Is(err, errNotFound):
		jsonErr(w, fmt.Errorf("no character found for account %d", req.AccountID), http.StatusNotFound)
		return
	case err != nil:
		componentLog("battlepass").Error().Int64("account_id", req.AccountID).Err(err).Msg("grant earned failed")
		jsonErr(w, err, http.StatusConflict)
		return
	}
	jsonOK(w, map[string]any{"granted_intel": total, "tiers": tiers})
}

// battlepassConfigPayload is the request/response shape for the battlepass
// config endpoints — the 5 tuning knobs operators can change at runtime.
type battlepassConfigPayload struct {
	Enabled          *bool `json:"battlepass_enabled"`
	AwardPast        *bool `json:"battlepass_award_past"`
	AutoGrant        *bool `json:"battlepass_auto_grant"`
	PollSeconds      int   `json:"battlepass_poll_seconds"`
	ScanPaceMs       int   `json:"battlepass_scan_pace_ms"`
	ScanStartDelayMs int   `json:"battlepass_scan_start_delay_ms"`
}

func battlepassConfigFromLoaded() battlepassConfigPayload {
	return battlepassConfigPayload{
		Enabled:          loadedConfig.BattlepassEnabled,
		AwardPast:        loadedConfig.BattlepassAwardPast,
		AutoGrant:        loadedConfig.BattlepassAutoGrant,
		PollSeconds:      loadedConfig.BattlepassPollSeconds,
		ScanPaceMs:       loadedConfig.BattlepassScanPaceMs,
		ScanStartDelayMs: loadedConfig.BattlepassScanStartDelayMs,
	}
}

// handleGetBattlepassConfig returns the current battlepass engine configuration.
func handleGetBattlepassConfig(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, battlepassConfigFromLoaded())
}

// handleSaveBattlepassConfig persists the battlepass engine config knobs to
// the config file and updates the in-memory loadedConfig. The running engine is
// NOT restarted — interval/pace changes take effect on the next server restart.
func handleSaveBattlepassConfig(w http.ResponseWriter, r *http.Request) {
	var p battlepassConfigPayload
	if err := decode(r, &p); err != nil {
		jsonErr(w, fmt.Errorf("decode: %w", err), http.StatusBadRequest)
		return
	}

	loadedConfig.BattlepassEnabled = p.Enabled
	loadedConfig.BattlepassAwardPast = p.AwardPast
	loadedConfig.BattlepassAutoGrant = p.AutoGrant
	loadedConfig.BattlepassPollSeconds = p.PollSeconds
	loadedConfig.BattlepassScanPaceMs = p.ScanPaceMs
	loadedConfig.BattlepassScanStartDelayMs = p.ScanStartDelayMs

	if err := persistGlobalSettings(loadedConfig); err != nil {
		componentLog("battlepass").Error().Err(err).Msg("persist config failed")
		jsonErr(w, fmt.Errorf("failed to write config"), http.StatusInternalServerError)
		return
	}

	applyBattlepassEngine(loadedConfig)
	jsonOK(w, battlepassConfigFromLoaded())
}

// handleCreateBattlepassTier inserts a new tier into the catalog.
// Returns 409 when the tier_key already exists.
func handleCreateBattlepassTier(w http.ResponseWriter, r *http.Request) {
	if globalBattlepassStore == nil {
		jsonErr(w, fmt.Errorf("battlepass store not available"), http.StatusServiceUnavailable)
		return
	}
	var t battlepassTier
	if err := decode(r, &t); err != nil {
		jsonErr(w, fmt.Errorf("invalid request body"), http.StatusBadRequest)
		return
	}
	if err := validateBattlepassTier(t); err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	tier, err := globalBattlepassStore.createTier(t)
	if errors.Is(err, errBattlepassDuplicateTierKey) {
		jsonErr(w, fmt.Errorf("tier_key %q already exists", t.TierKey), http.StatusConflict)
		return
	}
	if err != nil {
		componentLog("battlepass").Error().Str("tier_key", t.TierKey).Err(err).Msg("create tier failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, tier)
}

// handleExportBattlepassCatalog returns the full tier catalog as a versioned
// JSON envelope suitable for import on another server. Internal IDs are omitted.
func handleExportBattlepassCatalog(w http.ResponseWriter, r *http.Request) {
	if globalBattlepassStore == nil {
		jsonErr(w, fmt.Errorf("battlepass store not available"), http.StatusServiceUnavailable)
		return
	}
	tiers, err := globalBattlepassStore.listTiers()
	if err != nil {
		componentLog("battlepass").Error().Err(err).Msg("export catalog failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	exports := make([]battlepassTierExport, len(tiers))
	for i, t := range tiers {
		exports[i] = battlepassTierExport{
			TierKey: t.TierKey, Category: t.Category, Label: t.Label,
			Signal: t.Signal, SignalKey: t.SignalKey, Threshold: t.Threshold,
			Intel: t.Intel, RewardItems: t.RewardItems, Enabled: t.Enabled,
		}
	}
	jsonOK(w, battlepassCatalogEnvelope{Version: 1, Tiers: exports})
}

// validateBattlepassImport validates the envelope and converts its tiers to
// battlepassTier values ready for reseedTiers. Returns an error describing the
// first problem found — the whole payload is rejected on any violation.
func validateBattlepassImport(env battlepassCatalogEnvelope) ([]battlepassTier, error) {
	if len(env.Tiers) == 0 {
		return nil, fmt.Errorf("tiers must not be empty")
	}
	seen := make(map[string]bool, len(env.Tiers))
	out := make([]battlepassTier, len(env.Tiers))
	for i, e := range env.Tiers {
		t := battlepassTier{
			TierKey: e.TierKey, Category: e.Category, Label: e.Label,
			Signal: e.Signal, SignalKey: e.SignalKey, Threshold: e.Threshold,
			Intel: e.Intel, RewardItems: e.RewardItems, Enabled: e.Enabled,
		}
		if err := validateBattlepassTier(t); err != nil {
			return nil, fmt.Errorf("tier %d (%q): %w", i, e.TierKey, err)
		}
		if seen[e.TierKey] {
			return nil, fmt.Errorf("duplicate tier_key %q in import payload", e.TierKey)
		}
		seen[e.TierKey] = true
		out[i] = t
	}
	return out, nil
}

// handleImportBattlepassCatalog replaces the entire tier catalog with the
// tiers from the request body. Claims are preserved — they re-attach by
// tier_key after the reseed. The whole payload is validated before any write.
func handleImportBattlepassCatalog(w http.ResponseWriter, r *http.Request) {
	if globalBattlepassStore == nil {
		jsonErr(w, fmt.Errorf("battlepass store not available"), http.StatusServiceUnavailable)
		return
	}
	var env battlepassCatalogEnvelope
	if err := decode(r, &env); err != nil {
		jsonErr(w, fmt.Errorf("invalid request body"), http.StatusBadRequest)
		return
	}
	tiers, err := validateBattlepassImport(env)
	if err != nil {
		jsonErr(w, err, http.StatusBadRequest)
		return
	}
	if err := globalBattlepassStore.reseedTiers(tiers); err != nil {
		componentLog("battlepass").Error().Err(err).Msg("import catalog failed")
		jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"imported": len(tiers)})
}
