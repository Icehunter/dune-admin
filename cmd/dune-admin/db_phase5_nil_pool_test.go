package main

// Phase 5 — Red tests for pool-parameterised background-worker deps and RMQ
// utility functions. Each test calls a new pool-aware function or dep builder
// with a nil pool and verifies it returns an error rather than panicking.
// These tests fail to compile until the pool params are added (correct RED state).

import (
	"context"
	"testing"
)

// ── Battlepass engine deps ────────────────────────────────────────────────────

func TestProductionBattlepassDeps_NilPool_FetchPlayers(t *testing.T) {
	deps := productionBattlepassDeps(nil)
	_, err := deps.fetchPlayers(context.Background())
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestProductionBattlepassDeps_NilPool_FetchJourneyNodes(t *testing.T) {
	deps := productionBattlepassDeps(nil)
	_, err := deps.fetchCompletedJourneyNodes(context.Background(), 1)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestProductionBattlepassDeps_NilPool_FetchPlayerTags(t *testing.T) {
	deps := productionBattlepassDeps(nil)
	_, err := deps.fetchPlayerTags(context.Background(), 1)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

// ── Events engine deps ────────────────────────────────────────────────────────

func TestProductionEventDeps_NilPool_FetchOnlinePlayers(t *testing.T) {
	deps := productionEventDeps(nil, defaultServerID)
	_, err := deps.fetchOnlinePlayers(context.Background())
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestProductionEventDeps_NilPool_FetchOnlinePositions(t *testing.T) {
	deps := productionEventDeps(nil, defaultServerID)
	_, err := deps.fetchOnlinePositions(context.Background(), []int64{1})
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestProductionEventDeps_NilPool_FetchPlayerLevel(t *testing.T) {
	deps := productionEventDeps(nil, defaultServerID)
	_, err := deps.fetchPlayerLevel(context.Background(), 1)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestProductionEventDeps_NilPool_FetchPlayerTags(t *testing.T) {
	deps := productionEventDeps(nil, defaultServerID)
	_, err := deps.fetchPlayerTags(context.Background(), 1)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestProductionEventDeps_NilPool_GrantCurrency(t *testing.T) {
	deps := productionEventDeps(nil, defaultServerID)
	err := deps.grantCurrency(context.Background(), 1, 100)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestProductionEventDeps_NilPool_GrantItem(t *testing.T) {
	deps := productionEventDeps(nil, defaultServerID)
	err := deps.grantItem(context.Background(), 1, "template", 1, 0)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestProductionEventDeps_NilPool_GrantXP(t *testing.T) {
	deps := productionEventDeps(nil, defaultServerID)
	err := deps.grantXP(context.Background(), 1, "track", 100)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestProductionEventDeps_NilPool_ResolveGrantTargets(t *testing.T) {
	deps := productionEventDeps(nil, defaultServerID)
	_, _, err := deps.resolveGrantTargets(context.Background(), 1)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

// ── RMQ utility functions ─────────────────────────────────────────────────────

func TestFlsIDFromActorIDPool_NilPool(t *testing.T) {
	_, err := flsIDFromActorIDPool(context.Background(), nil, 1)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestPlayerIDDebugPool_NilPool(t *testing.T) {
	_, err := playerIDDebugPool(context.Background(), nil, 1)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestDisplayNameFromHexIDPool_NilPool(t *testing.T) {
	_, err := displayNameFromHexIDPool(context.Background(), nil, "abc")
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestIsHexIDOnlinePool_NilPool(t *testing.T) {
	result := isHexIDOnlinePool(context.Background(), nil, "abc")
	if result {
		t.Error("expected false for nil pool")
	}
}

// ── Inventory capacity ────────────────────────────────────────────────────────

func TestCheckInventoryCapacityPool_NilPool(t *testing.T) {
	err := checkInventoryCapacityPool(context.Background(), nil, 1, "template", 1)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}
