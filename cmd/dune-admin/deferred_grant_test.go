package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeDeferredSource is an in-memory deferredGrantSource for testing the
// generic retry core without a real store.
type fakeDeferredSource struct {
	claims   []deferredClaim
	listErr  error
	listedAt []time.Time
}

func (f *fakeDeferredSource) listRetryableDeferredClaims(now time.Time) ([]deferredClaim, error) {
	f.listedAt = append(f.listedAt, now)
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.claims, nil
}

func TestProcessDeferredGrantTick_Success(t *testing.T) {
	src := &fakeDeferredSource{claims: []deferredClaim{{OwnerID: 1}, {OwnerID: 2}}}
	var attempted []int64
	attempt := func(_ context.Context, c deferredClaim) error {
		attempted = append(attempted, c.OwnerID)
		return nil
	}
	processDeferredGrantTick(context.Background(), src, attempt, time.Now())
	if len(attempted) != 2 || attempted[0] != 1 || attempted[1] != 2 {
		t.Fatalf("attempted = %v, want [1 2]", attempted)
	}
}

func TestProcessDeferredGrantTick_FailThenSucceed(t *testing.T) {
	src := &fakeDeferredSource{claims: []deferredClaim{{OwnerID: 7}}}
	calls := 0
	attempt := func(_ context.Context, _ deferredClaim) error {
		calls++
		if calls == 1 {
			return errors.New("transient")
		}
		return nil
	}
	// First tick: failure is logged, not fatal.
	processDeferredGrantTick(context.Background(), src, attempt, time.Now())
	// Second tick: succeeds.
	processDeferredGrantTick(context.Background(), src, attempt, time.Now())
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestProcessDeferredGrantTick_OnlyDueClaimsRetried(t *testing.T) {
	// The source decides which claims are due; the core must attempt exactly
	// those returned, no more.
	src := &fakeDeferredSource{claims: []deferredClaim{{OwnerID: 42}}}
	attempts := 0
	attempt := func(_ context.Context, _ deferredClaim) error {
		attempts++
		return nil
	}
	now := time.Now()
	processDeferredGrantTick(context.Background(), src, attempt, now)
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if len(src.listedAt) != 1 || !src.listedAt[0].Equal(now) {
		t.Fatalf("listRetryable called with %v, want [%v]", src.listedAt, now)
	}
}

func TestProcessDeferredGrantTick_ListError(t *testing.T) {
	src := &fakeDeferredSource{listErr: errors.New("db down")}
	attempted := false
	attempt := func(_ context.Context, _ deferredClaim) error {
		attempted = true
		return nil
	}
	// A list error must not panic and must skip the tick entirely.
	processDeferredGrantTick(context.Background(), src, attempt, time.Now())
	if attempted {
		t.Fatalf("attempt called despite list error")
	}
}

func TestRunDeferredGrantLoop_NilSourceReturns(t *testing.T) {
	// A nil source is a no-op (engine disabled). Must return immediately.
	done := make(chan struct{})
	go func() {
		runDeferredGrantLoop(context.Background(), nil, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runDeferredGrantLoop did not return for nil source")
	}
}

func TestRunDeferredGrantLoop_CancelStops(t *testing.T) {
	src := &fakeDeferredSource{}
	attempt := func(_ context.Context, _ deferredClaim) error { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runDeferredGrantLoop(ctx, src, attempt)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runDeferredGrantLoop did not stop after cancel")
	}
}

func TestDeferredGrantConstants(t *testing.T) {
	if deferredGrantMaxAttempts != 3 {
		t.Errorf("deferredGrantMaxAttempts = %d, want 3", deferredGrantMaxAttempts)
	}
	if deferredGrantRetryBackoff != 24*time.Hour {
		t.Errorf("deferredGrantRetryBackoff = %v, want 24h", deferredGrantRetryBackoff)
	}
}
