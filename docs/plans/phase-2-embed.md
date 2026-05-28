# Phase 2 — Embed Market Bot as `internal/marketbot`

**Status:** Implemented ✅ (with Phase 3 embedded-only follow-up)  
**Branch:** `feature/phase-2-embed-marketbot`  
**PR:** Pending (base: `refactor/phase-1-go-layout` / #53)

---

## Scope

Pull `dune-market-bot` into this repo as `internal/marketbot/`, wire it into `cmd/dune-admin/` as an in-process goroutine, and replace the HTTP proxy + container lifecycle calls with direct in-process method calls.

See ADRs [0002](../adr/0002-embed-market-bot-as-library.md), [0003](../adr/0003-single-binary-deployment.md), [0004](../adr/0004-in-process-bot-lifecycle.md), [0005](../adr/0005-ring-buffer-log-streaming.md), [0008](../adr/0008-config-yaml-extensions.md).

---

## New package: `internal/marketbot/`

Files copied from `../dune-market-bot`, `package main` → `package marketbot`:

| File | Notes |
|------|-------|
| `bot.go` | **New.** `Run(ctx, BotConfig) (*Instance, error)` entry point; `runLoop`; `Start` non-blocking wrapper |
| `logsink.go` | **New.** `LogSink` ring buffer (1000 lines), `io.Writer`, fan-out to WS subscribers |
| `api.go` | `APIServer`, auth middleware, HTTP handlers — unchanged except package name |
| `catalog.go` | `loadCatalog(path string)` — flag global removed, path passed as parameter |
| `config.go` | `Config`, `configValues`, `Apply`, `Snapshot` — unchanged |
| `exchange.go` | `Exchange`, `NewExchange`, tick methods — lint fixes applied (13 unchecked errors, dead `createListing` removed) |
| `pricing.go` | Pricing helpers — unchanged |
| `report.go` | `reportData` — dead `runReport` (was `-report` flag mode) removed |
| `api_test.go` | `json.Decode` errors now checked |
| `config_test.go` | Unchanged |
| `pricing_test.go` | Unchanged |

### `Instance` type

```go
type Instance struct {
    API  *APIServer  // in-process HTTP sub-API (nil if APIAddr == "")
    Sink *LogSink    // ring-buffer log fan-out for WebSocket streaming
}

func (i *Instance) Pause()
func (i *Instance) Resume()
func (i *Instance) Restart(ctx context.Context) error
func (i *Instance) StatusSnapshot() any
```

### `LogSink`

```go
func NewLogSink() *LogSink
func (s *LogSink) Write(p []byte) (int, error)   // io.Writer — bot's logger writes here
func (s *LogSink) Subscribe() chan string          // replay ring + live lines
func (s *LogSink) Unsubscribe(ch chan string)
func (s *LogSink) Logger(prefix string, w io.Writer) *log.Logger
```

---

## Changes to `cmd/dune-admin/`

### `main.go`

**New config fields on `appConfig`:**

```yaml
market_bot_enabled: false
market_bot_cache_db: ""           # default: ~/.dune-admin/market-bot-cache.db
market_bot_item_data: ""          # default: ./item-data.json
market_bot_buy_interval: 5m
market_bot_list_interval: 30m
market_bot_buy_threshold: 1.05
market_bot_max_buys: 50
broker_user: ""                   # env: BROKER_USER
broker_pass: ""                   # env: BROKER_PASS
broker_jwt_secret: ""             # env: BROKER_JWT_SECRET
```

**Startup wiring** (after `connectAll`):

```go
if loadedConfig.MarketBotEnabled {
    inst, err := marketbot.Run(ctx, marketbot.BotConfig{ /* from config */ })
    if err != nil {
        fmt.Fprintf(os.Stderr, "market-bot: startup failed: %v\n", err)
    } else {
        embeddedBot = inst
        defer botCancel()
    }
}
```

**Global:** `var embeddedBot *marketbot.Instance` — nil when embedded bot is disabled.

### `handlers_market_bot.go`

All five handlers use embedded in-process routing:

| Handler | Embedded path |
|---------|--------------|
| `handleMarketBotStatus` | `embeddedBot.StatusSnapshot()` |
| `handleMarketBotConfig` | `embeddedBot.ConfigJSON()` / `embeddedBot.ApplyConfig(...)` |
| `handleMarketBotExec` | `Pause()` / `Resume()` / `Restart()` |
| `handleMarketBotLogsReady` | Always `{"ready": true, "mode": "embedded"}` |
| `handleMarketBotLogs` | `Sink.Subscribe()` → WS fan-out |

External/proxy mode paths were removed in Phase 3.

### `go.mod`

Added: `modernc.org/sqlite v1.50.1` (CGO-free SQLite for the category cache).

---

## How to enable

Add to `~/.dune-admin/config.yaml`:

```yaml
market_bot_enabled: true
# market_bot_cache_db and DB fields are inherited from existing config
```

Run from the repo root (so `item-data.json` is found by default):

```bash
./dune-admin
```

Check:

```bash
curl -s http://localhost:8080/api/v1/market-bot/status | jq .
# → {"running": true, "uptime": "...", ...}
```

---

## `go.mod` / dependency notes

`modernc.org/sqlite` is a pure-Go CGO-free SQLite implementation. It adds ~4 MB to the binary but requires no runtime C libraries — the Docker image stays `CGO_ENABLED=0` / `FROM debian:bookworm-slim`.

---

## Verification

```bash
make build
make test-race
go tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint run ./...
go tool github.com/securego/gosec/v2/cmd/gosec -severity high -confidence high ./...
go tool golang.org/x/vuln/cmd/govulncheck ./...
```

All pass. 0 lint issues, 0 security issues, 0 vulnerabilities.
