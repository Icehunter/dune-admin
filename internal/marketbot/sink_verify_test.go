package marketbot

import (
	"io"
	"strings"
	"testing"
	"time"
)

// TestSinkOneLinePerEvent verifies the zerolog->LogSink->Subscribe path still
// delivers exactly one clean (newline-free) line per log event.
func TestSinkOneLinePerEvent(t *testing.T) {
	s := NewLogSink()
	ch := s.Subscribe()
	defer s.Unsubscribe(ch)

	logger := s.Logger(io.Discard)
	logger.Info().Str("k", "v").Msg("hello world")
	logger.Warn().Int("n", 3).Msg("second line")

	got := drain(t, ch, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d: %#v", len(got), got)
	}
	for _, line := range got {
		if strings.Contains(line, "\n") {
			t.Errorf("line contains embedded newline: %q", line)
		}
	}
	if !strings.Contains(got[0], "hello world") || !strings.Contains(got[1], "second line") {
		t.Errorf("unexpected line content: %#v", got)
	}
}

func drain(t *testing.T, ch chan string, n int) []string {
	t.Helper()
	var out []string
	deadline := time.After(2 * time.Second)
	for len(out) < n {
		select {
		case line := <-ch:
			out = append(out, line)
		case <-deadline:
			return out
		}
	}
	return out
}
