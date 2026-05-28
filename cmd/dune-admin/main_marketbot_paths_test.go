package main

import (
	"path/filepath"
	"testing"
)

func TestResolveEmbeddedMarketBotPaths(t *testing.T) {
	t.Parallel()

	t.Run("uses explicit config values", func(t *testing.T) {
		t.Parallel()
		cfg := appConfig{
			MarketBotCacheDB:  "/tmp/custom-cache.db",
			MarketBotItemData: "/tmp/custom-item-data.json",
			MarketBotState:    "/tmp/custom-state.json",
		}
		cacheDB, itemDataForBot, statePath := resolveEmbeddedMarketBotPaths(cfg, "/fallback.json")
		if cacheDB != "/tmp/custom-cache.db" {
			t.Fatalf("expected explicit cache db path, got %q", cacheDB)
		}
		if itemDataForBot != "/tmp/custom-item-data.json" {
			t.Fatalf("expected explicit item-data path, got %q", itemDataForBot)
		}
		if statePath != "/tmp/custom-state.json" {
			t.Fatalf("expected explicit state path, got %q", statePath)
		}
	})

	t.Run("falls back to provided item-data path", func(t *testing.T) {
		t.Parallel()
		cfg := appConfig{}
		cacheDB, itemDataForBot, statePath := resolveEmbeddedMarketBotPaths(cfg, "/fallback.json")
		wantCache := filepath.Join(configDir(), "market-bot-cache.db")
		if cacheDB != wantCache {
			t.Fatalf("expected default cache path %q, got %q", wantCache, cacheDB)
		}
		if itemDataForBot != "/fallback.json" {
			t.Fatalf("expected fallback item-data path, got %q", itemDataForBot)
		}
		wantState := filepath.Join(configDir(), "market-bot-state.json")
		if statePath != wantState {
			t.Fatalf("expected default state path %q, got %q", wantState, statePath)
		}
	})

	t.Run("marketBotEnabled defaults to true when nil", func(t *testing.T) {
		t.Parallel()
		cfg := appConfig{} // MarketBotEnabled is nil
		if !marketBotEnabled(cfg) {
			t.Error("marketBotEnabled should default to true when field is nil")
		}
	})

	t.Run("marketBotEnabled respects explicit false", func(t *testing.T) {
		t.Parallel()
		f := false
		cfg := appConfig{MarketBotEnabled: &f}
		if marketBotEnabled(cfg) {
			t.Error("marketBotEnabled should return false when explicitly set to false")
		}
	})
}
