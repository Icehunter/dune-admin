package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBustPlayersCacheAfter(t *testing.T) {
	origCache := globalPlayersCache
	c, err := newRistrettoCache[[]playerInfo]("test-players", 256)
	if err != nil {
		t.Fatalf("newRistrettoCache: %v", err)
	}
	globalPlayersCache = c
	t.Cleanup(func() { globalPlayersCache = origCache })

	c.Set(cacheKey("1", "players"), []playerInfo{{AccountID: 1}}, time.Minute)
	if _, ok := c.Get(cacheKey("1", "players")); !ok {
		t.Fatal("seed failed")
	}

	called := false
	wrapped := bustPlayersCacheAfter(func(http.ResponseWriter, *http.Request) { called = true })
	req := httptest.NewRequest(http.MethodPost, "/api/v1/players/rename", nil)
	req = req.WithContext(context.WithValue(req.Context(), serverContextKey, &ServerContext{ID: "1"}))
	wrapped(httptest.NewRecorder(), req)

	if !called {
		t.Error("wrapped handler was not called")
	}
	c.inner.Wait()
	if _, ok := c.Get(cacheKey("1", "players")); ok {
		t.Error("player list cache not busted after a player-write request")
	}
}

// TestHandleAPI_BackupsManageBustsPlayersCache verifies routes registered
// under capBackupsManage (character backup create/restore/delete — the new
// backup & restore feature mutates player data just like capPlayersWrite/
// capPlayersDelete routes do) also bust the player-list cache. Without this,
// a restore leaves the cache serving a stale player list — including the
// #290-style orphan a restore transiently creates — until the 20s TTL
// expires on its own.
func TestHandleAPI_BackupsManageBustsPlayersCache(t *testing.T) {
	origCache := globalPlayersCache
	c, err := newRistrettoCache[[]playerInfo]("test-players", 256)
	if err != nil {
		t.Fatalf("newRistrettoCache: %v", err)
	}
	globalPlayersCache = c
	t.Cleanup(func() { globalPlayersCache = origCache })

	c.Set(cacheKey("1", "players"), []playerInfo{{AccountID: 1}}, time.Minute)
	if _, ok := c.Get(cacheKey("1", "players")); !ok {
		t.Fatal("seed failed")
	}

	mux := http.NewServeMux()
	handleAPI(mux, "POST /api/v1/character-backups/{id}/restore", capBackupsManage,
		func(http.ResponseWriter, *http.Request) {})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/character-backups/1/restore", nil)
	req = req.WithContext(context.WithValue(req.Context(), serverContextKey, &ServerContext{ID: "1"}))
	mux.ServeHTTP(httptest.NewRecorder(), req)

	c.inner.Wait()
	if _, ok := c.Get(cacheKey("1", "players")); ok {
		t.Error("player list cache not busted after a capBackupsManage request")
	}
}

// A request with no server context (legacy single-server / unscoped) must not
// panic in the bust wrapper.
func TestBustPlayersCacheAfter_NoServerContext(t *testing.T) {
	origCache := globalPlayersCache
	c, err := newRistrettoCache[[]playerInfo]("test-players", 256)
	if err != nil {
		t.Fatalf("newRistrettoCache: %v", err)
	}
	globalPlayersCache = c
	t.Cleanup(func() { globalPlayersCache = origCache })

	wrapped := bustPlayersCacheAfter(func(http.ResponseWriter, *http.Request) {})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/players/rename", nil)
	wrapped(httptest.NewRecorder(), req) // must not panic
}
