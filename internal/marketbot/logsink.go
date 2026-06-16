package marketbot

import (
	"io"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

const ringCapacity = 1000

// LogSink is a bounded ring buffer that captures log lines from the embedded
// bot and fan-outs to active WebSocket subscribers. It implements io.Writer so
// it can be passed to log.New as the output destination.
type LogSink struct {
	mu   sync.Mutex
	ring []string
	pos  int // next write position (wraps)
	full bool

	subsMu sync.RWMutex
	subs   map[chan string]struct{}
}

// NewLogSink returns an initialised LogSink.
func NewLogSink() *LogSink {
	return &LogSink{
		ring: make([]string, ringCapacity),
		subs: make(map[chan string]struct{}),
	}
}

// Write implements io.Writer; each call is treated as one log line.
func (s *LogSink) Write(p []byte) (int, error) {
	line := string(p)
	s.mu.Lock()
	s.ring[s.pos] = line
	s.pos = (s.pos + 1) % ringCapacity
	if s.pos == 0 {
		s.full = true
	}
	s.mu.Unlock()

	s.subsMu.RLock()
	for ch := range s.subs {
		select {
		case ch <- line:
		default: // slow subscriber — drop rather than block
		}
	}
	s.subsMu.RUnlock()
	return len(p), nil
}

// Subscribe returns a buffered channel that will receive new log lines.
// The caller must call Unsubscribe when done.
func (s *LogSink) Subscribe() chan string {
	ch := make(chan string, 256)
	// Replay existing ring contents in order.
	s.mu.Lock()
	var history []string
	if s.full {
		history = append(s.ring[s.pos:], s.ring[:s.pos]...)
	} else {
		history = s.ring[:s.pos]
	}
	// Register the subscriber while s.mu is still held so that no Write() call
	// between snapshot and registration can slip through undelivered.
	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	s.mu.Unlock()

	go func() {
		for _, line := range history {
			select {
			case ch <- line:
			default:
			}
		}
	}()
	return ch
}

// Unsubscribe removes a channel previously returned by Subscribe.
func (s *LogSink) Unsubscribe(ch chan string) {
	s.subsMu.Lock()
	delete(s.subs, ch)
	s.subsMu.Unlock()
}

// Logger returns a zerolog.Logger that emits human-readable console lines to
// BOTH this sink (for the live WebSocket log view) and w (typically stderr).
// Each zerolog event is one Write call, so the sink — and therefore every
// WebSocket subscriber — receives exactly one line per event. Pass io.Discard
// as w to suppress the secondary output.
//
// The trailing newline zerolog appends to console output is trimmed for the
// sink copy so the ring buffer / WS frames hold clean single lines.
func (s *LogSink) Logger(w io.Writer) zerolog.Logger {
	sinkConsole := zerolog.ConsoleWriter{Out: trimWriter{s}, TimeFormat: "15:04:05", NoColor: true}
	stderrConsole := zerolog.ConsoleWriter{Out: w, TimeFormat: "15:04:05"}
	mw := zerolog.MultiLevelWriter(sinkConsole, stderrConsole)
	return zerolog.New(mw).With().Timestamp().Logger()
}

// trimWriter strips the trailing newline from each write before forwarding to
// the underlying writer, so the LogSink stores one clean line per log event.
type trimWriter struct{ w io.Writer }

func (t trimWriter) Write(p []byte) (int, error) {
	if _, err := t.w.Write([]byte(strings.TrimRight(string(p), "\n"))); err != nil {
		return 0, err
	}
	return len(p), nil
}
