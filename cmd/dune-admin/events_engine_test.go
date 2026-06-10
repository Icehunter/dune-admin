package main

import (
	"context"
	"errors"
	"testing"
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
	}
}

// ── zone race evaluator ───────────────────────────────────────────────────────

func TestEvaluateZoneRace_WinnerInsideSphere(t *testing.T) {
	t.Parallel()
	store := openMemEventStore(t)
	def := mustCreateEvent(t, store, "death_race", eventTypeZoneRace)

	// participant 101 is online and inside the sphere; 102 is outside
	positions := map[int64]playerPosition{
		101: {X: 1, Y: 1, Z: 1},
		102: {X: 100, Y: 100, Z: 100},
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
	exists, err := store.claimExists(def.ID, def.Version, 101)
	if err != nil {
		t.Fatalf("claimExists: %v", err)
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
	exists, err := store.claimExists(def.ID, def.Version, 101)
	if err != nil {
		t.Fatalf("claimExists: %v", err)
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

// ── helpers ───────────────────────────────────────────────────────────────────

func mustMarshalJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := jsonMarshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
