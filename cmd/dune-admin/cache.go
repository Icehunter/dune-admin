package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"golang.org/x/sync/singleflight"
)

// readCache is the consumer-facing contract for the read-through cache. Handlers
// and warmers accept this interface; the concrete type is *ristrettoCache[T]. A
// future Redis-backed implementation could satisfy the same interface without
// touching call sites (Ristretto-only for now — Redis is not required).
type readCache[T any] interface {
	Get(key string) (T, bool)
	Set(key string, val T, ttl time.Duration)
	Delete(key string)
	// GetOrLoad returns the cached value for key, or calls load on a miss and
	// caches the result. Concurrent misses for the same key collapse into one
	// load via singleflight. A load error is returned and NOT cached, so the
	// next call retries (we never poison the cache with a transient failure).
	GetOrLoad(ctx context.Context, key string, ttl time.Duration, load func(context.Context) (T, error)) (T, error)
}

// ristrettoCache[T] is a generic, server-scope-keyed read-through cache backed
// by dgraph-io/ristretto with singleflight miss collapsing. It returns concrete
// pointers from its constructor and satisfies readCache[T].
type ristrettoCache[T any] struct {
	inner *ristretto.Cache[string, T]
	group singleflight.Group
	name  string // label for logs/metrics, e.g. "health"
}

// newRistrettoCache builds a cache holding up to maxItems entries (cost 1 each).
// NumCounters is set to 10× maxItems per Ristretto's sizing guidance.
func newRistrettoCache[T any](name string, maxItems int64) (*ristrettoCache[T], error) {
	c, err := ristretto.NewCache(&ristretto.Config[string, T]{
		NumCounters: maxItems * 10,
		MaxCost:     maxItems,
		BufferItems: 64,
	})
	if err != nil {
		return nil, fmt.Errorf("ristretto cache %q: %w", name, err)
	}
	return &ristrettoCache[T]{inner: c, name: name}, nil
}

func (rc *ristrettoCache[T]) Get(key string) (T, bool) {
	return rc.inner.Get(key)
}

// Set stores val under key with the given TTL. SetWithTTL is asynchronous in
// Ristretto; Wait blocks until the write is visible so an immediately-following
// Get/GetOrLoad sees it (prevents a redundant second load).
func (rc *ristrettoCache[T]) Set(key string, val T, ttl time.Duration) {
	rc.inner.SetWithTTL(key, val, 1, ttl)
	rc.inner.Wait()
}

func (rc *ristrettoCache[T]) Delete(key string) {
	rc.inner.Del(key)
}

func (rc *ristrettoCache[T]) GetOrLoad(
	ctx context.Context, key string, ttl time.Duration, load func(context.Context) (T, error),
) (T, error) {
	if val, ok := rc.inner.Get(key); ok {
		return val, nil
	}
	return rc.loadAndCache(ctx, key, ttl, load)
}

// loadAndCache runs load under singleflight and caches a successful result.
func (rc *ristrettoCache[T]) loadAndCache(
	ctx context.Context, key string, ttl time.Duration, load func(context.Context) (T, error),
) (T, error) {
	type result struct {
		val T
		err error
	}
	v, _, _ := rc.group.Do(key, func() (any, error) {
		val, err := load(ctx)
		if err != nil {
			return result{err: err}, nil // surfaced to caller; never cached
		}
		rc.inner.SetWithTTL(key, val, 1, ttl)
		rc.inner.Wait()
		return result{val: val}, nil
	})
	r := v.(result)
	return r.val, r.err
}

// cacheKey builds a server-scoped cache key: "<scope>:<part>[:<part>...]".
// The scope is the per-server string id (see serverScope), so two servers never
// share cached read state.
func cacheKey(scope string, parts ...string) string {
	return scope + ":" + strings.Join(parts, ":")
}

// ── global caches ─────────────────────────────────────────────────────────────

// healthCacheTTL bounds how stale a dashboard health card can be. The warmer
// refreshes ahead of this, so the typical staleness window is shorter.
const healthCacheTTL = 15 * time.Second

// globalHealthCache caches per-server health summaries (the expensive
// control-plane GetStatus + DB-stats fan-out — the dashboard cards). Nil when
// caching is unavailable (the handlers fall back to a live assemble).
var globalHealthCache *ristrettoCache[serverHealth]

// globalBGStatusCache caches the full per-server battlegroup status (the
// Battlegroup tab). It's derived from the same control-plane GetStatus call as
// health, so the warmer populates both from one fetch.
var globalBGStatusCache *ristrettoCache[*BattlegroupStatus]

// playersCacheTTL bounds staleness of the cached player list. Operator
// mutations bust it immediately (see handleAPI); externally-joined characters
// appear within the TTL (matching the game's own DB-write lag).
const playersCacheTTL = 20 * time.Second

// globalPlayersCache caches the per-server player list (the biggest single UI
// read query), busted on any player-write request for that scope.
var globalPlayersCache *ristrettoCache[[]playerInfo]

// initGlobalCaches builds the process-wide read caches. Called once at startup
// (before the connect path) from run(). On error the caches stay nil and the
// handlers serve live data.
func initGlobalCaches() error {
	var err error
	if globalHealthCache, err = newRistrettoCache[serverHealth]("health", 256); err != nil {
		return fmt.Errorf("init health cache: %w", err)
	}
	if globalBGStatusCache, err = newRistrettoCache[*BattlegroupStatus]("bgstatus", 256); err != nil {
		return fmt.Errorf("init bgstatus cache: %w", err)
	}
	if globalPlayersCache, err = newRistrettoCache[[]playerInfo]("players", 256); err != nil {
		return fmt.Errorf("init players cache: %w", err)
	}
	return nil
}

// invalidatePlayersCache drops a server's cached player list so the next read is
// live. Called after any player-write request for that scope (see handleAPI).
func invalidatePlayersCache(scope string) {
	if globalPlayersCache != nil {
		globalPlayersCache.Delete(cacheKey(scope, "players"))
	}
}

// invalidateServerHealth drops a server's cached health AND battlegroup status
// (both derive from the same control-plane status) so the next read is live.
// Called after mutations that change a server's status (reconnect, config edit,
// delete, lifecycle start/stop/restart).
func invalidateServerHealth(scope string) {
	if globalHealthCache != nil {
		globalHealthCache.Delete(cacheKey(scope, "health"))
	}
	if globalBGStatusCache != nil {
		globalBGStatusCache.Delete(cacheKey(scope, "bgstatus"))
	}
}
