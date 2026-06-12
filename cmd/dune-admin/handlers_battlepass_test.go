package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// setupBattlepassStore sets globalBattlepassStore to a fresh in-memory store
// and restores nil on cleanup. NOT parallel — mutates package global.
func setupBattlepassStore(t *testing.T) *battlepassStore {
	t.Helper()
	s := testBattlepassStore(t)
	globalBattlepassStore = s
	t.Cleanup(func() { globalBattlepassStore = nil })
	return s
}

// ── nil-guard tests ───────────────────────────────────────────────────────────

func TestBattlepassHandlers_NilStore(t *testing.T) {
	globalBattlepassStore = nil
	cases := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		target  string
		pathID  string
	}{
		{"tiers", handleListBattlepassTiers, http.MethodGet, "/api/v1/battlepass/tiers", ""},
		{"update", handleUpdateBattlepassTier, http.MethodPut, "/api/v1/battlepass/tiers/1", "1"},
		{"progress", handleBattlepassProgress, http.MethodGet, "/api/v1/battlepass/progress/1", "1"},
		{"pending", handleBattlepassPending, http.MethodGet, "/api/v1/battlepass/pending", ""},
		{"reseed", handleBattlepassReseed, http.MethodPost, "/api/v1/battlepass/reseed", ""},
		{"grant", handleBattlepassGrant, http.MethodPost, "/api/v1/battlepass/grant", ""},
		{"grant-tier", handleBattlepassGrantTier, http.MethodPost, "/api/v1/battlepass/grant-tier", ""},
		{"bulk", handleBattlepassTiersBulk, http.MethodPost, "/api/v1/battlepass/tiers/bulk", ""},
		{"create", handleCreateBattlepassTier, http.MethodPost, "/api/v1/battlepass/tiers", ""},
		{"export", handleExportBattlepassCatalog, http.MethodGet, "/api/v1/battlepass/export", ""},
		{"import", handleImportBattlepassCatalog, http.MethodPost, "/api/v1/battlepass/import", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.target, bytes.NewReader([]byte(`{}`)))
			if c.pathID != "" {
				req.SetPathValue("id", c.pathID)
				req.SetPathValue("accountId", c.pathID)
			}
			rec := httptest.NewRecorder()
			c.handler(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("want 503, got %d", rec.Code)
			}
		})
	}
}

// ── tiers ─────────────────────────────────────────────────────────────────────

func TestHandleListBattlepassTiers(t *testing.T) {
	s := setupBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/battlepass/tiers", nil)
	rec := httptest.NewRecorder()
	handleListBattlepassTiers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Tiers  []battlepassTier                `json:"tiers"`
		Counts map[string]battlepassTierCounts `json:"counts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Tiers) != 3 {
		t.Fatalf("tiers len = %d, want 3", len(resp.Tiers))
	}
	if resp.Counts["level:5"].Earned != 1 {
		t.Fatalf("counts = %+v, want level:5 earned 1", resp.Counts)
	}
}

func TestHandleUpdateBattlepassTier(t *testing.T) {
	s := setupBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tiers, _ := s.listTiers()

	rewards := `[{"template":"Kindjal_4","qty":1,"quality":3}]`
	body, _ := json.Marshal(map[string]any{"intel": 99, "enabled": false, "label": "Custom", "reward_items": rewards})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/battlepass/tiers/1", bytes.NewReader(body))
	req.SetPathValue("id", fmt.Sprintf("%d", tiers[0].ID))
	rec := httptest.NewRecorder()
	handleUpdateBattlepassTier(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var tier battlepassTier
	_ = json.Unmarshal(rec.Body.Bytes(), &tier)
	if tier.Intel != 99 || tier.Enabled || tier.Label != "Custom" || tier.RewardItems != rewards {
		t.Fatalf("updated tier = %+v", tier)
	}

	// Omitted label/reward_items keep their values (inline intel edit path).
	body, _ = json.Marshal(map[string]any{"intel": 50, "enabled": true})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/battlepass/tiers/1", bytes.NewReader(body))
	req.SetPathValue("id", fmt.Sprintf("%d", tiers[0].ID))
	rec = httptest.NewRecorder()
	handleUpdateBattlepassTier(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("partial update: want 200, got %d", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tier)
	if tier.Intel != 50 || tier.Label != "Custom" || tier.RewardItems != rewards {
		t.Fatalf("partial update lost fields: %+v", tier)
	}

	// Malformed reward_items rejected.
	bad := "not json"
	body, _ = json.Marshal(map[string]any{"intel": 5, "enabled": true, "reward_items": bad})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/battlepass/tiers/1", bytes.NewReader(body))
	req.SetPathValue("id", fmt.Sprintf("%d", tiers[0].ID))
	rec = httptest.NewRecorder()
	handleUpdateBattlepassTier(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad reward_items: want 400, got %d", rec.Code)
	}
}

func TestHandleBattlepassTiersBulk(t *testing.T) {
	s := setupBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tiers, _ := s.listTiers()
	ids := []int64{tiers[0].ID, tiers[1].ID}

	do := func(action string, ids []int64) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"ids": ids, "action": action})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/tiers/bulk", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		handleBattlepassTiersBulk(rec, req)
		return rec
	}

	if rec := do("disable", ids); rec.Code != http.StatusOK {
		t.Fatalf("disable: want 200, got %d", rec.Code)
	}
	after, _ := s.listTiers()
	if after[0].Enabled || after[1].Enabled || !after[2].Enabled {
		t.Fatalf("disable did not apply: %+v", after)
	}

	if rec := do("enable", ids); rec.Code != http.StatusOK {
		t.Fatalf("enable: want 200, got %d", rec.Code)
	}
	if rec := do("delete", []int64{tiers[2].ID}); rec.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d", rec.Code)
	}
	after, _ = s.listTiers()
	if len(after) != 2 {
		t.Fatalf("after delete %d tiers, want 2", len(after))
	}

	if rec := do("explode", ids); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad action: want 400, got %d", rec.Code)
	}
	if rec := do("enable", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("no ids: want 400, got %d", rec.Code)
	}
}

func TestHandleUpdateBattlepassTier_BadInput(t *testing.T) {
	setupBattlepassStore(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/battlepass/tiers/x", bytes.NewReader([]byte(`{}`)))
	req.SetPathValue("id", "x")
	rec := httptest.NewRecorder()
	handleUpdateBattlepassTier(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id: want 400, got %d", rec.Code)
	}

	body, _ := json.Marshal(map[string]any{"intel": -5, "enabled": true})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/battlepass/tiers/1", bytes.NewReader(body))
	req.SetPathValue("id", "1")
	rec = httptest.NewRecorder()
	handleUpdateBattlepassTier(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("negative intel: want 400, got %d", rec.Code)
	}

	body, _ = json.Marshal(map[string]any{"intel": 5, "enabled": true})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/battlepass/tiers/9999", bytes.NewReader(body))
	req.SetPathValue("id", "9999")
	rec = httptest.NewRecorder()
	handleUpdateBattlepassTier(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing tier: want 404, got %d", rec.Code)
	}
}

// ── progress ──────────────────────────────────────────────────────────────────

func TestHandleBattlepassProgress(t *testing.T) {
	s := setupBattlepassStore(t)
	_ = s.recordClaim("level:5", 42, 10, battlepassClaimBaseline)
	_ = s.recordClaim("level:10", 42, 15, battlepassClaimEarned)
	_ = s.recordClaim("level:15", 42, 20, battlepassClaimGranted)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/battlepass/progress/42", nil)
	req.SetPathValue("accountId", "42")
	rec := httptest.NewRecorder()
	handleBattlepassProgress(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Claims       []battlepassClaim `json:"claims"`
		PendingIntel int64             `json:"pending_intel"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Claims) != 3 {
		t.Fatalf("claims len = %d, want 3", len(resp.Claims))
	}
	if resp.PendingIntel != 15 {
		t.Fatalf("pending intel = %d, want 15 (earned only)", resp.PendingIntel)
	}
}

func TestHandleBattlepassProgress_BadID(t *testing.T) {
	setupBattlepassStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/battlepass/progress/x", nil)
	req.SetPathValue("accountId", "x")
	rec := httptest.NewRecorder()
	handleBattlepassProgress(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

// ── reseed ────────────────────────────────────────────────────────────────────

func TestHandleBattlepassReseed(t *testing.T) {
	s := setupBattlepassStore(t)
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimGranted)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/reseed", nil)
	rec := httptest.NewRecorder()
	handleBattlepassReseed(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	tiers, _ := s.listTiers()
	if len(tiers) != len(defaultBattlepassCatalog()) {
		t.Fatalf("tiers = %d, want full default catalog", len(tiers))
	}
	keys, _ := s.claimedKeys(1)
	if keys["level:5"] != battlepassClaimGranted {
		t.Fatal("reseed must preserve claims")
	}
}

// ── grant ─────────────────────────────────────────────────────────────────────

func grantTestDeps(players []battlepassPlayer, awardErr error, awarded *[]int64) battlepassGrantDeps {
	return battlepassGrantDeps{
		fetchPlayers: func(ctx context.Context) ([]battlepassPlayer, error) {
			return players, nil
		},
		awardIntel: func(ctx context.Context, pawnID, amount int64) error {
			if awardErr != nil {
				return awardErr
			}
			if awarded != nil {
				*awarded = append(*awarded, pawnID, amount)
			}
			return nil
		},
		giveItem: func(ctx context.Context, actorID int64, template string, qty, quality int64) error {
			return nil
		},
	}
}

func TestGrantBattlepassEarned_DeliversItemRewards(t *testing.T) {
	s := testBattlepassStore(t)
	rewarded := testTiers()
	rewarded[0].RewardItems = `[{"template":"Kindjal_4","qty":2,"quality":3}]`
	if _, err := s.seedTiersIfEmpty(rewarded); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)
	_ = s.recordClaim("journey:DA_MQ_FindTheFremen", 1, 40, battlepassClaimEarned)

	type given struct {
		actorID  int64
		template string
		qty      int64
		quality  int64
	}
	var items []given
	deps := grantTestDeps([]battlepassPlayer{{AccountID: 1, PawnID: 100}}, nil, nil)
	deps.giveItem = func(ctx context.Context, actorID int64, template string, qty, quality int64) error {
		items = append(items, given{actorID, template, qty, quality})
		return nil
	}

	if _, _, err := grantBattlepassEarned(context.Background(), s, deps, 1); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("gave %d items, want 1 (only level:5 has rewards)", len(items))
	}
	if items[0] != (given{100, "Kindjal_4", 2, 3}) {
		t.Fatalf("gave %+v", items[0])
	}
}

func TestGrantBattlepassEarned_Success(t *testing.T) {
	s := testBattlepassStore(t)
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)
	_ = s.recordClaim("level:10", 1, 15, battlepassClaimEarned)
	_ = s.recordClaim("level:15", 1, 20, battlepassClaimBaseline)

	var awarded []int64
	deps := grantTestDeps([]battlepassPlayer{{AccountID: 1, PawnID: 100, Online: false}}, nil, &awarded)

	total, n, err := grantBattlepassEarned(context.Background(), s, deps, 1)
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if total != 25 || n != 2 {
		t.Fatalf("granted %d intel over %d tiers, want 25/2", total, n)
	}
	if len(awarded) != 2 || awarded[0] != 100 || awarded[1] != 25 {
		t.Fatalf("awardIntel called with %v, want [100 25]", awarded)
	}
	keys, _ := s.claimedKeys(1)
	if keys["level:5"] != battlepassClaimGranted || keys["level:10"] != battlepassClaimGranted {
		t.Fatalf("claims after grant = %v, want granted", keys)
	}
	if keys["level:15"] != battlepassClaimBaseline {
		t.Fatal("baseline claim must remain baseline")
	}
}

func TestGrantBattlepassEarned_NothingEarned(t *testing.T) {
	s := testBattlepassStore(t)
	deps := grantTestDeps([]battlepassPlayer{{AccountID: 1, PawnID: 100}}, nil, nil)
	if _, _, err := grantBattlepassEarned(context.Background(), s, deps, 1); err != errBattlepassNothingEarned {
		t.Fatalf("err = %v, want errBattlepassNothingEarned", err)
	}
}

func TestGrantBattlepassEarned_UnknownAccount(t *testing.T) {
	s := testBattlepassStore(t)
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)
	deps := grantTestDeps([]battlepassPlayer{{AccountID: 2, PawnID: 200}}, nil, nil)
	if _, _, err := grantBattlepassEarned(context.Background(), s, deps, 1); err != errNotFound {
		t.Fatalf("err = %v, want errNotFound", err)
	}
}

func TestGrantBattlepassEarned_AwardFailureKeepsEarned(t *testing.T) {
	s := testBattlepassStore(t)
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)
	deps := grantTestDeps([]battlepassPlayer{{AccountID: 1, PawnID: 100, Online: true}},
		fmt.Errorf("player is currently Online"), nil)

	if _, _, err := grantBattlepassEarned(context.Background(), s, deps, 1); err == nil {
		t.Fatal("expected grant error")
	}
	claims, _ := s.listClaims(1)
	if claims[0].Status != battlepassClaimEarned || claims[0].Attempts != 1 || claims[0].LastError == "" {
		t.Fatalf("claim after failed grant = %+v, want earned with attempt recorded", claims[0])
	}
}

// ── config ────────────────────────────────────────────────────────────────────

func TestHandleGetBattlepassConfig(t *testing.T) {
	orig := loadedConfig
	t.Cleanup(func() { loadedConfig = orig })

	enabled := true
	loadedConfig = appConfig{
		BattlepassEnabled:          &enabled,
		BattlepassAwardPast:        nil,
		BattlepassPollSeconds:      120,
		BattlepassScanPaceMs:       100,
		BattlepassScanStartDelayMs: 5000,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/battlepass/config", nil)
	rec := httptest.NewRecorder()
	handleGetBattlepassConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got battlepassConfigPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Enabled == nil || !*got.Enabled {
		t.Errorf("Enabled = %v, want true", got.Enabled)
	}
	if got.AwardPast != nil {
		t.Errorf("AwardPast = %v, want nil (unset)", got.AwardPast)
	}
	if got.PollSeconds != 120 {
		t.Errorf("PollSeconds = %d, want 120", got.PollSeconds)
	}
	if got.ScanPaceMs != 100 {
		t.Errorf("ScanPaceMs = %d, want 100", got.ScanPaceMs)
	}
	if got.ScanStartDelayMs != 5000 {
		t.Errorf("ScanStartDelayMs = %d, want 5000", got.ScanStartDelayMs)
	}
}

func TestHandleSaveBattlepassConfig(t *testing.T) {
	orig := loadedConfig
	t.Cleanup(func() { loadedConfig = orig })

	// Redirect config writes to a temp dir so we don't touch the real config.
	t.Setenv("DUNE_ADMIN_CONFIG_DIR", t.TempDir())

	body, _ := json.Marshal(battlepassConfigPayload{
		Enabled:          boolPtr(true),
		AwardPast:        boolPtr(false),
		PollSeconds:      90,
		ScanPaceMs:       50,
		ScanStartDelayMs: 2000,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/battlepass/config", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handleSaveBattlepassConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got battlepassConfigPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Enabled == nil || !*got.Enabled {
		t.Errorf("Enabled = %v, want true", got.Enabled)
	}
	if got.PollSeconds != 90 {
		t.Errorf("PollSeconds = %d, want 90", got.PollSeconds)
	}
	// Verify loadedConfig was updated in-memory.
	if loadedConfig.BattlepassPollSeconds != 90 {
		t.Errorf("loadedConfig.PollSeconds = %d, want 90", loadedConfig.BattlepassPollSeconds)
	}
}

func TestHandleSaveBattlepassConfig_BadJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/api/v1/battlepass/config", bytes.NewReader([]byte(`{bad`)))
	rec := httptest.NewRecorder()
	handleSaveBattlepassConfig(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestApplyBattlepassEngine_StopsRunningEngine(t *testing.T) {
	// globalBattlepassStore == nil in unit tests, so applyBattlepassEngine returns
	// before reaching the cancel logic. We test the cancel directly via stopBattlepassEngine.
	called := false
	globalBattlepassMu.Lock()
	globalBattlepassCancel = func() { called = true }
	globalBattlepassMu.Unlock()
	t.Cleanup(func() {
		globalBattlepassMu.Lock()
		globalBattlepassCancel = nil
		globalBattlepassMu.Unlock()
	})

	stopBattlepassEngine()

	if !called {
		t.Error("expected existing cancel to be called by stopBattlepassEngine")
	}
	globalBattlepassMu.Lock()
	nilAfter := globalBattlepassCancel == nil
	globalBattlepassMu.Unlock()
	if !nilAfter {
		t.Error("expected globalBattlepassCancel to be nil after stop")
	}
}

// ── grant-tier ────────────────────────────────────────────────────────────────

func TestGrantBattlepassTier_success(t *testing.T) {
	s := testBattlepassStore(t)
	tiers := testTiers()
	tiers[0].RewardItems = `[{"template":"Kindjal_4","qty":1,"quality":3}]`
	if _, err := s.seedTiersIfEmpty(tiers); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)
	_ = s.recordClaim("journey:DA_MQ_FindTheFremen", 1, 40, battlepassClaimEarned)

	var awarded []int64
	var givenItems []string
	deps := grantTestDeps([]battlepassPlayer{{AccountID: 1, PawnID: 100}}, nil, &awarded)
	deps.giveItem = func(_ context.Context, _ int64, template string, qty, _ int64) error {
		givenItems = append(givenItems, template)
		return nil
	}

	intel, err := grantBattlepassTier(context.Background(), s, deps, 1, "level:5")
	if err != nil {
		t.Fatalf("grantBattlepassTier: %v", err)
	}
	if intel != 10 {
		t.Errorf("granted intel = %d, want 10", intel)
	}
	// Only level:5 claim should be granted.
	keys, _ := s.claimedKeys(1)
	if keys["level:5"] != battlepassClaimGranted {
		t.Errorf("level:5 = %q, want granted", keys["level:5"])
	}
	if keys["journey:DA_MQ_FindTheFremen"] != battlepassClaimEarned {
		t.Errorf("journey claim = %q, want earned (untouched)", keys["journey:DA_MQ_FindTheFremen"])
	}
	if len(givenItems) != 1 || givenItems[0] != "Kindjal_4" {
		t.Errorf("givenItems = %v, want [Kindjal_4]", givenItems)
	}
}

func TestGrantBattlepassTier_notEarned(t *testing.T) {
	s := testBattlepassStore(t)
	deps := grantTestDeps([]battlepassPlayer{{AccountID: 1, PawnID: 100}}, nil, nil)

	if _, err := grantBattlepassTier(context.Background(), s, deps, 1, "level:5"); !errors.Is(err, errBattlepassNothingEarned) {
		t.Fatalf("want errBattlepassNothingEarned, got %v", err)
	}
}

func TestGrantBattlepassTier_playerNotFound(t *testing.T) {
	s := testBattlepassStore(t)
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)
	deps := grantTestDeps([]battlepassPlayer{{AccountID: 99, PawnID: 999}}, nil, nil)

	if _, err := grantBattlepassTier(context.Background(), s, deps, 1, "level:5"); !errors.Is(err, errNotFound) {
		t.Fatalf("want errNotFound, got %v", err)
	}
}

func TestGrantBattlepassTier_intelFailure_recordsOnTier(t *testing.T) {
	s := testBattlepassStore(t)
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)
	_ = s.recordClaim("level:10", 1, 15, battlepassClaimEarned)
	deps := grantTestDeps([]battlepassPlayer{{AccountID: 1, PawnID: 100}}, fmt.Errorf("db offline"), nil)

	if _, err := grantBattlepassTier(context.Background(), s, deps, 1, "level:5"); err == nil {
		t.Fatal("expected error")
	}
	claims, _ := s.listClaims(1)
	byKey := map[string]battlepassClaim{}
	for _, c := range claims {
		byKey[c.TierKey] = c
	}
	if byKey["level:5"].Attempts != 1 || byKey["level:5"].LastError == "" {
		t.Errorf("level:5 = %+v, want attempts=1 and last_error set", byKey["level:5"])
	}
	if byKey["level:10"].Attempts != 0 {
		t.Errorf("level:10 attempts = %d, want 0 (untouched)", byKey["level:10"].Attempts)
	}
}

func TestHandleBattlepassGrantTier(t *testing.T) {
	s := setupBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)

	// Use nil globalDB — productionBattlepassGrantDeps will fail player lookup.
	// Override with test deps via the handler's seam. Since handleBattlepassGrantTier
	// calls productionBattlepassGrantDeps() directly (no seam yet), we instead test
	// via grantBattlepassTier directly and verify the HTTP plumbing separately.
	// HTTP plumbing: bad JSON → 400.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/grant-tier", bytes.NewReader([]byte(`{bad`)))
	rec := httptest.NewRecorder()
	handleBattlepassGrantTier(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad JSON: want 400, got %d", rec.Code)
	}

	// Missing account_id → 400.
	body, _ := json.Marshal(map[string]any{"tier_key": "level:5"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/grant-tier", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	handleBattlepassGrantTier(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing account_id: want 400, got %d", rec.Code)
	}

	// Missing tier_key → 400.
	body, _ = json.Marshal(map[string]any{"account_id": 1})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/grant-tier", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	handleBattlepassGrantTier(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing tier_key: want 400, got %d", rec.Code)
	}
}

// ── default count ─────────────────────────────────────────────────────────────

func TestHandleListBattlepassTiers_DefaultCount(t *testing.T) {
	s := setupBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/battlepass/tiers", nil)
	rec := httptest.NewRecorder()
	handleListBattlepassTiers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		DefaultCount int `json:"default_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := len(defaultBattlepassCatalog())
	if resp.DefaultCount != want {
		t.Fatalf("default_count = %d, want %d", resp.DefaultCount, want)
	}
}

// ── create ────────────────────────────────────────────────────────────────────

func TestHandleCreateBattlepassTier(t *testing.T) {
	setupBattlepassStore(t)

	validBody := func(overrides map[string]any) []byte {
		base := map[string]any{
			"tier_key":  "level:99",
			"category":  "level",
			"label":     "Level 99",
			"signal":    "level",
			"threshold": 99,
			"intel":     25,
			"enabled":   true,
		}
		for k, v := range overrides {
			base[k] = v
		}
		b, _ := json.Marshal(base)
		return b
	}

	// Validation failures.
	cases := []struct {
		name string
		body []byte
	}{
		{"empty tier_key", validBody(map[string]any{"tier_key": ""})},
		{"bad signal", validBody(map[string]any{"signal": "bad"})},
		{"neg intel", validBody(map[string]any{"intel": -1})},
		{"level no threshold", validBody(map[string]any{"threshold": 0})},
		{"journey no signal_key", validBody(map[string]any{"signal": "journey_node", "signal_key": "", "threshold": 0})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/tiers", bytes.NewReader(c.body))
			rec := httptest.NewRecorder()
			handleCreateBattlepassTier(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}

	// Success.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/tiers", bytes.NewReader(validBody(nil)))
	rec := httptest.NewRecorder()
	handleCreateBattlepassTier(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("success: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var tier battlepassTier
	if err := json.Unmarshal(rec.Body.Bytes(), &tier); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if tier.ID == 0 || tier.TierKey != "level:99" || tier.Intel != 25 {
		t.Fatalf("created tier = %+v", tier)
	}

	// Duplicate tier_key → 409.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/tiers", bytes.NewReader(validBody(nil)))
	rec = httptest.NewRecorder()
	handleCreateBattlepassTier(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate: want 409, got %d", rec.Code)
	}
}

// ── full-fields update ────────────────────────────────────────────────────────

func TestHandleUpdateBattlepassTier_FullFields(t *testing.T) {
	s := setupBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tiers, _ := s.listTiers()
	// tiers[1] = journey:DA_MQ_FindTheFremen, category=story, signal=journey_node.
	id := fmt.Sprintf("%d", tiers[1].ID)

	body, _ := json.Marshal(map[string]any{
		"intel": 55, "enabled": true, "label": "Updated Journey",
		"category": "faction", "signal": "journey_node", "signal_key": "DA_NEW_NODE",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/battlepass/tiers/"+id, bytes.NewReader(body))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	handleUpdateBattlepassTier(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var tier battlepassTier
	_ = json.Unmarshal(rec.Body.Bytes(), &tier)
	if tier.Category != "faction" || tier.SignalKey != "DA_NEW_NODE" || tier.Label != "Updated Journey" {
		t.Fatalf("full update result = %+v", tier)
	}

	// tier_key in body is ignored.
	body, _ = json.Marshal(map[string]any{"tier_key": "should-be-ignored", "intel": 55, "enabled": true})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/battlepass/tiers/"+id, bytes.NewReader(body))
	req.SetPathValue("id", id)
	rec = httptest.NewRecorder()
	handleUpdateBattlepassTier(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tier_key-ignore: want 200, got %d", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tier)
	if tier.TierKey != "journey:DA_MQ_FindTheFremen" {
		t.Fatalf("tier_key changed to %q, must be immutable", tier.TierKey)
	}
}

// ── export ────────────────────────────────────────────────────────────────────

func TestHandleExportBattlepassCatalog(t *testing.T) {
	s := setupBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/battlepass/export", nil)
	rec := httptest.NewRecorder()
	handleExportBattlepassCatalog(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var envelope battlepassCatalogEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.Version != 1 {
		t.Fatalf("version = %d, want 1", envelope.Version)
	}
	if len(envelope.Tiers) != 3 {
		t.Fatalf("tiers len = %d, want 3", len(envelope.Tiers))
	}
	for _, tier := range envelope.Tiers {
		if tier.TierKey == "" {
			t.Errorf("exported tier has empty tier_key: %+v", tier)
		}
	}

	// Verify no "id" field in raw JSON.
	var raw map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	for _, tr := range raw["tiers"].([]any) {
		if _, ok := tr.(map[string]any)["id"]; ok {
			t.Errorf("export tier must not contain 'id' field: %+v", tr)
		}
	}
}

// ── import ────────────────────────────────────────────────────────────────────

func TestHandleImportBattlepassCatalog(t *testing.T) {
	s := setupBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimGranted)

	importTiers := []battlepassTierExport{
		{TierKey: "level:5", Category: "level", Label: "Imported Level 5", Signal: battlepassSignalLevel, Threshold: 5, Intel: 20, Enabled: true},
		{TierKey: "journey:new", Category: "story", Label: "New Journey", Signal: battlepassSignalJourneyNode, SignalKey: "DA_NEW_NODE", Intel: 50, Enabled: true},
	}
	body, _ := json.Marshal(battlepassCatalogEnvelope{Version: 1, Tiers: importTiers})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/import", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handleImportBattlepassCatalog(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Imported int `json:"imported"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Imported != 2 {
		t.Fatalf("imported = %d, want 2", resp.Imported)
	}

	// Catalog is replaced.
	tiers, _ := s.listTiers()
	if len(tiers) != 2 {
		t.Fatalf("after import %d tiers, want 2", len(tiers))
	}
	// Claims survive.
	keys, _ := s.claimedKeys(1)
	if keys["level:5"] != battlepassClaimGranted {
		t.Fatalf("claim lost after import: %v", keys)
	}

	// Invalid tier (bad signal) → 400.
	badBody, _ := json.Marshal(battlepassCatalogEnvelope{Version: 1, Tiers: []battlepassTierExport{
		{TierKey: "bad:1", Category: "level", Label: "Bad", Signal: "invalid", Threshold: 5, Intel: 10, Enabled: true},
	}})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/import", bytes.NewReader(badBody))
	rec = httptest.NewRecorder()
	handleImportBattlepassCatalog(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad signal: want 400, got %d", rec.Code)
	}

	// Duplicate tier_key within payload → 400.
	dupBody, _ := json.Marshal(battlepassCatalogEnvelope{Version: 1, Tiers: []battlepassTierExport{
		{TierKey: "dup:1", Category: "level", Label: "Dup 1", Signal: battlepassSignalLevel, Threshold: 5, Intel: 10, Enabled: true},
		{TierKey: "dup:1", Category: "level", Label: "Dup 2", Signal: battlepassSignalLevel, Threshold: 5, Intel: 10, Enabled: true},
	}})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/import", bytes.NewReader(dupBody))
	rec = httptest.NewRecorder()
	handleImportBattlepassCatalog(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("dup keys: want 400, got %d", rec.Code)
	}

	// Empty tiers → 400.
	emptyBody, _ := json.Marshal(battlepassCatalogEnvelope{Version: 1, Tiers: []battlepassTierExport{}})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/battlepass/import", bytes.NewReader(emptyBody))
	rec = httptest.NewRecorder()
	handleImportBattlepassCatalog(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty tiers: want 400, got %d", rec.Code)
	}
}

func TestHandleGetBattlepassPending_tierFormat(t *testing.T) {
	s := setupBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)
	_ = s.recordClaim("level:5", 2, 10, battlepassClaimBaseline)
	_ = s.recordClaim("journey:DA_MQ_FindTheFremen", 1, 40, battlepassClaimGranted)
	_ = s.recordClaim("tag:Exploration.Cave.Large.Altar1", 3, 5, battlepassClaimEarned)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/battlepass/pending", nil)
	rec := httptest.NewRecorder()
	handleBattlepassPending(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var rows []struct {
		AccountID int64  `json:"account_id"`
		TierKey   string `json:"tier_key"`
		TierLabel string `json:"tier_label"`
		Intel     int64  `json:"intel"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 tier rows (only earned), got %d: %+v", len(rows), rows)
	}
	// Verify tier_label is populated from the join.
	for _, r := range rows {
		if r.TierLabel == "" {
			t.Errorf("row %+v has empty tier_label", r)
		}
	}
}
