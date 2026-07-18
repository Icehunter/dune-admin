package main

import (
	"errors"
	"testing"
)

func testBattlepassStore(t *testing.T) *battlepassStore {
	t.Helper()
	s, err := openBattlepassStore(":memory:")
	if err != nil {
		t.Fatalf("openBattlepassStore: %v", err)
	}
	t.Cleanup(func() { _ = s.db.Close() })
	return s
}

func testTiers() []battlepassTier {
	return []battlepassTier{
		{TierKey: "level:5", Category: "level", Label: "Level 5", Signal: battlepassSignalLevel, Threshold: 5, Intel: 10, Enabled: true},
		{TierKey: "journey:DA_MQ_FindTheFremen", Category: "story", Label: "Find the Fremen", Signal: battlepassSignalJourneyNode, SignalKey: "DA_MQ_FindTheFremen", Intel: 40, Enabled: true},
		{TierKey: "tag:Exploration.Cave.Large.Altar1", Category: "exploration", Label: "Altar 1", Signal: battlepassSignalPlayerTag, SignalKey: "Exploration.Cave.Large.Altar1", Intel: 5, Enabled: true},
	}
}

func TestBattlepassStoreSeedIfEmpty(t *testing.T) {
	s := testBattlepassStore(t)

	n, err := s.seedTiersIfEmpty(testTiers())
	if err != nil {
		t.Fatalf("seedTiersIfEmpty: %v", err)
	}
	if n != 3 {
		t.Fatalf("seeded %d tiers, want 3", n)
	}

	// Second seed must be a no-op.
	n, err = s.seedTiersIfEmpty(testTiers())
	if err != nil {
		t.Fatalf("second seedTiersIfEmpty: %v", err)
	}
	if n != 0 {
		t.Fatalf("second seed inserted %d tiers, want 0", n)
	}

	tiers, err := s.listTiers()
	if err != nil {
		t.Fatalf("listTiers: %v", err)
	}
	if len(tiers) != 3 {
		t.Fatalf("listTiers returned %d, want 3", len(tiers))
	}
	if tiers[0].TierKey != "level:5" || tiers[0].Intel != 10 || !tiers[0].Enabled {
		t.Fatalf("unexpected first tier: %+v", tiers[0])
	}
}

// TestBattlepassSeedIfEmpty_PreservesEdits guards the boot behavior change:
// applyBattlepassEngine now seeds-if-empty instead of reseeding, so an operator
// edit to a tier must survive a subsequent seed (i.e. a restart / config save).
func TestBattlepassSeedIfEmpty_PreservesEdits(t *testing.T) {
	s := testBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Operator edits a tier's intel reward.
	if _, err := s.db.Exec(`UPDATE battlepass_tiers SET intel = 999 WHERE tier_key = 'level:5'`); err != nil {
		t.Fatalf("edit tier: %v", err)
	}
	// A later boot/config-save seeds again — must NOT clobber the edit.
	if n, err := s.seedTiersIfEmpty(testTiers()); err != nil || n != 0 {
		t.Fatalf("reseed-if-empty: n=%d err=%v (want 0/nil — no clobber)", n, err)
	}
	tiers, err := s.listTiers()
	if err != nil {
		t.Fatalf("listTiers: %v", err)
	}
	for _, tr := range tiers {
		if tr.TierKey == "level:5" && tr.Intel != 999 {
			t.Errorf("operator edit lost: level:5 intel = %d, want 999", tr.Intel)
		}
	}
}

func TestBattlepassStoreGetTierNotFound(t *testing.T) {
	s := testBattlepassStore(t)
	if _, err := s.getTier(99); err != errNotFound {
		t.Fatalf("getTier(99) err = %v, want errNotFound", err)
	}
}

func TestBattlepassStoreUpdateTier(t *testing.T) {
	s := testBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tiers, _ := s.listTiers()
	id := tiers[0].ID

	rewards := `[{"template":"Kindjal_4","qty":1,"quality":3}]`
	// tiers[0] is level:5 (category=level, signal=level, threshold=5).
	got, err := s.updateTier(id, "Renamed Tier", 25, false, rewards, "level", battlepassSignalLevel, "", 5)
	if err != nil {
		t.Fatalf("updateTier: %v", err)
	}
	if got.Intel != 25 || got.Enabled || got.Label != "Renamed Tier" || got.RewardItems != rewards {
		t.Fatalf("updateTier result = %+v", got)
	}

	if _, err := s.updateTier(9999, "x", 1, true, "", "level", battlepassSignalLevel, "", 5); err != errNotFound {
		t.Fatalf("updateTier missing err = %v, want errNotFound", err)
	}
}

func TestBattlepassStoreCreateTier(t *testing.T) {
	s := testBattlepassStore(t)

	t.Run("success returns populated row", func(t *testing.T) {
		tier := battlepassTier{
			TierKey: "level:5", Category: "level", Label: "Level 5",
			Signal: battlepassSignalLevel, Threshold: 5, Intel: 10, Enabled: true,
		}
		got, err := s.createTier(tier)
		if err != nil {
			t.Fatalf("createTier: %v", err)
		}
		if got.ID == 0 || got.TierKey != "level:5" || got.Category != "level" || got.Intel != 10 {
			t.Fatalf("createTier result = %+v", got)
		}
	})

	t.Run("duplicate tier_key returns errBattlepassDuplicateTierKey", func(t *testing.T) {
		dup := battlepassTier{
			TierKey: "level:5", Category: "level", Label: "Duplicate",
			Signal: battlepassSignalLevel, Threshold: 5, Intel: 5, Enabled: true,
		}
		if _, err := s.createTier(dup); !errors.Is(err, errBattlepassDuplicateTierKey) {
			t.Fatalf("want errBattlepassDuplicateTierKey, got %v", err)
		}
	})
}

func TestBattlepassStoreUpdateTier_FullFields(t *testing.T) {
	s := testBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tiers, _ := s.listTiers()
	// tiers[1] is journey:DA_MQ_FindTheFremen (category=story, signal=journey_node).
	id := tiers[1].ID

	got, err := s.updateTier(id, "Renamed", 99, false, "", "faction", battlepassSignalJourneyNode, "DA_NEW_NODE", 0)
	if err != nil {
		t.Fatalf("updateTier: %v", err)
	}
	if got.Category != "faction" || got.SignalKey != "DA_NEW_NODE" || got.Label != "Renamed" {
		t.Fatalf("full-fields update did not persist: %+v", got)
	}
	// tier_key must not change.
	if got.TierKey != "journey:DA_MQ_FindTheFremen" {
		t.Fatalf("tier_key changed to %q, must be immutable", got.TierKey)
	}
}

func TestBattlepassStoreSchemaIdempotent(t *testing.T) {
	s := testBattlepassStore(t)
	// Re-applying the schema (as applyUnifiedSchema does on every startup)
	// must not fail on the reward_items migration.
	if err := initBattlepassSchema(s.db); err != nil {
		t.Fatalf("second initBattlepassSchema: %v", err)
	}
}

func TestBattlepassStoreDeleteTier(t *testing.T) {
	s := testBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tiers, _ := s.listTiers()

	if err := s.deleteTiers([]int64{tiers[0].ID}); err != nil {
		t.Fatalf("deleteTiers: %v", err)
	}
	after, _ := s.listTiers()
	if len(after) != 2 {
		t.Fatalf("after delete %d tiers, want 2", len(after))
	}
}

func TestBattlepassStoreBulkSetEnabled(t *testing.T) {
	s := testBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tiers, _ := s.listTiers()
	ids := []int64{tiers[0].ID, tiers[1].ID}

	if err := s.setTiersEnabled(ids, false); err != nil {
		t.Fatalf("setTiersEnabled: %v", err)
	}
	after, _ := s.listTiers()
	for _, tier := range after {
		want := tier.ID == tiers[2].ID
		if tier.Enabled != want {
			t.Fatalf("tier %d enabled = %t, want %t", tier.ID, tier.Enabled, want)
		}
	}

	if err := s.setTiersEnabled(nil, true); err != nil {
		t.Fatalf("setTiersEnabled empty: %v", err)
	}
}

func TestBattlepassStoreRecordClaimIdempotent(t *testing.T) {
	s := testBattlepassStore(t)

	if err := s.recordClaim("level:5", 42, 10, battlepassClaimEarned); err != nil {
		t.Fatalf("recordClaim: %v", err)
	}
	// Re-recording must not downgrade or duplicate.
	if err := s.recordClaim("level:5", 42, 10, battlepassClaimBaseline); err != nil {
		t.Fatalf("recordClaim repeat: %v", err)
	}

	keys, err := s.claimedKeys(42)
	if err != nil {
		t.Fatalf("claimedKeys: %v", err)
	}
	if keys["level:5"] != battlepassClaimEarned {
		t.Fatalf("claim status = %q, want earned", keys["level:5"])
	}
	if len(keys) != 1 {
		t.Fatalf("claimedKeys len = %d, want 1", len(keys))
	}
}

func TestBattlepassStoreBaselineNotGrantable(t *testing.T) {
	s := testBattlepassStore(t)

	if err := s.recordClaim("level:5", 42, 10, battlepassClaimBaseline); err != nil {
		t.Fatalf("recordClaim baseline: %v", err)
	}
	if err := s.recordClaim("level:10", 42, 15, battlepassClaimEarned); err != nil {
		t.Fatalf("recordClaim earned: %v", err)
	}

	claims, err := s.earnedClaims(42)
	if err != nil {
		t.Fatalf("earnedClaims: %v", err)
	}
	if len(claims) != 1 || claims[0].TierKey != "level:10" || claims[0].Intel != 15 {
		t.Fatalf("earnedClaims = %+v, want only level:10/15", claims)
	}
}

func TestBattlepassStoreEarnedTotals(t *testing.T) {
	s := testBattlepassStore(t)

	mustRecord := func(key string, account, intel int64, status string) {
		t.Helper()
		if err := s.recordClaim(key, account, intel, status); err != nil {
			t.Fatalf("recordClaim %s/%d: %v", key, account, err)
		}
	}
	mustRecord("level:5", 1, 10, battlepassClaimEarned)
	mustRecord("level:10", 1, 15, battlepassClaimEarned)
	mustRecord("level:5", 2, 10, battlepassClaimBaseline)
	mustRecord("level:5", 3, 10, battlepassClaimEarned)

	totals, err := s.earnedTotals()
	if err != nil {
		t.Fatalf("earnedTotals: %v", err)
	}
	if totals[1] != 25 || totals[3] != 10 {
		t.Fatalf("earnedTotals = %v, want {1:25 3:10}", totals)
	}
	if _, ok := totals[2]; ok {
		t.Fatalf("baseline-only account 2 must not appear in earnedTotals: %v", totals)
	}
}

func TestBattlepassStoreMarkGrantedForAccount(t *testing.T) {
	s := testBattlepassStore(t)

	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)
	_ = s.recordClaim("level:10", 1, 15, battlepassClaimEarned)
	_ = s.recordClaim("level:5", 2, 10, battlepassClaimEarned)

	if err := s.markGrantedForAccount(1); err != nil {
		t.Fatalf("markGrantedForAccount: %v", err)
	}

	keys1, _ := s.claimedKeys(1)
	if keys1["level:5"] != battlepassClaimGranted || keys1["level:10"] != battlepassClaimGranted {
		t.Fatalf("account 1 claims = %v, want all granted", keys1)
	}
	keys2, _ := s.claimedKeys(2)
	if keys2["level:5"] != battlepassClaimEarned {
		t.Fatalf("account 2 claim = %v, must stay earned", keys2)
	}

	claims, _ := s.listClaims(1)
	for _, c := range claims {
		if c.GrantedAt == "" {
			t.Fatalf("granted claim missing granted_at: %+v", c)
		}
	}
}

func TestBattlepassStoreRecordGrantFailure(t *testing.T) {
	s := testBattlepassStore(t)
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)

	if err := s.recordGrantFailure(1, "player is online"); err != nil {
		t.Fatalf("recordGrantFailure: %v", err)
	}

	claims, _ := s.listClaims(1)
	if len(claims) != 1 {
		t.Fatalf("listClaims len = %d, want 1", len(claims))
	}
	c := claims[0]
	if c.Status != battlepassClaimEarned || c.Attempts != 1 || c.LastError != "player is online" {
		t.Fatalf("claim after failure = %+v, want earned/attempts=1/last_error set", c)
	}
}

// TestBattlepassReseedUpdatesCategoryAndLabel verifies that reseeding an
// existing tier updates its mutable fields (category, label, intel) even
// when the tier_key stays the same — the scenario that occurs when the
// catalog source changes a tier's category between server restarts.
func TestBattlepassReseedUpdatesCategoryAndLabel(t *testing.T) {
	s := testBattlepassStore(t)

	old := []battlepassTier{{
		TierKey:  "journey:DA_LDR_Combat_DecapitationStrike_01",
		Category: "contracts", Label: "Contract: Decapitation Strike",
		Signal: battlepassSignalJourneyNode, SignalKey: "DA_LDR_Combat_DecapitationStrike_01",
		Intel: 5, Enabled: true,
	}}
	if err := s.reseedTiers(old); err != nil {
		t.Fatalf("initial reseed: %v", err)
	}

	updated := []battlepassTier{{
		TierKey:  "journey:DA_LDR_Combat_DecapitationStrike_01",
		Category: "faction", Label: "Contract: Decapitation Strike",
		Signal: battlepassSignalJourneyNode, SignalKey: "DA_LDR_Combat_DecapitationStrike_01",
		Intel: 5, Enabled: true,
	}}
	if err := s.reseedTiers(updated); err != nil {
		t.Fatalf("reseed with updated category: %v", err)
	}

	tiers, err := s.listTiers()
	if err != nil {
		t.Fatalf("listTiers: %v", err)
	}
	if len(tiers) != 1 {
		t.Fatalf("want 1 tier, got %d", len(tiers))
	}
	if tiers[0].Category != "faction" {
		t.Errorf("category = %q, want %q", tiers[0].Category, "faction")
	}
}

func TestBattlepassStoreReseedPreservesClaims(t *testing.T) {
	s := testBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimGranted)

	// Reseed with a changed catalog (extra tier).
	newCatalog := append(testTiers(), battlepassTier{
		TierKey: "level:10", Category: "level", Label: "Level 10",
		Signal: battlepassSignalLevel, Threshold: 10, Intel: 10, Enabled: true,
	})
	if err := s.reseedTiers(newCatalog); err != nil {
		t.Fatalf("reseedTiers: %v", err)
	}

	tiers, _ := s.listTiers()
	if len(tiers) != 4 {
		t.Fatalf("after reseed, %d tiers, want 4", len(tiers))
	}
	keys, _ := s.claimedKeys(1)
	if keys["level:5"] != battlepassClaimGranted {
		t.Fatalf("reseed lost claims: %v", keys)
	}
}

func TestEarnedClaimsWithTiers_empty(t *testing.T) {
	s := testBattlepassStore(t)
	rows, err := s.earnedClaimsWithTiers()
	if err != nil {
		t.Fatalf("earnedClaimsWithTiers: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("want 0 rows, got %d", len(rows))
	}
}

func TestEarnedClaimsWithTiers_onlyEarned(t *testing.T) {
	s := testBattlepassStore(t)
	if _, err := s.seedTiersIfEmpty(testTiers()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)
	_ = s.recordClaim("level:5", 2, 10, battlepassClaimBaseline)
	_ = s.recordClaim("journey:DA_MQ_FindTheFremen", 1, 40, battlepassClaimGranted)
	_ = s.recordClaim("tag:Exploration.Cave.Large.Altar1", 3, 5, battlepassClaimEarned)

	rows, err := s.earnedClaimsWithTiers()
	if err != nil {
		t.Fatalf("earnedClaimsWithTiers: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 earned rows, got %d: %+v", len(rows), rows)
	}
	// Verify tier label comes through from the join.
	labels := map[string]string{}
	for _, r := range rows {
		labels[r.TierKey] = r.TierLabel
	}
	if labels["level:5"] != "Level 5" {
		t.Errorf("tier label for level:5 = %q, want %q", labels["level:5"], "Level 5")
	}
	if labels["tag:Exploration.Cave.Large.Altar1"] != "Altar 1" {
		t.Errorf("tier label for tag:Exploration.Cave.Large.Altar1 = %q, want %q", labels["tag:Exploration.Cave.Large.Altar1"], "Altar 1")
	}
}

func TestEarnedClaimsWithTiers_noTierLabel(t *testing.T) {
	s := testBattlepassStore(t)
	// Claim without a matching tier row — label falls back to tier_key.
	_ = s.recordClaim("orphan:key", 1, 10, battlepassClaimEarned)

	rows, err := s.earnedClaimsWithTiers()
	if err != nil {
		t.Fatalf("earnedClaimsWithTiers: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].TierLabel != "orphan:key" {
		t.Errorf("fallback label = %q, want tier_key", rows[0].TierLabel)
	}
}

func TestEarnedClaim_found(t *testing.T) {
	s := testBattlepassStore(t)
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)

	c, err := s.earnedClaim(1, "level:5")
	if err != nil {
		t.Fatalf("earnedClaim: %v", err)
	}
	if c.TierKey != "level:5" || c.AccountID != 1 || c.Intel != 10 {
		t.Fatalf("claim = %+v, want level:5/account=1/intel=10", c)
	}
}

func TestEarnedClaim_notFound(t *testing.T) {
	s := testBattlepassStore(t)
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimBaseline)

	if _, err := s.earnedClaim(1, "level:5"); !errors.Is(err, errBattlepassNothingEarned) {
		t.Fatalf("want errBattlepassNothingEarned, got %v", err)
	}
	if _, err := s.earnedClaim(1, "nonexistent"); !errors.Is(err, errBattlepassNothingEarned) {
		t.Fatalf("want errBattlepassNothingEarned for missing key, got %v", err)
	}
}

func TestMarkGrantedForTier_singleTier(t *testing.T) {
	s := testBattlepassStore(t)
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)
	_ = s.recordClaim("level:10", 1, 15, battlepassClaimEarned)

	if err := s.markGrantedForTier(1, "level:5"); err != nil {
		t.Fatalf("markGrantedForTier: %v", err)
	}

	keys, _ := s.claimedKeys(1)
	if keys["level:5"] != battlepassClaimGranted {
		t.Errorf("level:5 = %q, want granted", keys["level:5"])
	}
	if keys["level:10"] != battlepassClaimEarned {
		t.Errorf("level:10 = %q, want earned (untouched)", keys["level:10"])
	}
}

func TestRecordGrantFailureForTier_singleTier(t *testing.T) {
	s := testBattlepassStore(t)
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)
	_ = s.recordClaim("level:10", 1, 15, battlepassClaimEarned)

	if err := s.recordGrantFailureForTier(1, "level:5", "timed out"); err != nil {
		t.Fatalf("recordGrantFailureForTier: %v", err)
	}

	claims, _ := s.listClaims(1)
	byKey := map[string]battlepassClaim{}
	for _, c := range claims {
		byKey[c.TierKey] = c
	}
	if byKey["level:5"].Attempts != 1 || byKey["level:5"].LastError != "timed out" {
		t.Errorf("level:5 = %+v, want attempts=1 lastError=timed out", byKey["level:5"])
	}
	if byKey["level:10"].Attempts != 0 {
		t.Errorf("level:10 attempts = %d, want 0 (untouched)", byKey["level:10"].Attempts)
	}
}

// TestBattlepassStoreMarkSeenUnsatisfiedIdempotent covers the per-(account,tier)
// baseline marker (replaces the old per-account isBaselined/markBaselined
// gate): repeated marks for the same tier/account must not duplicate rows or
// error, matching the ON CONFLICT DO NOTHING pattern used elsewhere in this
// store (e.g. recordClaim).
func TestBattlepassStoreMarkSeenUnsatisfiedIdempotent(t *testing.T) {
	s := testBattlepassStore(t)

	if err := s.markSeenUnsatisfied("level:10", 1); err != nil {
		t.Fatalf("markSeenUnsatisfied: %v", err)
	}
	if err := s.markSeenUnsatisfied("level:10", 1); err != nil {
		t.Fatalf("markSeenUnsatisfied repeat: %v", err)
	}

	seen, err := s.seenUnsatisfiedKeys(1)
	if err != nil {
		t.Fatalf("seenUnsatisfiedKeys: %v", err)
	}
	if len(seen) != 1 || !seen["level:10"] {
		t.Fatalf("seenUnsatisfiedKeys = %v, want {level:10: true}", seen)
	}
}

// TestBattlepassStoreSeenUnsatisfiedKeysScopedPerAccount guards against the
// marker leaking across accounts — each account's baseline must be judged
// independently.
func TestBattlepassStoreSeenUnsatisfiedKeysScopedPerAccount(t *testing.T) {
	s := testBattlepassStore(t)

	if err := s.markSeenUnsatisfied("level:10", 1); err != nil {
		t.Fatalf("markSeenUnsatisfied account 1: %v", err)
	}

	seen2, err := s.seenUnsatisfiedKeys(2)
	if err != nil {
		t.Fatalf("seenUnsatisfiedKeys account 2: %v", err)
	}
	if len(seen2) != 0 {
		t.Fatalf("account 2 must not see account 1's markers: %v", seen2)
	}
}

func TestBattlepassStoreCountsByTier(t *testing.T) {
	s := testBattlepassStore(t)
	_ = s.recordClaim("level:5", 1, 10, battlepassClaimEarned)
	_ = s.recordClaim("level:5", 2, 10, battlepassClaimGranted)
	_ = s.recordClaim("level:5", 3, 10, battlepassClaimBaseline)
	_ = s.recordClaim("level:10", 1, 15, battlepassClaimEarned)

	counts, err := s.countsByTier()
	if err != nil {
		t.Fatalf("countsByTier: %v", err)
	}
	c5 := counts["level:5"]
	if c5.Earned != 1 || c5.Granted != 1 || c5.Baseline != 1 {
		t.Fatalf("level:5 counts = %+v, want 1/1/1", c5)
	}
	if counts["level:10"].Earned != 1 {
		t.Fatalf("level:10 counts = %+v", counts["level:10"])
	}
}
