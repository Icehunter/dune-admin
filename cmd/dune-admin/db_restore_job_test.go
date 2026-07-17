package main

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

// newTestRestoreJobs returns a fresh manager so tests don't share the global.
func newTestRestoreJobs() *dbRestoreJobs {
	return newDBRestoreJobs()
}

func TestDBRestoreJobs_StatusBeforeAnyRun(t *testing.T) {
	t.Parallel()
	jobs := newTestRestoreJobs()
	st := jobs.Status("s1")
	if st.Running || st.Done || st.Failed {
		t.Fatalf("fresh status should be idle, got %+v", st)
	}
	if len(st.Steps) != 4 {
		t.Fatalf("steps = %d, want 4 pending steps", len(st.Steps))
	}
	for _, s := range st.Steps {
		if s.Status != "pending" {
			t.Fatalf("step %s = %s, want pending", s.Key, s.Status)
		}
	}
}

func TestDBRestoreJobs_RunSuccessLifecycle(t *testing.T) {
	t.Parallel()
	jobs := newTestRestoreJobs()
	release := make(chan struct{})
	started := make(chan struct{})

	err := jobs.Start("s1", "dune-x.dump", func(report func(step, status string)) (dbRestoreResult, error) {
		close(started)
		report(restoreStepCheck, restoreStatusRunning)
		report(restoreStepCheck, restoreStatusDone)
		report(restoreStepStop, restoreStatusSkipped)
		report(restoreStepRestore, restoreStatusRunning)
		<-release
		report(restoreStepRestore, restoreStatusDone)
		report(restoreStepFinalize, restoreStatusRunning)
		report(restoreStepFinalize, restoreStatusDone)
		return dbRestoreResult{Output: "restored", IgnoredErrors: 38, ServersStopped: false}, nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started

	// Mid-run: running, restore step in flight, stop skipped.
	st := jobs.Status("s1")
	if !st.Running || st.Done {
		t.Fatalf("mid-run status = %+v, want running", st)
	}
	if st.File != "dune-x.dump" {
		t.Fatalf("file = %q", st.File)
	}
	byKey := map[string]string{}
	for _, s := range st.Steps {
		byKey[s.Key] = s.Status
	}
	if byKey[restoreStepCheck] != "done" || byKey[restoreStepStop] != "skipped" || byKey[restoreStepRestore] != "running" {
		t.Fatalf("mid-run steps = %v", byKey)
	}

	close(release)
	jobs.wait("s1") // test helper: block until the goroutine finishes

	st = jobs.Status("s1")
	if st.Running || !st.Done || st.Failed {
		t.Fatalf("final status = %+v, want done", st)
	}
	if st.IgnoredErrors != 38 || st.Output != "restored" {
		t.Fatalf("result fields = %+v", st)
	}
}

func TestDBRestoreJobs_RunFailureLifecycle(t *testing.T) {
	t.Parallel()
	jobs := newTestRestoreJobs()
	boom := errors.New("pg_restore: connection refused")

	err := jobs.Start("s1", "dune-x.dump", func(report func(step, status string)) (dbRestoreResult, error) {
		report(restoreStepCheck, restoreStatusRunning)
		report(restoreStepCheck, restoreStatusFailed)
		return dbRestoreResult{Output: "some output"}, boom
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	jobs.wait("s1")

	st := jobs.Status("s1")
	if st.Running || !st.Done || !st.Failed {
		t.Fatalf("final status = %+v, want done+failed", st)
	}
	if !strings.Contains(st.Error, "connection refused") {
		t.Fatalf("error = %q, want the run error", st.Error)
	}
	if st.Output != "some output" {
		t.Fatalf("output = %q, want preserved output", st.Output)
	}
}

func TestDBRestoreJobs_RejectsConcurrentStartSameScope(t *testing.T) {
	t.Parallel()
	jobs := newTestRestoreJobs()
	release := make(chan struct{})
	started := make(chan struct{})

	if err := jobs.Start("s1", "a.dump", func(func(string, string)) (dbRestoreResult, error) {
		close(started)
		<-release
		return dbRestoreResult{}, nil
	}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	<-started

	if err := jobs.Start("s1", "b.dump", func(func(string, string)) (dbRestoreResult, error) {
		return dbRestoreResult{}, nil
	}); err == nil {
		t.Fatal("second Start on the same scope must be rejected while running")
	}

	close(release)
	jobs.wait("s1")

	// After completion a new run is allowed and resets state.
	if err := jobs.Start("s1", "c.dump", func(func(string, string)) (dbRestoreResult, error) {
		return dbRestoreResult{}, nil
	}); err != nil {
		t.Fatalf("Start after completion: %v", err)
	}
	jobs.wait("s1")
	if st := jobs.Status("s1"); st.File != "c.dump" {
		t.Fatalf("state not reset for new run: %+v", st)
	}
}

func TestDBRestoreJobs_ScopesAreIsolated(t *testing.T) {
	t.Parallel()
	jobs := newTestRestoreJobs()
	release := make(chan struct{})
	started := make(chan struct{})

	if err := jobs.Start("s1", "a.dump", func(func(string, string)) (dbRestoreResult, error) {
		close(started)
		<-release
		return dbRestoreResult{}, nil
	}); err != nil {
		t.Fatalf("Start s1: %v", err)
	}
	<-started

	// A different scope can start concurrently.
	var wg sync.WaitGroup
	wg.Add(1)
	if err := jobs.Start("s2", "b.dump", func(func(string, string)) (dbRestoreResult, error) {
		defer wg.Done()
		return dbRestoreResult{}, nil
	}); err != nil {
		t.Fatalf("Start s2: %v", err)
	}
	wg.Wait()
	jobs.wait("s2")

	if st := jobs.Status("s2"); !st.Done || st.Failed {
		t.Fatalf("s2 should be done, got %+v", st)
	}
	if st := jobs.Status("s1"); !st.Running {
		t.Fatalf("s1 should still be running, got %+v", st)
	}
	close(release)
	jobs.wait("s1")
}
