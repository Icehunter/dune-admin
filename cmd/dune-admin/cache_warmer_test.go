package main

import (
	"context"
	"testing"
)

func TestCacheWarmer_WarmAllPopulatesHealthAndBGStatus(t *testing.T) {
	origHealth, origBG := globalHealthCache, globalBGStatusCache
	hc, err := newRistrettoCache[serverHealth]("test-health", 256)
	if err != nil {
		t.Fatalf("newRistrettoCache health: %v", err)
	}
	bc, err := newRistrettoCache[*BattlegroupStatus]("test-bgstatus", 256)
	if err != nil {
		t.Fatalf("newRistrettoCache bgstatus: %v", err)
	}
	globalHealthCache, globalBGStatusCache = hc, bc
	t.Cleanup(func() { globalHealthCache, globalBGStatusCache = origHealth, origBG })

	ctrl := &healthFakeControl{status: &BattlegroupStatus{Phase: "Running", Database: "Connected"}}
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "1", Name: "One", Cfg: ServerConfig{ID: 1, Control: "amp"}, Control: ctrl})
	reg.Register(&ServerContext{ID: "2", Name: "Two", Cfg: ServerConfig{ID: 2, Control: "amp"}, Control: ctrl})

	if _, ok := hc.Get(cacheKey("1", "health")); ok {
		t.Fatal("expected cold miss before warm")
	}

	newCacheWarmer(reg).warmAll(context.Background())

	for _, scope := range []string{"1", "2"} {
		h, ok := hc.Get(cacheKey(scope, "health"))
		if !ok || !h.Running {
			t.Errorf("scope %s: health not warmed (ok=%v running=%v)", scope, ok, h.Running)
		}
		// The Battlegroup tab's status must be warmed too (prewarmed tab).
		st, ok := bc.Get(cacheKey(scope, "bgstatus"))
		if !ok || st == nil || st.Phase != "Running" {
			t.Errorf("scope %s: bgstatus not warmed (ok=%v)", scope, ok)
		}
	}
}

func TestPrewarmCaches_PopulatesBeforeFirstRequest(t *testing.T) {
	origCache := globalHealthCache
	c, err := newRistrettoCache[serverHealth]("test-health", 256)
	if err != nil {
		t.Fatalf("newRistrettoCache: %v", err)
	}
	globalHealthCache = c
	t.Cleanup(func() { globalHealthCache = origCache })

	ctrl := &healthFakeControl{status: &BattlegroupStatus{Phase: "Running", Database: "Connected"}}
	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "1", Name: "One", Cfg: ServerConfig{ID: 1, Control: "amp"}, Control: ctrl})

	prewarmCaches(context.Background(), newCacheWarmer(reg))

	if _, ok := c.Get(cacheKey("1", "health")); !ok {
		t.Error("prewarm did not populate health cache")
	}
}

func TestCacheWarmer_NoCacheIsNoop(t *testing.T) {
	origCache := globalHealthCache
	globalHealthCache = nil
	t.Cleanup(func() { globalHealthCache = origCache })

	reg := newServerRegistry(nil)
	reg.Register(&ServerContext{ID: "1", Name: "One"})
	// Must not panic when the cache is unavailable.
	newCacheWarmer(reg).warmAll(context.Background())
}
