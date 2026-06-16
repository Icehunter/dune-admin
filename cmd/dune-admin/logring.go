package main

import (
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

// ringLine is one captured log event: the JSON bytes zerolog produced (newline
// trimmed) plus the level it was emitted at.
type ringLine struct {
	Level string `json:"level"`
	Line  string `json:"line"`
}

// logRing is a fixed-capacity in-memory ring buffer of recent log events. It
// implements zerolog.LevelWriter so it can be installed as a second sink
// alongside stderr. Writes never block: on overflow the oldest entry is
// dropped, and slow subscribers drop events rather than stalling the logger.
type logRing struct {
	mu      sync.Mutex
	buf     []ringLine
	start   int
	n       int
	subs    map[int]chan ringLine
	nextSub int
}

func newLogRing(capacity int) *logRing {
	if capacity < 1 {
		capacity = 1
	}
	return &logRing{
		buf:  make([]ringLine, capacity),
		subs: make(map[int]chan ringLine),
	}
}

func (r *logRing) Write(p []byte) (int, error) {
	return r.WriteLevel(zerolog.NoLevel, p)
}

func (r *logRing) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	entry := ringLine{Level: level.String(), Line: strings.TrimRight(string(p), "\n")}
	r.mu.Lock()
	idx := (r.start + r.n) % len(r.buf)
	r.buf[idx] = entry
	if r.n < len(r.buf) {
		r.n++
	} else {
		r.start = (r.start + 1) % len(r.buf)
	}
	for _, ch := range r.subs {
		select {
		case ch <- entry:
		default:
		}
	}
	r.mu.Unlock()
	return len(p), nil
}

// Snapshot returns a copy of the buffered entries oldest-first.
func (r *logRing) Snapshot() []ringLine {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ringLine, r.n)
	for i := 0; i < r.n; i++ {
		out[i] = r.buf[(r.start+i)%len(r.buf)]
	}
	return out
}

// Subscribe returns a channel of live entries and a cancel func that closes it.
func (r *logRing) Subscribe() (<-chan ringLine, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.nextSub
	r.nextSub++
	ch := make(chan ringLine, 256)
	r.subs[id] = ch
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			if c, ok := r.subs[id]; ok {
				delete(r.subs, id)
				close(c)
			}
		})
	}
	return ch, cancel
}
