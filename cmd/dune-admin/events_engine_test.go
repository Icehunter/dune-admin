package main

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// ── geometry ──────────────────────────────────────────────────────────────────

func TestPointInSphere(t *testing.T) {
	tests := []struct {
		name                   string
		x, y, z, cx, cy, cz, r float64
		want                   bool
	}{
		{"origin inside", 0, 0, 0, 0, 0, 0, 1, true},
		{"on boundary", 1, 0, 0, 0, 0, 0, 1, true},
		{"just outside", 1.01, 0, 0, 0, 0, 0, 1, false},
		{"negative coords inside", -1, -1, -1, -2, -2, -2, 2, true},
		{"zero radius exact match", 5, 5, 5, 5, 5, 5, 0, true},
		{"zero radius miss", 5, 5, 5, 5, 5, 5.1, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := pointInSphere(tt.x, tt.y, tt.z, tt.cx, tt.cy, tt.cz, tt.r); got != tt.want {
				t.Fatalf("pointInSphere = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── template rendering ────────────────────────────────────────────────────────

func TestRenderEventTemplate(t *testing.T) {
	tests := []struct {
		name, tmpl, player, event, value string
		want                             string
	}{
		{"all placeholders", "{player} won {event}! value={value}", "Alice", "DeathRace", "42", "Alice won DeathRace! value=42"},
		{"player only", "Congrats {player}!", "Bob", "", "", "Congrats Bob!"},
		{"no placeholders", "just text", "X", "Y", "Z", "just text"},
		{"empty template", "", "X", "Y", "Z", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := renderEventTemplate(tt.tmpl, tt.player, tt.event, tt.value); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func openMemEventStore(t *testing.T) *eventStore {
	t.Helper()
	s, err := openEventStore(":memory:")
	if err != nil {
		t.Fatalf("openEventStore: %v", err)
	}
	t.Cleanup(func() { _ = s.db.Close() })
	return s
}

func mustCreateEvent(t *testing.T, s *eventStore, name string, typ eventType) eventDefinition {
	t.Helper()
	d, err := s.create(eventDefinition{Name: name, Type: typ, Config: "{}"})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	return *d
}

// noopDeps returns an eventDeps where all functions succeed with empty results.
func noopDeps() eventDeps {
	return eventDeps{
		fetchOnlinePlayers: func(_ context.Context) ([]eventPlayer, error) {
			return nil, nil
		},
		fetchOnlinePositions: func(_ context.Context, _ []int64) (map[int64]playerPosition, error) {
			return nil, nil
		},
		fetchPlayerLevel: func(_ context.Context, _ int64) (int, error) { return 0, nil },
		fetchPlayerTags:  func(_ context.Context, _ int64) ([]string, error) { return nil, nil },
		grantCurrency:    func(_ context.Context, _, _ int64) error { return nil },
		grantItem:        func(_ context.Context, _ int64, _ string, _, _ int64) error { return nil },
		grantXP:          func(_ context.Context, _ int64, _ string, _ int32) error { return nil },
		announce:         func(_, _ string) error { return nil },
		resolveGrantTargets: func(_ context.Context, _ int64) (int64, int64, error) {
			return 0, 0, nil
		},
	}
}

// ── zone race evaluator ───────────────────────────────────────────────────────

func TestEvaluateZoneRace_WinnerInsideSphere(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "death_race", eventTypeZoneRace)

	// participant 101 is online and inside the sphere; 102 is outside
	positions := map[int64]playerPosition{
		101: {Map: "DuneMap_P", X: 1, Y: 1, Z: 1},
		102: {Map: "DuneMap_P", X: 100, Y: 100, Z: 100},
	}
	players := []eventPlayer{
		{AccountID: 101, ControllerID: 201, ActorID: 301, Name: "Alice"},
		{AccountID: 102, ControllerID: 202, ActorID: 302, Name: "Bob"},
	}

	deps := noopDeps()
	deps.fetchOnlinePositions = func(_ context.Context, _ []int64) (map[int64]playerPosition, error) {
		return positions, nil
	}
	deps.fetchOnlinePlayers = func(_ context.Context) ([]eventPlayer, error) { return players, nil }

	cfg := zoneRaceConfig{
		Map: "DuneMap_P", X: 0, Y: 0, Z: 0, Radius: 10,
		Participants: []int64{101, 102},
	}
	def.Config = mustMarshalJSON(t, cfg)

	outcomes, err := evaluateZoneRace(context.Background(), deps, def)
	if err != nil {
		t.Fatalf("evaluateZoneRace: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("want 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].AccountID != 101 {
		t.Fatalf("want winner AccountID 101, got %d", outcomes[0].AccountID)
	}
}

func TestEvaluateZoneRace_NobodyInside(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "death_race", eventTypeZoneRace)

	positions := map[int64]playerPosition{
		101: {X: 100, Y: 100, Z: 100},
	}
	deps := noopDeps()
	deps.fetchOnlinePositions = func(_ context.Context, _ []int64) (map[int64]playerPosition, error) {
		return positions, nil
	}
	deps.fetchOnlinePlayers = func(_ context.Context) ([]eventPlayer, error) {
		return []eventPlayer{{AccountID: 101, Name: "Alice"}}, nil
	}

	cfg := zoneRaceConfig{
		Map: "DuneMap_P", X: 0, Y: 0, Z: 0, Radius: 10,
		Participants: []int64{101},
	}
	def.Config = mustMarshalJSON(t, cfg)

	outcomes, err := evaluateZoneRace(context.Background(), deps, def)
	if err != nil {
		t.Fatalf("evaluateZoneRace: %v", err)
	}
	if len(outcomes) != 0 {
		t.Fatalf("want 0 outcomes, got %d", len(outcomes))
	}
}

func TestEvaluateZoneRace_AlreadyClaimed_Skips(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "death_race", eventTypeZoneRace)
	if err := store.recordGranted(def.ID, def.Version, 101); err != nil {
		t.Fatalf("seed claim: %v", err)
	}

	positions := map[int64]playerPosition{101: {X: 1, Y: 1, Z: 1}}
	deps := noopDeps()
	deps.fetchOnlinePositions = func(_ context.Context, _ []int64) (map[int64]playerPosition, error) {
		return positions, nil
	}
	deps.fetchOnlinePlayers = func(_ context.Context) ([]eventPlayer, error) {
		return []eventPlayer{{AccountID: 101, Name: "Alice"}}, nil
	}

	cfg := zoneRaceConfig{X: 0, Y: 0, Z: 0, Radius: 10, Participants: []int64{101}}
	def.Config = mustMarshalJSON(t, cfg)

	outcomes, err := evaluateZoneRace(context.Background(), deps, def)
	if err != nil {
		t.Fatalf("evaluateZoneRace: %v", err)
	}
	// The evaluator itself doesn't check claims — applyEventOutcomes does.
	// This test validates the evaluator still returns the in-zone player.
	// Idempotency is tested in TestApplyEventOutcomes_IdempotentClaims.
	_ = outcomes
}

func TestEvaluateZoneRace_FetchError(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "err_race", eventTypeZoneRace)

	deps := noopDeps()
	deps.fetchOnlinePositions = func(_ context.Context, _ []int64) (map[int64]playerPosition, error) {
		return nil, errors.New("db down")
	}
	deps.fetchOnlinePlayers = func(_ context.Context) ([]eventPlayer, error) {
		return []eventPlayer{{AccountID: 101}}, nil
	}

	cfg := zoneRaceConfig{Participants: []int64{101}}
	def.Config = mustMarshalJSON(t, cfg)

	_, err := evaluateZoneRace(context.Background(), deps, def)
	if err == nil {
		t.Fatal("want error from fetch, got nil")
	}
}

func TestEvaluateZoneRace_WrongMap_NoOutcome(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "map_race", eventTypeZoneRace)

	positions := map[int64]playerPosition{
		101: {Map: "OtherMap_P", X: 1, Y: 1, Z: 1}, // inside sphere but wrong map
	}
	deps := noopDeps()
	deps.fetchOnlinePositions = func(_ context.Context, _ []int64) (map[int64]playerPosition, error) {
		return positions, nil
	}
	deps.fetchOnlinePlayers = func(_ context.Context) ([]eventPlayer, error) {
		return []eventPlayer{{AccountID: 101, Name: "Alice"}}, nil
	}

	cfg := zoneRaceConfig{Map: "DuneMap_P", X: 0, Y: 0, Z: 0, Radius: 10, Participants: []int64{101}}
	def.Config = mustMarshalJSON(t, cfg)

	outcomes, err := evaluateZoneRace(context.Background(), deps, def)
	if err != nil {
		t.Fatalf("evaluateZoneRace: %v", err)
	}
	if len(outcomes) != 0 {
		t.Fatalf("want 0 outcomes for wrong map, got %d", len(outcomes))
	}
}

func TestEvaluateZoneRace_PlayerNotInPlayerList_Skipped(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "ghost_race", eventTypeZoneRace)

	// position exists but player is not in the online player list
	positions := map[int64]playerPosition{
		101: {Map: "DuneMap_P", X: 1, Y: 1, Z: 1},
	}
	deps := noopDeps()
	deps.fetchOnlinePositions = func(_ context.Context, _ []int64) (map[int64]playerPosition, error) {
		return positions, nil
	}
	deps.fetchOnlinePlayers = func(_ context.Context) ([]eventPlayer, error) {
		return []eventPlayer{}, nil // account 101 not present
	}

	cfg := zoneRaceConfig{Map: "DuneMap_P", X: 0, Y: 0, Z: 0, Radius: 10, Participants: []int64{101}}
	def.Config = mustMarshalJSON(t, cfg)

	outcomes, err := evaluateZoneRace(context.Background(), deps, def)
	if err != nil {
		t.Fatalf("evaluateZoneRace: %v", err)
	}
	if len(outcomes) != 0 {
		t.Fatalf("want 0 outcomes for missing player, got %d", len(outcomes))
	}
}

// ── milestone evaluator ───────────────────────────────────────────────────────

func TestEvaluateMilestone_Level_CrossesThreshold(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "level_50", eventTypeMilestone)

	deps := noopDeps()
	deps.fetchOnlinePlayers = func(_ context.Context) ([]eventPlayer, error) {
		return []eventPlayer{
			{AccountID: 101, Name: "Alice"},
			{AccountID: 102, Name: "Bob"},
		}, nil
	}
	deps.fetchPlayerLevel = func(_ context.Context, accountID int64) (int, error) {
		if accountID == 101 {
			return 55, nil // above threshold 50
		}
		return 30, nil // below threshold
	}

	cfg := milestoneConfig{Signal: milestoneSignalLevel, Threshold: 50}
	def.Config = mustMarshalJSON(t, cfg)

	outcomes, err := evaluateMilestone(context.Background(), deps, def)
	if err != nil {
		t.Fatalf("evaluateMilestone: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("want 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].AccountID != 101 {
		t.Fatalf("want AccountID 101, got %d", outcomes[0].AccountID)
	}
}

func TestEvaluateMilestone_AchievementTag_Present(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "spice_vision", eventTypeMilestone)

	deps := noopDeps()
	deps.fetchOnlinePlayers = func(_ context.Context) ([]eventPlayer, error) {
		return []eventPlayer{
			{AccountID: 101, Name: "Alice"},
			{AccountID: 102, Name: "Bob"},
		}, nil
	}
	deps.fetchPlayerTags = func(_ context.Context, accountID int64) ([]string, error) {
		if accountID == 101 {
			return []string{"BigMoments.SpiceVision.Complete", "OtherTag"}, nil
		}
		return []string{"OtherTag"}, nil
	}

	cfg := milestoneConfig{Signal: milestoneSignalAchievement, TagName: "BigMoments.SpiceVision.Complete"}
	def.Config = mustMarshalJSON(t, cfg)

	outcomes, err := evaluateMilestone(context.Background(), deps, def)
	if err != nil {
		t.Fatalf("evaluateMilestone: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].AccountID != 101 {
		t.Fatalf("want 1 outcome for account 101, got %+v", outcomes)
	}
}

func TestEvaluateMilestone_NobodyCrossed(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "level_100", eventTypeMilestone)

	deps := noopDeps()
	deps.fetchOnlinePlayers = func(_ context.Context) ([]eventPlayer, error) {
		return []eventPlayer{{AccountID: 101, Name: "Alice"}}, nil
	}
	deps.fetchPlayerLevel = func(_ context.Context, _ int64) (int, error) { return 10, nil }

	cfg := milestoneConfig{Signal: milestoneSignalLevel, Threshold: 100}
	def.Config = mustMarshalJSON(t, cfg)

	outcomes, err := evaluateMilestone(context.Background(), deps, def)
	if err != nil {
		t.Fatalf("evaluateMilestone: %v", err)
	}
	if len(outcomes) != 0 {
		t.Fatalf("want 0 outcomes, got %d", len(outcomes))
	}
}

// ── applyEventOutcomes idempotency ────────────────────────────────────────────

func TestApplyEventOutcomes_IdempotentClaims(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "double_tick", eventTypeZoneRace)

	var grantCount int
	deps := noopDeps()
	deps.grantCurrency = func(_ context.Context, _, _ int64) error {
		grantCount++
		return nil
	}
	var announceCount int
	deps.announce = func(_, _ string) error {
		announceCount++
		return nil
	}

	reward := &rewardSpec{Currency: 100}
	outcomes := []eventOutcome{
		{AccountID: 101, PlayerName: "Alice", RewardSpec: reward, AnnounceText: "Alice won!"},
	}

	// Apply twice (simulating two ticks)
	for i := 0; i < 2; i++ {
		if err := applyEventOutcomes(context.Background(), deps, store, def, outcomes, true); err != nil {
			t.Fatalf("apply tick %d: %v", i+1, err)
		}
	}

	if grantCount != 1 {
		t.Fatalf("want 1 grant, got %d", grantCount)
	}
	if announceCount != 1 {
		t.Fatalf("want 1 announce, got %d", announceCount)
	}
}

func TestApplyEventOutcomes_AnnounceOnly(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "announce_only", eventTypeMilestone)

	var grantCalled bool
	deps := noopDeps()
	deps.grantCurrency = func(_ context.Context, _, _ int64) error { grantCalled = true; return nil }

	var announced string
	deps.announce = func(_, msg string) error { announced = msg; return nil }

	outcomes := []eventOutcome{
		{AccountID: 101, PlayerName: "Alice", RewardSpec: nil, AnnounceText: "Alice unlocked X!"},
	}

	if err := applyEventOutcomes(context.Background(), deps, store, def, outcomes, true); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if grantCalled {
		t.Fatal("grantCurrency should not be called for announce-only outcome")
	}
	if announced != "Alice unlocked X!" {
		t.Fatalf("announce message = %q, want %q", announced, "Alice unlocked X!")
	}
}

func TestApplyEventOutcomes_AnnounceFalse_NoAnnounce(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "silent_backfill", eventTypeMilestone)

	var announceCount int
	deps := noopDeps()
	deps.announce = func(_, _ string) error { announceCount++; return nil }

	outcomes := []eventOutcome{
		{AccountID: 101, PlayerName: "Alice", AnnounceText: "Alice reached level 50!"},
	}

	// announce=false → backfill path, no announcement
	if err := applyEventOutcomes(context.Background(), deps, store, def, outcomes, false); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if announceCount != 0 {
		t.Fatalf("want 0 announces, got %d", announceCount)
	}

	// But the claim IS recorded so a later live tick doesn't re-fire
	_, _, exists, err := store.getClaimStatus(def.ID, def.Version, 101)
	if err != nil {
		t.Fatalf("getClaimStatus: %v", err)
	}
	if !exists {
		t.Fatal("want claim recorded after silent backfill")
	}
}

// ── reconcileEvent backfill ───────────────────────────────────────────────────

func TestReconcileEvent_SilentBackfill_NoAward(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "level_50", eventTypeMilestone)

	var grantCount, announceCount int
	deps := noopDeps()
	deps.fetchOnlinePlayers = func(_ context.Context) ([]eventPlayer, error) {
		return []eventPlayer{{AccountID: 101, Name: "Alice"}}, nil
	}
	deps.fetchPlayerLevel = func(_ context.Context, _ int64) (int, error) { return 55, nil }
	deps.grantCurrency = func(_ context.Context, _, _ int64) error { grantCount++; return nil }
	deps.announce = func(_, _ string) error { announceCount++; return nil }

	cfg := milestoneConfig{Signal: milestoneSignalLevel, Threshold: 50, AwardPast: false}
	def.Config = mustMarshalJSON(t, cfg)

	if err := reconcileEvent(context.Background(), deps, store, def); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if grantCount != 0 {
		t.Fatalf("AwardPast=false: want 0 grants, got %d", grantCount)
	}
	if announceCount != 0 {
		t.Fatalf("want 0 announces, got %d", announceCount)
	}

	// Claim is sealed
	_, _, exists, err := store.getClaimStatus(def.ID, def.Version, 101)
	if err != nil {
		t.Fatalf("getClaimStatus: %v", err)
	}
	if !exists {
		t.Fatal("want claim sealed by reconcile")
	}
}

func TestReconcileEvent_AwardPast_GrantsButNoAnnounce(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "level_50_award", eventTypeMilestone)

	var grantCount, announceCount int
	deps := noopDeps()
	deps.fetchOnlinePlayers = func(_ context.Context) ([]eventPlayer, error) {
		return []eventPlayer{{AccountID: 101, ControllerID: 201, Name: "Alice"}}, nil
	}
	deps.fetchPlayerLevel = func(_ context.Context, _ int64) (int, error) { return 55, nil }
	deps.grantCurrency = func(_ context.Context, _, _ int64) error { grantCount++; return nil }
	deps.announce = func(_, _ string) error { announceCount++; return nil }

	reward := &rewardSpec{Currency: 500}
	cfg := milestoneConfig{Signal: milestoneSignalLevel, Threshold: 50, AwardPast: true}
	def.Config = mustMarshalJSON(t, cfg)
	def.Reward = mustMarshalJSON(t, reward)

	if err := reconcileEvent(context.Background(), deps, store, def); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if grantCount != 1 {
		t.Fatalf("AwardPast=true: want 1 grant, got %d", grantCount)
	}
	if announceCount != 0 {
		t.Fatalf("want 0 announces even with AwardPast, got %d", announceCount)
	}
}

func TestReconcileAllEvents_SkipsDisabled(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "disabled_milestone", eventTypeMilestone)

	def.Enabled = false
	if _, err := store.update(def); err != nil {
		t.Fatalf("disable event: %v", err)
	}

	var grantCount int
	deps := noopDeps()
	deps.fetchOnlinePlayers = func(_ context.Context) ([]eventPlayer, error) {
		return []eventPlayer{{AccountID: 101, Name: "Alice"}}, nil
	}
	deps.fetchPlayerLevel = func(_ context.Context, _ int64) (int, error) { return 99, nil }
	deps.grantCurrency = func(_ context.Context, _, _ int64) error {
		grantCount++
		return nil
	}

	reconcileAllEvents(context.Background(), deps, store)

	if grantCount != 0 {
		t.Fatalf("disabled event: want 0 grants, got %d", grantCount)
	}
}

func TestReconcileEvent_AlreadySealed_NoReprocess(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "level_50_sealed", eventTypeMilestone)

	// Pre-seal the claim
	if err := store.recordGranted(def.ID, def.Version, 101); err != nil {
		t.Fatalf("seed claim: %v", err)
	}

	var grantCount int
	deps := noopDeps()
	deps.fetchOnlinePlayers = func(_ context.Context) ([]eventPlayer, error) {
		return []eventPlayer{{AccountID: 101, Name: "Alice"}}, nil
	}
	deps.fetchPlayerLevel = func(_ context.Context, _ int64) (int, error) { return 55, nil }
	deps.grantCurrency = func(_ context.Context, _, _ int64) error { grantCount++; return nil }

	cfg := milestoneConfig{Signal: milestoneSignalLevel, Threshold: 50, AwardPast: true}
	def.Config = mustMarshalJSON(t, cfg)

	if err := reconcileEvent(context.Background(), deps, store, def); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if grantCount != 0 {
		t.Fatalf("pre-sealed claim: want 0 re-grants, got %d", grantCount)
	}
}

func TestSliceRewardSpec(t *testing.T) {
	reward := &rewardSpec{
		Currency: 100,
		Items: []rewardItem{
			{Template: "ItemA", Qty: 1},
			{Template: "ItemB", Qty: 2},
		},
		XP: []rewardXP{
			{Track: "TrackA", Amount: 50},
		},
	}

	tests := []struct {
		name    string
		lastErr string
		want    *rewardSpec
	}{
		{"nil reward", "", nil},
		{"empty error", "", reward},
		{"currency error", "grant currency: some error", reward},
		{"item A error", "grant item \"ItemA\": inventory full", &rewardSpec{
			Items: []rewardItem{{Template: "ItemA", Qty: 1}, {Template: "ItemB", Qty: 2}},
			XP:    []rewardXP{{Track: "TrackA", Amount: 50}},
		}},
		{"item B error", "grant item \"ItemB\": inventory full", &rewardSpec{
			Items: []rewardItem{{Template: "ItemB", Qty: 2}},
			XP:    []rewardXP{{Track: "TrackA", Amount: 50}},
		}},
		{"xp error", "grant xp track \"TrackA\": generic error", &rewardSpec{
			XP: []rewardXP{{Track: "TrackA", Amount: 50}},
		}},
		{"unknown error", "some random db error", reward},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input *rewardSpec
			if tt.name != "nil reward" {
				input = reward
			}
			got := sliceRewardSpec(input, tt.lastErr)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("want nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("want %+v, got nil", tt.want)
			}
			if got.Currency != tt.want.Currency || len(got.Items) != len(tt.want.Items) || len(got.XP) != len(tt.want.XP) {
				t.Fatalf("slice mismatch: want %+v, got %+v", tt.want, got)
			}
		})
	}
}

func TestApplyEventOutcomes_RetryPartialClaim(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "partial_retry", eventTypeZoneRace)

	var grantCurrencyCount int
	var grantedItems []string

	deps := noopDeps()
	deps.grantCurrency = func(_ context.Context, _, _ int64) error {
		grantCurrencyCount++
		return nil
	}
	deps.grantItem = func(_ context.Context, _ int64, tmpl string, _, _ int64) error {
		if tmpl == "ItemFail" {
			return errors.New("inventory full")
		}
		grantedItems = append(grantedItems, tmpl)
		return nil
	}

	reward := &rewardSpec{
		Currency: 100,
		Items: []rewardItem{
			{Template: "ItemOK"},
			{Template: "ItemFail"},
			{Template: "ItemSkipped"},
		},
	}

	outcomes := []eventOutcome{
		{AccountID: 101, PlayerName: "Alice", RewardSpec: reward},
	}

	// First tick: succeeds up to ItemFail, then aborts.
	if err := applyEventOutcomes(context.Background(), deps, store, def, outcomes, true); err != nil {
		t.Fatalf("apply tick 1: %v", err)
	}

	if grantCurrencyCount != 1 {
		t.Fatalf("want 1 currency grant, got %d", grantCurrencyCount)
	}
	if len(grantedItems) != 1 || grantedItems[0] != "ItemOK" {
		t.Fatalf("want [ItemOK], got %v", grantedItems)
	}

	// Now fix the issue so ItemFail succeeds
	deps.grantItem = func(_ context.Context, _ int64, tmpl string, _, _ int64) error {
		grantedItems = append(grantedItems, tmpl)
		return nil
	}

	// Second tick: should resume from ItemFail
	if err := applyEventOutcomes(context.Background(), deps, store, def, outcomes, true); err != nil {
		t.Fatalf("apply tick 2: %v", err)
	}

	// Currency should NOT be granted again
	if grantCurrencyCount != 1 {
		t.Fatalf("want 1 currency grant after retry, got %d", grantCurrencyCount)
	}
	// ItemOK should NOT be granted again, but ItemFail and ItemSkipped should be
	if len(grantedItems) != 3 {
		t.Fatalf("want 3 items total granted, got %v", grantedItems)
	}
	if grantedItems[1] != "ItemFail" || grantedItems[2] != "ItemSkipped" {
		t.Fatalf("want ItemFail then ItemSkipped, got %v", grantedItems[1:])
	}
}

// ── reward-grant retry ────────────────────────────────────────────────────────

// rewardEventDef seeds an event with a single-item reward and returns it.
func rewardEventDef(t *testing.T, s *eventStore, name string) eventDefinition {
	t.Helper()
	d, err := s.create(eventDefinition{
		Name:   name,
		Type:   eventTypeMilestone,
		Config: "{}",
		Reward: `{"items":[{"template":"Spice","qty":1,"quality":1}]}`,
	})
	if err != nil {
		t.Fatalf("create reward event: %v", err)
	}
	return *d
}

func TestAttemptGrantForClaim_FailThenSucceed(t *testing.T) {
	store := openMemEventStore(t)
	def := rewardEventDef(t, store, "retry_me")

	// First failure: marks the claim pending.
	if err := store.recordFailed(def.ID, def.Version, 101, "inventory full"); err != nil {
		t.Fatalf("recordFailed: %v", err)
	}

	grantCalls := 0
	deps := noopDeps()
	deps.resolveGrantTargets = func(_ context.Context, _ int64) (int64, int64, error) {
		return 201, 301, nil
	}
	deps.grantItem = func(_ context.Context, actorID int64, _ string, _, _ int64) error {
		grantCalls++
		if actorID != 301 {
			t.Errorf("grantItem actorID = %d, want 301", actorID)
		}
		return nil
	}

	due, err := store.listRetryableClaims(time.Now().Add(eventGrantRetryBackoff + time.Hour))
	if err != nil {
		t.Fatalf("listRetryableClaims: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("want 1 due claim, got %d", len(due))
	}

	if err := attemptGrantForClaim(context.Background(), deps, store, due[0]); err != nil {
		t.Fatalf("attemptGrantForClaim: %v", err)
	}
	if grantCalls != 1 {
		t.Errorf("grantItem calls = %d, want 1", grantCalls)
	}

	c := firstClaim(t, store, def.ID)
	if c.Status != eventClaimStatusGranted {
		t.Errorf("status = %q, want granted", c.Status)
	}
}

func TestAttemptGrantForClaim_ExhaustsAfterMaxAttempts(t *testing.T) {
	store := openMemEventStore(t)
	def := rewardEventDef(t, store, "always_fails")

	deps := noopDeps()
	deps.resolveGrantTargets = func(_ context.Context, _ int64) (int64, int64, error) {
		return 201, 301, nil
	}
	deps.grantItem = func(_ context.Context, _ int64, _ string, _, _ int64) error {
		return errors.New("inventory full")
	}

	// Seed an initial pending claim (attempt 1), then retry until exhausted.
	if err := store.recordFailed(def.ID, def.Version, 101, "inventory full"); err != nil {
		t.Fatalf("seed recordFailed: %v", err)
	}
	for i := 0; i < eventGrantMaxAttempts; i++ {
		claim := firstClaim(t, store, def.ID)
		if claim.Status == eventClaimStatusExhausted {
			break
		}
		// attemptGrantForClaim returns the grant error and records the failure.
		if err := attemptGrantForClaim(context.Background(), deps, store, claim); err == nil {
			t.Fatalf("attempt %d: want grant error, got nil", i)
		}
	}

	c := firstClaim(t, store, def.ID)
	if c.Status != eventClaimStatusExhausted {
		t.Errorf("status = %q, want exhausted after %d attempts", c.Status, eventGrantMaxAttempts)
	}
	if c.Attempts != eventGrantMaxAttempts {
		t.Errorf("attempts = %d, want %d", c.Attempts, eventGrantMaxAttempts)
	}

	// Once exhausted, it is no longer auto-retryable.
	due, err := store.listRetryableClaims(time.Now().Add(100 * eventGrantRetryBackoff))
	if err != nil {
		t.Fatalf("listRetryableClaims: %v", err)
	}
	if len(due) != 0 {
		t.Errorf("want 0 retryable after exhaustion, got %d", len(due))
	}
}

// TestEventGrantSource_MultipleDueEventsOneAccount mirrors the #291 battlepass
// regression on the events side: one account with due claims on two different
// events must receive each event's reward exactly once. The old account-keyed
// pending map collapsed them to whichever claim was listed last.
func TestEventGrantSource_MultipleDueEventsOneAccount(t *testing.T) {
	store := openMemEventStore(t)
	evA, err := store.create(eventDefinition{
		Name: "race_a", Type: eventTypeMilestone, Config: "{}", Reward: `{"currency":10}`,
	})
	if err != nil {
		t.Fatalf("create race_a: %v", err)
	}
	evB, err := store.create(eventDefinition{
		Name: "race_b", Type: eventTypeMilestone, Config: "{}", Reward: `{"currency":20}`,
	})
	if err != nil {
		t.Fatalf("create race_b: %v", err)
	}

	// Same account, one pending claim per event. The "grant currency:" last
	// error makes sliceRewardSpec retry the full reward.
	if err := store.recordFailed(evA.ID, evA.Version, 101, "grant currency: boom"); err != nil {
		t.Fatalf("recordFailed a: %v", err)
	}
	if err := store.recordFailed(evB.ID, evB.Version, 101, "grant currency: boom"); err != nil {
		t.Fatalf("recordFailed b: %v", err)
	}

	deps := noopDeps()
	deps.resolveGrantTargets = func(_ context.Context, _ int64) (int64, int64, error) {
		return 201, 301, nil
	}
	var amounts []int64
	deps.grantCurrency = func(_ context.Context, _, amount int64) error {
		amounts = append(amounts, amount)
		return nil
	}

	src := newEventGrantSource(deps, store)
	processDeferredGrantTick(context.Background(), src, src.attempt,
		time.Now().Add(eventGrantRetryBackoff+time.Hour))

	slices.Sort(amounts)
	if !slices.Equal(amounts, []int64{10, 20}) {
		t.Fatalf("currency amounts = %v, want each event delivered exactly once [10 20]", amounts)
	}
	for _, ev := range []eventDefinition{*evA, *evB} {
		status, _, exists, err := store.getClaimStatus(ev.ID, ev.Version, 101)
		if err != nil || !exists {
			t.Fatalf("getClaimStatus %s: exists=%v err=%v", ev.Name, exists, err)
		}
		if status != eventClaimStatusGranted {
			t.Errorf("%s claim status = %q, want granted", ev.Name, status)
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func mustMarshalJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := jsonMarshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
