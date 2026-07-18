package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// globalEventsCancel stops the running events engine goroutine.
// Protected by globalEventsMu; nil when the engine is not running.
var globalEventsCancel context.CancelFunc
var globalEventsMu sync.Mutex

// milestoneSignal names the data source used to evaluate a milestone event.
type milestoneSignal string

const (
	milestoneSignalLevel       milestoneSignal = "level"
	milestoneSignalAchievement milestoneSignal = "achievement_tag"
)

// zoneRaceConfig is the type-specific config for a zone_race event.
type zoneRaceConfig struct {
	Map          string  `json:"map"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Z            float64 `json:"z"`
	Radius       float64 `json:"radius"`
	Participants []int64 `json:"participants"`
}

// milestoneConfig is the type-specific config for a milestone event.
// AwardPast controls whether startup backfill also grants the reward
// (announcements are never sent during backfill regardless).
type milestoneConfig struct {
	Signal    milestoneSignal `json:"signal"`
	Threshold int64           `json:"threshold"` // for level signals
	TagName   string          `json:"tag_name"`  // for achievement_tag signals
	AwardPast bool            `json:"award_past"`
}

// rewardSpec describes items, currency, and XP to grant on event completion.
type rewardSpec struct {
	Items    []rewardItem `json:"items,omitempty"`
	Currency int64        `json:"currency,omitempty"`
	XP       []rewardXP   `json:"xp,omitempty"`
}

type rewardItem struct {
	Template string `json:"template"`
	Qty      int64  `json:"qty"`
	Quality  int64  `json:"quality"`
}

type rewardXP struct {
	Track  string `json:"track"`
	Amount int32  `json:"amount"`
}

// eventPlayer is the minimal player info the engine needs for evaluations.
type eventPlayer struct {
	AccountID    int64
	ControllerID int64
	ActorID      int64
	Name         string
}

// eventOutcome is one player who won/qualified in a tick, with their reward and announcement.
type eventOutcome struct {
	AccountID    int64
	ControllerID int64
	ActorID      int64
	PlayerName   string
	RewardSpec   *rewardSpec
	AnnounceText string
}

// eventDeps holds injectable functions so the engine can be unit-tested
// without a live DB, Discord session, or game server.
type eventDeps struct {
	fetchOnlinePlayers   func(ctx context.Context) ([]eventPlayer, error)
	fetchOnlinePositions func(ctx context.Context, accountIDs []int64) (map[int64]playerPosition, error)
	fetchPlayerLevel     func(ctx context.Context, accountID int64) (int, error)
	fetchPlayerTags      func(ctx context.Context, accountID int64) ([]string, error)
	grantCurrency        func(ctx context.Context, controllerID, amount int64) error
	grantItem            func(ctx context.Context, actorID int64, template string, qty, quality int64) error
	grantXP              func(ctx context.Context, actorID int64, track string, amount int32) error
	announce             func(channelID, message string) error
	// resolveGrantTargets returns (controllerID, actorID) for an account without
	// requiring the player to be online — used by reward-grant retries.
	resolveGrantTargets func(ctx context.Context, accountID int64) (controllerID, actorID int64, err error)
}

// globalEventStore is set once at startup, guarded in every handler.
var globalEventStore *eventStore

// jsonMarshal wraps json.Marshal so tests can call it via mustMarshalJSON.
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// ── geometry ──────────────────────────────────────────────────────────────────

// pointInSphere reports whether (x,y,z) lies inside or on the sphere centered
// at (cx,cy,cz) with radius r.
func pointInSphere(x, y, z, cx, cy, cz, r float64) bool {
	dx, dy, dz := x-cx, y-cy, z-cz
	return dx*dx+dy*dy+dz*dz <= r*r
}

// ── template rendering ────────────────────────────────────────────────────────

// renderEventTemplate substitutes {player}, {event}, and {value} placeholders.
func renderEventTemplate(tmpl, player, event, value string) string {
	r := strings.NewReplacer("{player}", player, "{event}", event, "{value}", value)
	return r.Replace(tmpl)
}

// ── evaluators ────────────────────────────────────────────────────────────────

// evaluateZoneRace returns outcomes for zone_race events: the first (up to one)
// online participant whose position falls inside the finish sphere.
func evaluateZoneRace(ctx context.Context, deps eventDeps, def eventDefinition) ([]eventOutcome, error) {
	var cfg zoneRaceConfig
	if err := json.Unmarshal([]byte(def.Config), &cfg); err != nil {
		return nil, fmt.Errorf("parse zone_race config: %w", err)
	}
	if len(cfg.Participants) == 0 {
		return nil, nil
	}

	players, err := deps.fetchOnlinePlayers(ctx)
	if err != nil {
		return nil, fmt.Errorf("zone race fetch players: %w", err)
	}

	playerByAccount := make(map[int64]eventPlayer, len(players))
	for _, p := range players {
		playerByAccount[p.AccountID] = p
	}

	positions, err := deps.fetchOnlinePositions(ctx, cfg.Participants)
	if err != nil {
		return nil, fmt.Errorf("zone race fetch positions: %w", err)
	}

	var reward *rewardSpec
	if def.Reward != "" {
		var rs rewardSpec
		if err := json.Unmarshal([]byte(def.Reward), &rs); err == nil {
			reward = &rs
		}
	}

	for _, accountID := range cfg.Participants {
		if outcome := zoneRaceWinner(cfg, def, positions, playerByAccount, accountID, reward); outcome != nil {
			return []eventOutcome{*outcome}, nil
		}
	}
	return nil, nil
}

// zoneRaceWinner returns a non-nil outcome when accountID is online, on the
// correct map, inside the finish sphere, and present in the player list.
func zoneRaceWinner(cfg zoneRaceConfig, def eventDefinition, positions map[int64]playerPosition, players map[int64]eventPlayer, accountID int64, reward *rewardSpec) *eventOutcome {
	pos, online := positions[accountID]
	if !online {
		return nil
	}
	if cfg.Map != "" && pos.Map != cfg.Map {
		return nil
	}
	if !pointInSphere(pos.X, pos.Y, pos.Z, cfg.X, cfg.Y, cfg.Z, cfg.Radius) {
		return nil
	}
	p, found := players[accountID]
	if !found {
		return nil
	}
	text := renderEventTemplate(def.AnnounceTemplate, p.Name, def.Name, "")
	return &eventOutcome{
		AccountID:    accountID,
		ControllerID: p.ControllerID,
		ActorID:      p.ActorID,
		PlayerName:   p.Name,
		RewardSpec:   reward,
		AnnounceText: text,
	}
}

// evaluateMilestone returns outcomes for milestone events: all online players
// who satisfy the milestone condition and have not yet been claimed.
func evaluateMilestone(ctx context.Context, deps eventDeps, def eventDefinition) ([]eventOutcome, error) {
	var cfg milestoneConfig
	if err := json.Unmarshal([]byte(def.Config), &cfg); err != nil {
		return nil, fmt.Errorf("parse milestone config: %w", err)
	}

	players, err := deps.fetchOnlinePlayers(ctx)
	if err != nil {
		return nil, fmt.Errorf("milestone fetch players: %w", err)
	}

	var reward *rewardSpec
	if def.Reward != "" {
		var rs rewardSpec
		if err := json.Unmarshal([]byte(def.Reward), &rs); err == nil {
			reward = &rs
		}
	}

	var outcomes []eventOutcome
	for _, p := range players {
		qualified, val, err := checkMilestoneSignal(ctx, deps, p.AccountID, cfg)
		if err != nil {
			componentLog("events").Warn().Str("signal", string(cfg.Signal)).Int64("account_id", p.AccountID).Err(err).Msg("milestone signal failed")
			continue
		}
		if !qualified {
			continue
		}
		text := renderEventTemplate(def.AnnounceTemplate, p.Name, def.Name, fmt.Sprintf("%d", val))
		outcomes = append(outcomes, eventOutcome{
			AccountID:    p.AccountID,
			ControllerID: p.ControllerID,
			ActorID:      p.ActorID,
			PlayerName:   p.Name,
			RewardSpec:   reward,
			AnnounceText: text,
		})
	}
	return outcomes, nil
}

// checkMilestoneSignal returns (qualified, value, error) for a single player.
func checkMilestoneSignal(ctx context.Context, deps eventDeps, accountID int64, cfg milestoneConfig) (bool, int64, error) {
	switch cfg.Signal {
	case milestoneSignalLevel:
		level, err := deps.fetchPlayerLevel(ctx, accountID)
		if err != nil {
			return false, 0, err
		}
		return int64(level) >= cfg.Threshold, int64(level), nil
	case milestoneSignalAchievement:
		tags, err := deps.fetchPlayerTags(ctx, accountID)
		if err != nil {
			return false, 0, err
		}
		for _, tag := range tags {
			if tag == cfg.TagName {
				return true, 1, nil
			}
		}
		return false, 0, nil
	default:
		return false, 0, fmt.Errorf("unknown milestone signal %q", cfg.Signal)
	}
}

// ── outcome application (claim-guarded) ──────────────────────────────────────

// applyEventOutcomes runs each outcome through the claim ledger: if not already
// claimed, it grants the reward and (when announce=true) posts the announcement,
// then records the claim. announce=false is the silent-backfill path.
func applyEventOutcomes(ctx context.Context, deps eventDeps, store *eventStore, def eventDefinition, outcomes []eventOutcome, announce bool) error {
	for _, o := range outcomes {
		applyOneOutcome(ctx, deps, store, def, o, announce)
	}
	return nil
}

func sliceRewardSpec(reward *rewardSpec, lastErr string) *rewardSpec {
	if reward == nil {
		return nil
	}
	if lastErr == "" || strings.HasPrefix(lastErr, "grant currency:") {
		return reward
	}

	rem := &rewardSpec{}

	itemFailedIdx := -1
	for i, item := range reward.Items {
		prefix := fmt.Sprintf("grant item %q:", item.Template)
		if strings.HasPrefix(lastErr, prefix) {
			itemFailedIdx = i
			break
		}
	}

	if itemFailedIdx != -1 {
		rem.Items = reward.Items[itemFailedIdx:]
		rem.XP = reward.XP
		return rem
	}

	xpFailedIdx := -1
	for i, xp := range reward.XP {
		prefix := fmt.Sprintf("grant xp track %q:", xp.Track)
		if strings.HasPrefix(lastErr, prefix) {
			xpFailedIdx = i
			break
		}
	}

	if xpFailedIdx != -1 {
		rem.XP = reward.XP[xpFailedIdx:]
		return rem
	}

	return reward
}

// applyOneOutcome applies a single outcome: checks the claim ledger, grants the
// reward, optionally announces, then records the claim.
func applyOneOutcome(ctx context.Context, deps eventDeps, store *eventStore, def eventDefinition, o eventOutcome, announce bool) {
	status, lastErr, exists, err := store.getClaimStatus(def.ID, def.Version, o.AccountID)
	if err != nil {
		componentLog("events").Warn().Int64("event_id", def.ID).Int("version", def.Version).Int64("account_id", o.AccountID).Err(err).Msg("getClaimStatus failed")
		return
	}
	if exists {
		// granted is terminal; exhausted is manual-grant-only — both block auto delivery.
		if status == eventClaimStatusGranted || status == eventClaimStatusExhausted {
			return
		}
		// pending (or a legacy "failed" row) is mid-retry: resume only the
		// components that haven't been granted yet, so a partial failure doesn't
		// re-deliver currency/items that already landed.
		o.RewardSpec = sliceRewardSpec(o.RewardSpec, lastErr)
	}
	if o.RewardSpec != nil {
		if err := grantEventReward(ctx, deps, o.RewardSpec, o.ControllerID, o.ActorID); err != nil {
			componentLog("events").Error().Int64("account_id", o.AccountID).Err(err).Msg("grant reward failed")
			_ = store.recordFailed(def.ID, def.Version, o.AccountID, err.Error())
			return
		}
	}
	// An explicit per-event AnnounceChannelID is an override; otherwise pass an
	// empty channel so deps.announce resolves the server's linked announce channel
	// (falling back to the legacy global announce channel when there's no link).
	channelID := def.AnnounceChannelID
	if announce && o.AnnounceText != "" {
		if err := deps.announce(channelID, o.AnnounceText); err != nil {
			componentLog("events").Warn().Int64("account_id", o.AccountID).Err(err).Msg("announce failed")
		}
	}
	if err := store.recordGranted(def.ID, def.Version, o.AccountID); err != nil {
		componentLog("events").Warn().Int64("event_id", def.ID).Int("version", def.Version).Int64("account_id", o.AccountID).Err(err).Msg("recordGranted failed")
	}
}

// grantEventReward delivers all reward components to a player.
func grantEventReward(ctx context.Context, deps eventDeps, reward *rewardSpec, controllerID, actorID int64) error {
	if reward.Currency != 0 {
		if err := deps.grantCurrency(ctx, controllerID, reward.Currency); err != nil {
			return fmt.Errorf("grant currency: %w", err)
		}
	}
	for _, item := range reward.Items {
		if err := deps.grantItem(ctx, actorID, item.Template, item.Qty, item.Quality); err != nil {
			return fmt.Errorf("grant item %q: %w", item.Template, err)
		}
	}
	for _, xp := range reward.XP {
		if err := deps.grantXP(ctx, actorID, xp.Track, xp.Amount); err != nil {
			return fmt.Errorf("grant xp track %q: %w", xp.Track, err)
		}
	}
	return nil
}

// ── startup backfill ──────────────────────────────────────────────────────────

// reconcileEvent silently seals already-satisfied milestones on startup so
// the live loop never announces past deeds. Zone races are not backfilled
// (they are session-scoped). If AwardPast is set the reward is also granted.
func reconcileEvent(ctx context.Context, deps eventDeps, store *eventStore, def eventDefinition) error {
	if def.Type == eventTypeZoneRace {
		return nil
	}
	outcomes, err := evaluateMilestone(ctx, deps, def)
	if err != nil {
		return fmt.Errorf("reconcile evaluate %d: %w", def.ID, err)
	}

	var cfg milestoneConfig
	if err := json.Unmarshal([]byte(def.Config), &cfg); err != nil {
		return fmt.Errorf("reconcile parse config %d: %w", def.ID, err)
	}

	// Build outcomes with reward only when AwardPast is set
	if !cfg.AwardPast {
		for i := range outcomes {
			outcomes[i].RewardSpec = nil
		}
	}
	// announce=false → silent backfill, never announces
	return applyEventOutcomes(ctx, deps, store, def, outcomes, false)
}

// ── polling loop ──────────────────────────────────────────────────────────────

// runEventEngine is the background polling loop. It checks every second and
// dispatches events that are due based on their per-event poll_seconds +
// random jitter (0..jitter_seconds).
func runEventEngine(ctx context.Context, deps eventDeps, store *eventStore) {
	lastEval := make(map[int64]time.Time)
	nextDue := make(map[int64]time.Time) // when each event is next scheduled

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			processEventTick(ctx, deps, store, now, lastEval, nextDue)
		}
	}
}

func processEventTick(
	ctx context.Context,
	deps eventDeps,
	store *eventStore,
	now time.Time,
	lastEval map[int64]time.Time,
	nextDue map[int64]time.Time,
) {
	events, err := store.list()
	if err != nil {
		componentLog("events").Warn().Err(err).Msg("list failed")
		return
	}
	for _, def := range events {
		if !def.Enabled {
			continue
		}
		if !scheduleEventIfDue(def, now, lastEval, nextDue) {
			continue
		}
		processOneEvent(ctx, deps, store, def)
	}
	pruneDeletedEvents(events, lastEval, nextDue)
}

// scheduleEventIfDue returns true if def is due to fire at now, and advances
// its nextDue entry. On first sight of an event it fires immediately.
func scheduleEventIfDue(def eventDefinition, now time.Time, lastEval, nextDue map[int64]time.Time) bool {
	due, hasDue := nextDue[def.ID]
	if !hasDue {
		nextDue[def.ID] = now
		due = now
	}
	if now.Before(due) {
		return false
	}
	lastEval[def.ID] = now
	poll := time.Duration(def.PollSeconds) * time.Second
	if poll <= 0 {
		poll = 7 * time.Second
	}
	jitter := time.Duration(0)
	if def.JitterSeconds > 0 {
		jitter = time.Duration(rand.IntN(def.JitterSeconds+1)) * time.Second //nolint:gosec
	}
	nextDue[def.ID] = now.Add(poll + jitter)
	return true
}

// pruneDeletedEvents removes map entries for events that no longer exist.
func pruneDeletedEvents(events []eventDefinition, lastEval, nextDue map[int64]time.Time) {
	live := make(map[int64]struct{}, len(events))
	for _, d := range events {
		live[d.ID] = struct{}{}
	}
	for id := range lastEval {
		if _, ok := live[id]; !ok {
			delete(lastEval, id)
			delete(nextDue, id)
		}
	}
}

func processOneEvent(ctx context.Context, deps eventDeps, store *eventStore, def eventDefinition) {
	var outcomes []eventOutcome
	var err error
	switch def.Type {
	case eventTypeZoneRace:
		outcomes, err = evaluateZoneRace(ctx, deps, def)
	case eventTypeMilestone:
		outcomes, err = evaluateMilestone(ctx, deps, def)
	default:
		componentLog("events").Warn().Str("type", string(def.Type)).Int64("event_id", def.ID).Msg("unknown event type")
		return
	}
	if err != nil {
		componentLog("events").Warn().Int64("event_id", def.ID).Err(err).Msg("evaluate failed")
		return
	}
	if err := applyEventOutcomes(ctx, deps, store, def, outcomes, true); err != nil {
		componentLog("events").Warn().Int64("event_id", def.ID).Err(err).Msg("apply outcomes failed")
	}
}

// ── reward-grant retry loop ────────────────────────────────────────────────────

// attemptGrantForClaim re-delivers a single claim's reward. It resolves the
// event's reward spec and the player's current grant targets, attempts the
// grant, and records success or failure on the claim ledger. Returns an error
// only when the grant itself fails (callers may surface that to a user).
func attemptGrantForClaim(ctx context.Context, deps eventDeps, store *eventStore, claim eventClaimRecord) error {
	def, err := store.get(claim.EventID)
	if err != nil {
		return fmt.Errorf("load event %d: %w", claim.EventID, err)
	}

	reward, err := parseRewardSpec(def.Reward)
	if err != nil {
		return fmt.Errorf("parse reward %d: %w", def.ID, err)
	}

	if reward != nil {
		// Resume only the un-granted remainder so a scheduled retry of a partial
		// failure doesn't re-deliver components that already landed.
		reward = sliceRewardSpec(reward, claim.LastError)
		controllerID, actorID, terr := deps.resolveGrantTargets(ctx, claim.AccountID)
		if terr != nil {
			recordErr := fmt.Errorf("resolve grant targets: %w", terr)
			_ = store.recordFailed(claim.EventID, claim.Version, claim.AccountID, recordErr.Error())
			return recordErr
		}
		if grantErr := grantEventReward(ctx, deps, reward, controllerID, actorID); grantErr != nil {
			_ = store.recordFailed(claim.EventID, claim.Version, claim.AccountID, grantErr.Error())
			return grantErr
		}
	}

	if err := store.recordGranted(claim.EventID, claim.Version, claim.AccountID); err != nil {
		return fmt.Errorf("record granted %d/%d/%d: %w", claim.EventID, claim.Version, claim.AccountID, err)
	}
	return nil
}

// parseRewardSpec parses an event's reward JSON. An empty string yields a nil
// spec (nothing to grant) with no error.
func parseRewardSpec(rewardJSON string) (*rewardSpec, error) {
	if rewardJSON == "" {
		return nil, nil
	}
	var rs rewardSpec
	if err := json.Unmarshal([]byte(rewardJSON), &rs); err != nil {
		return nil, err
	}
	return &rs, nil
}

// eventGrantSource adapts an eventStore to the shared deferredGrantSource
// contract. It carries the eventDeps and store needed to attempt grants so the
// generic loop can drive event reward retries without knowing event specifics.
type eventGrantSource struct {
	deps  eventDeps
	store *eventStore
	// pending maps a full claim key (event:version:account) to the typed claim
	// record for the current tick. Keying by account alone collapses an
	// account's claims across multiple events, double-delivering one of them
	// and skipping the rest (#291).
	pending map[string]eventClaimRecord
}

// eventClaimKey identifies one claim within a tick: event, version, account.
func eventClaimKey(eventID int64, version int, accountID int64) string {
	return fmt.Sprintf("%d:%d:%d", eventID, version, accountID)
}

func newEventGrantSource(deps eventDeps, store *eventStore) *eventGrantSource {
	return &eventGrantSource{deps: deps, store: store, pending: map[string]eventClaimRecord{}}
}

func (s *eventGrantSource) listRetryableDeferredClaims(now time.Time) ([]deferredClaim, error) {
	claims, err := s.store.listRetryableClaims(now)
	if err != nil {
		return nil, err
	}
	out := make([]deferredClaim, 0, len(claims))
	s.pending = make(map[string]eventClaimRecord, len(claims))
	for _, c := range claims {
		s.pending[eventClaimKey(c.EventID, c.Version, c.AccountID)] = c
		out = append(out, deferredClaim{
			OwnerID:  c.AccountID,
			Attempts: c.Attempts,
			Ref:      fmt.Sprintf("%d:%d", c.EventID, c.Version),
		})
	}
	return out, nil
}

func (s *eventGrantSource) attempt(ctx context.Context, dc deferredClaim) error {
	claim, ok := s.pending[fmt.Sprintf("%s:%d", dc.Ref, dc.OwnerID)]
	if !ok {
		return fmt.Errorf("event claim %s for account %d not found in tick", dc.Ref, dc.OwnerID)
	}
	return attemptGrantForClaim(ctx, s.deps, s.store, claim)
}

// runEventRetryLoop periodically retries pending reward grants whose backoff
// window has elapsed. It is ctx-cancellable and a no-op when the store is nil.
// Delegates to the shared deferred-grant core.
func runEventRetryLoop(ctx context.Context, deps eventDeps, store *eventStore) {
	if store == nil {
		runDeferredGrantLoop(ctx, nil, nil)
		return
	}
	src := newEventGrantSource(deps, store)
	runDeferredGrantLoop(ctx, src, src.attempt)
}

// applyEventEngine stops any running events engine goroutine, then starts a
// new one if events_enabled is true. Safe to call from config save handlers.
func applyEventEngine(cfg appConfig) {
	globalEventsMu.Lock()
	defer globalEventsMu.Unlock()

	if globalEventsCancel != nil {
		globalEventsCancel()
		globalEventsCancel = nil
		componentLog("events").Info().Msg("engine stopped")
	}

	if !eventsEnabled(cfg) {
		return
	}
	if globalEventStore == nil {
		componentLog("events").Warn().Msg("store not initialised; engine disabled")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	globalEventsCancel = cancel
	for _, sc := range globalRegistry.All() {
		if sc.DB == nil {
			continue
		}
		deps := productionEventDeps(sc.DB, sc.StoreScope)
		go reconcileAllEvents(context.Background(), deps, globalEventStore)
		go runEventEngine(ctx, deps, globalEventStore)
		go runEventRetryLoop(ctx, deps, globalEventStore)
	}
	componentLog("events").Info().Msg("engine started (per-event scheduling + reward-grant retries)")
}

// stopEventEngine cancels the running events engine goroutine if any.
func stopEventEngine() {
	globalEventsMu.Lock()
	defer globalEventsMu.Unlock()
	if globalEventsCancel != nil {
		globalEventsCancel()
		globalEventsCancel = nil
	}
}

// reconcileAllEvents backfills claims for all milestone events on startup.
func reconcileAllEvents(ctx context.Context, deps eventDeps, store *eventStore) {
	events, err := store.list()
	if err != nil {
		componentLog("events").Warn().Err(err).Msg("reconcile list failed")
		return
	}
	for _, def := range events {
		if def.Type == eventTypeZoneRace || !def.Enabled {
			continue
		}
		if err := reconcileEvent(ctx, deps, store, def); err != nil {
			componentLog("events").Warn().Int64("event_id", def.ID).Err(err).Msg("reconcile event failed")
		}
	}
}

// productionEventDeps builds the event deps from the given pool, bound to the
// owning server id so announces post to that server's linked announce channel.
// Called from applyEventEngine only; tests inject mocks directly.
func productionEventDeps(pool *pgxpool.Pool, serverID int) eventDeps {
	return eventDeps{
		fetchOnlinePlayers: func(ctx context.Context) ([]eventPlayer, error) {
			if pool == nil {
				return nil, fmt.Errorf("database not connected")
			}
			return cmdFetchEventPlayers(ctx, pool)
		},
		fetchOnlinePositions: func(ctx context.Context, accountIDs []int64) (map[int64]playerPosition, error) {
			if pool == nil {
				return nil, fmt.Errorf("database not connected")
			}
			return cmdFetchOnlinePositions(ctx, pool, accountIDs)
		},
		fetchPlayerLevel: func(ctx context.Context, accountID int64) (int, error) {
			if pool == nil {
				return 0, fmt.Errorf("database not connected")
			}
			return cmdFetchCharacterLevel(ctx, pool, accountID)
		},
		fetchPlayerTags: func(ctx context.Context, accountID int64) ([]string, error) {
			if pool == nil {
				return nil, fmt.Errorf("database not connected")
			}
			return cmdFetchPlayerTagsForAccount(ctx, pool, accountID)
		},
		grantCurrency: func(ctx context.Context, controllerID, amount int64) error {
			if pool == nil {
				return fmt.Errorf("database not connected")
			}
			_, err := cmdGiveCurrencyCtx(ctx, pool, controllerID, amount)
			return err
		},
		grantItem: func(ctx context.Context, actorID int64, template string, qty, quality int64) error {
			if pool == nil {
				return fmt.Errorf("database not connected")
			}
			return cmdGiveItemCtx(ctx, pool, actorID, template, qty, quality)
		},
		grantXP: func(ctx context.Context, actorID int64, track string, amount int32) error {
			if pool == nil {
				return fmt.Errorf("database not connected")
			}
			return cmdAwardXPCtx(ctx, pool, actorID, track, amount)
		},
		announce:            makeEventAnnounceFn(serverID),
		resolveGrantTargets: makeEventResolveGrantTargetsFn(pool),
	}
}

// makeEventAnnounceFn returns the announce dep for a server: an explicit
// per-event channel override posts directly; an empty channel fans out to every
// guild mapped to serverID. Extracted to keep productionEventDeps within the
// cognitive-complexity gate.
func makeEventAnnounceFn(serverID int) func(channelID, message string) error {
	return func(channelID, message string) error {
		if channelID != "" {
			return postDiscordAnnouncement(channelID, message)
		}
		return announceToServer(serverID, message)
	}
}

// makeEventResolveGrantTargetsFn returns a closure that resolves event grant targets
// for the given pool, extracted to keep productionEventDeps complexity in bounds.
func makeEventResolveGrantTargetsFn(pool *pgxpool.Pool) func(context.Context, int64) (int64, int64, error) {
	return func(ctx context.Context, accountID int64) (int64, int64, error) {
		if pool == nil {
			return 0, 0, fmt.Errorf("database not connected")
		}
		return cmdFetchEventGrantTargets(ctx, pool, accountID)
	}
}

// productionResolveGrantTargets resolves grant targets against the live DB.
func productionResolveGrantTargets(ctx context.Context, accountID int64) (int64, int64, error) {
	if globalDB == nil {
		return 0, 0, fmt.Errorf("database not connected")
	}
	return cmdFetchEventGrantTargets(ctx, globalDB, accountID)
}

// eventsEnabled returns the effective events-enabled flag (default off).
func eventsEnabled(cfg appConfig) bool {
	if cfg.EventsEnabled == nil {
		return false
	}
	return *cfg.EventsEnabled
}
