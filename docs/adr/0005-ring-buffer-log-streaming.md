# ADR 0005 — Ring-buffer for embedded bot log streaming

**Status:** Implemented (Phase 2)  
**Date:** 2025-05-27

## Context

`handleMarketBotLogs` previously streamed logs by running `kubectl logs -f <pod>` or `docker logs -f <container>` via the executor. After embedding the bot as a goroutine there is no pod or container to tail.

Three approaches were considered:

| Approach | Notes |
|----------|-------|
| **In-process ring buffer** | Bot writes to a `LogSink`; WS handler subscribes | Chosen |
| Pipe `log.SetOutput` to custom writer | Hijacks the global logger; affects all log output |
| Write bot logs to a temp file | I/O overhead, cleanup required, no benefit over ring buffer |

## Decision

`internal/marketbot/logsink.go` implements `LogSink`:

```go
type LogSink struct { /* ring buffer + subscriber map */ }

func NewLogSink() *LogSink
func (s *LogSink) Write(p []byte) (int, error)   // implements io.Writer
func (s *LogSink) Subscribe() chan string          // replay history + live lines
func (s *LogSink) Unsubscribe(ch chan string)
func (s *LogSink) Logger(prefix string, w io.Writer) *log.Logger
```

- Fixed capacity: 1000 lines
- `Subscribe()` replays existing ring contents immediately, then delivers new lines
- Slow subscribers are dropped (non-blocking send) to prevent log backpressure from blocking the bot tick loop
- `Logger()` returns a `*log.Logger` writing to `io.MultiWriter(sink, os.Stderr)` so logs appear in both the WS stream and the process's stderr

`handleMarketBotLogs` detects `embeddedBot != nil` and subscribes to `embeddedBot.Sink` instead of using the kubectl/docker path.

`handleMarketBotLogsReady` returns `{"ready": true, "mode": "embedded"}` whenever `embeddedBot != nil`, regardless of control plane.

## Consequences

- Memory cost bounded: ~1000 × ~200 bytes ≈ 200 KB worst case
- Older log lines (up to 1000) replayed immediately on WS connect — no "waiting for first line" delay
- External-bot log streaming paths were removed; market-bot logs now come from the embedded `LogSink` path only
