package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

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
			log.Printf("events: milestone signal %s account %d: %v", cfg.Signal, p.AccountID, err)
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

// applyOneOutcome applies a single outcome: checks the claim ledger, grants the
// reward, optionally announces, then records the claim.
func applyOneOutcome(ctx context.Context, deps eventDeps, store *eventStore, def eventDefinition, o eventOutcome, announce bool) {
	claimed, err := store.claimExists(def.ID, def.Version, o.AccountID)
	if err != nil {
		log.Printf("events: claimExists %d/%d/%d: %v", def.ID, def.Version, o.AccountID, err)
		return
	}
	if claimed {
		return
	}
	if o.RewardSpec != nil {
		if err := grantEventReward(ctx, deps, o.RewardSpec, o.ControllerID, o.ActorID); err != nil {
			log.Printf("events: grant reward account %d: %v", o.AccountID, err)
			_ = store.recordFailed(def.ID, def.Version, o.AccountID, err.Error())
			return
		}
	}
	channelID := def.AnnounceChannelID
	if channelID == "" {
		channelID = loadedConfig.DiscordAnnounceChannelID
	}
	if announce && o.AnnounceText != "" {
		if err := deps.announce(channelID, o.AnnounceText); err != nil {
			log.Printf("events: announce account %d: %v", o.AccountID, err)
		}
	}
	if err := store.recordGranted(def.ID, def.Version, o.AccountID); err != nil {
		log.Printf("events: recordGranted %d/%d/%d: %v", def.ID, def.Version, o.AccountID, err)
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

// runEventEngine is the background polling loop. It loads enabled events on
// each tick and dispatches each to its type evaluator.
func runEventEngine(ctx context.Context, deps eventDeps, store *eventStore, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processEventTick(ctx, deps, store)
		}
	}
}

func processEventTick(ctx context.Context, deps eventDeps, store *eventStore) {
	events, err := store.list()
	if err != nil {
		log.Printf("events: list: %v", err)
		return
	}
	for _, def := range events {
		if !def.Enabled {
			continue
		}
		processOneEvent(ctx, deps, store, def)
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
		log.Printf("events: unknown type %q for event %d", def.Type, def.ID)
		return
	}
	if err != nil {
		log.Printf("events: evaluate %d: %v", def.ID, err)
		return
	}
	if err := applyEventOutcomes(ctx, deps, store, def, outcomes, true); err != nil {
		log.Printf("events: apply outcomes %d: %v", def.ID, err)
	}
}

// startEventEngineIfEnabled wires production dependencies and starts the
// background goroutine. Mirrors the startSessionPoller / startWelcomePackageScanner
// lifecycle pattern in sessions.go and welcome_package.go.
func startEventEngineIfEnabled(cfg appConfig) context.CancelFunc {
	if !eventsEnabled(cfg) {
		return func() {}
	}
	if globalEventStore == nil {
		log.Printf("events: store not initialised — engine disabled")
		return func() {}
	}

	interval := clampEventInterval(cfg.EventsPollSeconds)
	deps := productionEventDeps()

	// Reconcile already-satisfied milestones silently before the live loop starts.
	go reconcileAllEvents(context.Background(), deps, globalEventStore)

	ctx, cancel := context.WithCancel(context.Background())
	go runEventEngine(ctx, deps, globalEventStore, interval)
	return cancel
}

// reconcileAllEvents backfills claims for all milestone events on startup.
func reconcileAllEvents(ctx context.Context, deps eventDeps, store *eventStore) {
	events, err := store.list()
	if err != nil {
		log.Printf("events: reconcile list: %v", err)
		return
	}
	for _, def := range events {
		if def.Type == eventTypeZoneRace || !def.Enabled {
			continue
		}
		if err := reconcileEvent(ctx, deps, store, def); err != nil {
			log.Printf("events: reconcile event %d: %v", def.ID, err)
		}
	}
}

// clampEventInterval converts EventsPollSeconds to a Duration, clamped [1s, 60s].
func clampEventInterval(secs int) time.Duration {
	if secs < 1 {
		secs = 7
	}
	if secs > 60 {
		secs = 60
	}
	return time.Duration(secs) * time.Second
}

// productionEventDeps builds the event deps using live globals. Called from
// startEventEngineIfEnabled only; tests inject mocks directly.
func productionEventDeps() eventDeps {
	return eventDeps{
		fetchOnlinePlayers: func(ctx context.Context) ([]eventPlayer, error) {
			if globalDB == nil {
				return nil, fmt.Errorf("database not connected")
			}
			return cmdFetchEventPlayers(ctx, globalDB)
		},
		fetchOnlinePositions: func(ctx context.Context, accountIDs []int64) (map[int64]playerPosition, error) {
			if globalDB == nil {
				return nil, fmt.Errorf("database not connected")
			}
			return cmdFetchOnlinePositions(ctx, globalDB, accountIDs)
		},
		fetchPlayerLevel: func(ctx context.Context, accountID int64) (int, error) {
			if globalDB == nil {
				return 0, fmt.Errorf("database not connected")
			}
			return cmdFetchCharacterLevel(ctx, globalDB, accountID)
		},
		fetchPlayerTags: func(ctx context.Context, accountID int64) ([]string, error) {
			if globalDB == nil {
				return nil, fmt.Errorf("database not connected")
			}
			return cmdFetchPlayerTagsForAccount(ctx, globalDB, accountID)
		},
		grantCurrency: func(ctx context.Context, controllerID, amount int64) error {
			if globalDB == nil {
				return fmt.Errorf("database not connected")
			}
			_, err := cmdGiveCurrencyCtx(ctx, globalDB, controllerID, amount)
			return err
		},
		grantItem: func(ctx context.Context, actorID int64, template string, qty, quality int64) error {
			if globalDB == nil {
				return fmt.Errorf("database not connected")
			}
			return cmdGiveItemCtx(ctx, globalDB, actorID, template, qty, quality)
		},
		grantXP: func(ctx context.Context, actorID int64, track string, amount int32) error {
			if globalDB == nil {
				return fmt.Errorf("database not connected")
			}
			return cmdAwardXPCtx(ctx, globalDB, actorID, track, amount)
		},
		announce: func(channelID, message string) error {
			return postDiscordAnnouncement(channelID, message)
		},
	}
}

// eventsEnabled returns the effective events-enabled flag (default off).
func eventsEnabled(cfg appConfig) bool {
	if cfg.EventsEnabled == nil {
		return false
	}
	return *cfg.EventsEnabled
}
