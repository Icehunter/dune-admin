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
		}
		cacheDB, itemDataForBot := resolveEmbeddedMarketBotPaths(cfg, "/fallback.json")
		if cacheDB != "/tmp/custom-cache.db" {
			t.Fatalf("expected explicit cache db path, got %q", cacheDB)
		}
		if itemDataForBot != "/tmp/custom-item-data.json" {
			t.Fatalf("expected explicit item-data path, got %q", itemDataForBot)
		}
	})

	t.Run("falls back to provided item-data path", func(t *testing.T) {
		t.Parallel()
		cfg := appConfig{}
		cacheDB, itemDataForBot := resolveEmbeddedMarketBotPaths(cfg, "/fallback.json")
		wantCache := filepath.Join(configDir(), "market-bot-cache.db")
		if cacheDB != wantCache {
			t.Fatalf("expected default cache path %q, got %q", wantCache, cacheDB)
		}
		if itemDataForBot != "/fallback.json" {
			t.Fatalf("expected fallback item-data path, got %q", itemDataForBot)
		}
	})
}
