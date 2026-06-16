package main

import (
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
)

func TestLogRingOverflowDropsOldest(t *testing.T) {
	r := newLogRing(3)
	for _, m := range []string{"a", "b", "c", "d", "e"} {
		_, _ = r.WriteLevel(zerolog.InfoLevel, []byte(m+"\n"))
	}
	got := r.Snapshot()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	want := []string{"c", "d", "e"}
	for i, w := range want {
		if got[i].Line != w {
			t.Errorf("entry %d = %q, want %q", i, got[i].Line, w)
		}
	}
}

func TestLogRingCapturesLevelAndTrimsNewline(t *testing.T) {
	r := newLogRing(10)
	_, _ = r.WriteLevel(zerolog.WarnLevel, []byte("hello\n"))
	got := r.Snapshot()
	if len(got) != 1 || got[0].Line != "hello" || got[0].Level != "warn" {
		t.Fatalf("got %+v", got)
	}
}

func TestLogRingSnapshotIsCopy(t *testing.T) {
	r := newLogRing(5)
	_, _ = r.WriteLevel(zerolog.InfoLevel, []byte("x\n"))
	s := r.Snapshot()
	s[0].Line = "mutated"
	if r.Snapshot()[0].Line != "x" {
		t.Fatal("Snapshot must return a copy, not aliased backing storage")
	}
}

func TestLogRingSubscribeReceivesLiveAndCancelStops(t *testing.T) {
	r := newLogRing(5)
	ch, cancel := r.Subscribe()
	_, _ = r.WriteLevel(zerolog.InfoLevel, []byte("live\n"))
	if got := <-ch; got.Line != "live" {
		t.Fatalf("got %q, want live", got.Line)
	}
	cancel()
	_, _ = r.WriteLevel(zerolog.InfoLevel, []byte("after\n"))
	if _, open := <-ch; open {
		t.Fatal("channel should be closed after cancel")
	}
}

func TestLogRingConcurrentWriters(t *testing.T) {
	r := newLogRing(100)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.WriteLevel(zerolog.InfoLevel, []byte("x\n"))
			_ = r.Snapshot()
		}()
	}
	wg.Wait()
}

func TestInitLoggingCapturesToRing(t *testing.T) {
	t.Setenv("DIAG_LOG_BUFFER", "50")
	initLogging()
	if globalLogRing == nil {
		t.Fatal("globalLogRing must be initialised by initLogging")
	}
	componentLog("test").Info().Msg("ring-capture-probe")
	found := false
	for _, e := range globalLogRing.Snapshot() {
		if strings.Contains(e.Line, "ring-capture-probe") {
			found = true
		}
	}
	if !found {
		t.Fatal("log event was not captured in the ring")
	}
}
