package main

import (
	"sync/atomic"
	"testing"
	"time"
)

// applyDiscordGuildsAsync must return to the caller immediately (the HTTP save
// path must not block on Discord REST work) and must still invoke the underlying
// worker with the supplied removed-guild ids.
func TestApplyDiscordGuildsAsync_NonBlockingAndRuns(t *testing.T) {
	orig := applyDiscordGuildsFn
	t.Cleanup(func() { applyDiscordGuildsFn = orig })

	got := make(chan []string, 1)
	applyDiscordGuildsFn = func(removed ...string) {
		got <- removed
	}

	start := time.Now()
	applyDiscordGuildsAsync("guild-7")
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("applyDiscordGuildsAsync blocked the caller for %v; want near-instant return", elapsed)
	}

	select {
	case removed := <-got:
		if len(removed) != 1 || removed[0] != "guild-7" {
			t.Fatalf("worker got removed=%v; want [guild-7]", removed)
		}
	case <-time.After(time.Second):
		t.Fatal("worker was never invoked")
	}
}

// Overlapping CRUD writes each fire applyDiscordGuildsAsync; the applies must run
// one-at-a-time so they don't race on the status-loop registry or interleave
// command registration.
func TestApplyDiscordGuildsAsync_Serialized(t *testing.T) {
	orig := applyDiscordGuildsFn
	t.Cleanup(func() { applyDiscordGuildsFn = orig })

	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	var concurrent, maxConcurrent int32
	applyDiscordGuildsFn = func(_ ...string) {
		n := atomic.AddInt32(&concurrent, 1)
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if n <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, n) {
				break
			}
		}
		entered <- struct{}{}
		<-release
		atomic.AddInt32(&concurrent, -1)
	}

	applyDiscordGuildsAsync()
	<-entered // first worker is now inside, holding the apply mutex

	// Give a would-be second worker a window to (wrongly) enter concurrently.
	applyDiscordGuildsAsync()
	select {
	case <-entered:
		t.Fatal("second apply ran while the first held the lock — not serialized")
	case <-time.After(50 * time.Millisecond):
		// expected: the second apply is blocked on the mutex
	}

	close(release) // let both proceed and finish
	<-entered      // second worker finally runs

	if got := atomic.LoadInt32(&maxConcurrent); got != 1 {
		t.Fatalf("max concurrent applies = %d; want 1 (serialized)", got)
	}
}
