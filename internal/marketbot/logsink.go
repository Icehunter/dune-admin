package marketbot

import (
	"io"
	"log"
	"sync"
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
	s.mu.Unlock()

	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()

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

// Logger returns a *log.Logger that writes to this sink as well as w.
// Pass io.Discard to suppress the secondary output.
func (s *LogSink) Logger(prefix string, w io.Writer) *log.Logger {
	return log.New(io.MultiWriter(s, w), prefix, log.Ldate|log.Ltime|log.Lmsgprefix)
}
