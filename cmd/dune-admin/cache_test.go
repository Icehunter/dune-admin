package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Compile-time assertion: the concrete cache satisfies the consumer interface.
var _ readCache[int] = (*ristrettoCache[int])(nil)

func newTestCache[T any](t *testing.T) *ristrettoCache[T] {
	t.Helper()
	c, err := newRistrettoCache[T]("test", 100)
	if err != nil {
		t.Fatalf("newRistrettoCache: %v", err)
	}
	return c
}

func TestRistrettoCache_GetSetDelete(t *testing.T) {
	c := newTestCache[string](t)

	if _, ok := c.Get("k"); ok {
		t.Error("Get on empty cache returned ok=true")
	}

	c.Set("k", "v", time.Minute)
	if got, ok := c.Get("k"); !ok || got != "v" {
		t.Errorf("Get after Set = %q, %v; want \"v\", true", got, ok)
	}

	c.Delete("k")
	// Del is async; Wait flushes the buffer so the delete is visible.
	c.inner.Wait()
	if _, ok := c.Get("k"); ok {
		t.Error("Get after Delete returned ok=true")
	}
}

func TestRistrettoCache_GetOrLoad_LoadsOnceThenHits(t *testing.T) {
	c := newTestCache[int](t)
	var calls atomic.Int32
	load := func(context.Context) (int, error) {
		calls.Add(1)
		return 42, nil
	}

	// Miss → load.
	v, err := c.GetOrLoad(context.Background(), "k", time.Minute, load)
	if err != nil || v != 42 {
		t.Fatalf("first GetOrLoad = %d, %v; want 42, nil", v, err)
	}
	// Hit → no second load (Set+Wait made the write visible).
	v, err = c.GetOrLoad(context.Background(), "k", time.Minute, load)
	if err != nil || v != 42 {
		t.Fatalf("second GetOrLoad = %d, %v; want 42, nil", v, err)
	}
	if calls.Load() != 1 {
		t.Errorf("load called %d times, want 1 (second call must hit cache)", calls.Load())
	}
}

func TestRistrettoCache_GetOrLoad_ErrorNotCached(t *testing.T) {
	c := newTestCache[int](t)
	wantErr := errors.New("boom")
	var calls atomic.Int32
	load := func(context.Context) (int, error) {
		calls.Add(1)
		return 0, wantErr
	}

	if _, err := c.GetOrLoad(context.Background(), "k", time.Minute, load); !errors.Is(err, wantErr) {
		t.Fatalf("GetOrLoad err = %v, want %v", err, wantErr)
	}
	// Error must not be cached → next call retries.
	if _, err := c.GetOrLoad(context.Background(), "k", time.Minute, load); !errors.Is(err, wantErr) {
		t.Fatalf("second GetOrLoad err = %v, want %v", err, wantErr)
	}
	if calls.Load() != 2 {
		t.Errorf("load called %d times, want 2 (errors are never cached)", calls.Load())
	}
}

// Concurrent GetOrLoad on the same key must be race-free (run with -race) and
// collapse via singleflight so load runs far fewer times than the caller count.
func TestRistrettoCache_GetOrLoad_ConcurrentCollapses(t *testing.T) {
	c := newTestCache[int](t)
	var calls atomic.Int32
	release := make(chan struct{})
	load := func(context.Context) (int, error) {
		calls.Add(1)
		<-release // hold the in-flight load so concurrent callers queue behind it
		return 7, nil
	}

	const n = 50
	var wg sync.WaitGroup
	got := make([]int, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := c.GetOrLoad(context.Background(), "k", time.Minute, load)
			if err == nil {
				got[i] = v
			}
		}(i)
	}
	close(release)
	wg.Wait()

	for i, v := range got {
		if v != 7 {
			t.Fatalf("goroutine %d got %d, want 7", i, v)
		}
	}
	if calls.Load() < 1 || calls.Load() > n {
		t.Errorf("load called %d times, want between 1 and %d", calls.Load(), n)
	}
	if calls.Load() == n {
		t.Errorf("no singleflight collapse: load called all %d times", n)
	}
}

func TestCacheKey(t *testing.T) {
	tests := []struct {
		scope string
		parts []string
		want  string
	}{
		{"1", []string{"health"}, "1:health"},
		{"default", []string{"players"}, "default:players"},
		{"2", []string{"journey", "88472"}, "2:journey:88472"},
	}
	for _, tt := range tests {
		if got := cacheKey(tt.scope, tt.parts...); got != tt.want {
			t.Errorf("cacheKey(%q, %v) = %q, want %q", tt.scope, tt.parts, got, tt.want)
		}
	}
}
