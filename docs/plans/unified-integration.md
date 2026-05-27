# Plan: Unified dune-admin + dune-market-bot — Restructure & Integration

**Status:** In progress — Phase 3 deployment/documentation sync  
**Created:** 2025-05-27  
**Updated:** 2025-05-27  
**Relates to:** Issues #10–14, #18–51  
**PRs:** #52 (Phase 0), #53 (Phase 1), Phase 2/3 local-only (not pushed)

---

## Goal

1. Ensure dune-admin runs identically in Podman / Kubernetes / Docker / AMP or locally.
2. Absorb `../dune-market-bot` as an in-process goroutine library — single binary, single process.
3. Restructure the repo to standard Go layout to support both goals cleanly.
4. Fix the five open bug/chore issues before restructuring.
5. Batch-resolve the 34 cognitive-complexity refactor issues.

All significant architectural decisions are captured in `docs/adr/`.

---

## Target directory layout

```
dune-admin/
├── cmd/
│   └── dune-admin/        ← main package (all former root *.go files)
├── internal/
│   └── marketbot/         ← market bot (from dune-market-bot repo)
│       ├── bot.go         ← Run(ctx, BotConfig) → (*Instance, error)
│       ├── logsink.go     ← ring-buffer log fan-out for WS streaming
│       ├── api.go
│       ├── catalog.go
│       ├── config.go
│       ├── exchange.go
│       ├── pricing.go
│       └── report.go
├── deploy/
│   ├── Dockerfile
│   ├── docker-compose.yml
│   └── k8s/
│       └── dune-admin.yaml   ← Phase 3
├── docs/
│   ├── adr/
│   └── plans/
│       └── unified-integration.md
├── go.mod
└── Makefile
```

---

## ADR index

| ADR | Title | Status |
|-----|-------|--------|
| [0001](../adr/0001-standard-go-layout.md) | Standard Go project layout | ✅ Implemented (PR #53) |
| [0002](../adr/0002-embed-market-bot-as-library.md) | Embed market bot as `internal/marketbot` | ✅ Implemented (Phase 2) |
| [0003](../adr/0003-single-binary-deployment.md) | Single binary and container image | ✅ Implemented (Phase 2) |
| [0004](../adr/0004-in-process-bot-lifecycle.md) | In-process bot lifecycle control | ✅ Implemented (Phase 2) |
| [0005](../adr/0005-ring-buffer-log-streaming.md) | Ring-buffer for embedded bot log streaming | ✅ Implemented (Phase 2) |
| [0006](../adr/0006-unified-k8s-manifest.md) | Unified k8s manifest | ✅ Implemented (Phase 3) |
| [0007](../adr/0007-sqlite-cache-storage.md) | PVC/volume for SQLite cache | ✅ Implemented (Phase 3) |
| [0008](../adr/0008-config-yaml-extensions.md) | Config YAML extensions for embedded bot | ✅ Implemented (Phase 2) |

---

## Phase 0 — Bug/chore fixes ✅ PR #52

| Issue | Fix |
|-------|-----|
| #10 | Deleted `capture.go`; removed `-capture` flag; moved `dialAMQP`/`buildCaptureJWT` to `broker.go`/`jwt_helpers.go`; removed `ListExchanges`, `EnsureCaptureUser` from all control planes |
| #11 | `splitAtDuneAdminMarker` returns `error` when BEGIN present but END missing; callers return HTTP 409 |
| #12 | `parseINIRaw` preserves `+`/`-` prefixes; duplicate array keys stored with `\x00N` suffix, stripped on render |
| #13 | `originAllowedForRequest` uses `r.RemoteAddr` (not spoofable `r.Host`) for loopback check; CORS headers skipped when Origin absent |
| #14 | `ampExecutor` embeds `Executor` interface instead of `*localExecutor`; works with SSH executor |

**Copilot/CodeQL fixes (follow-up commits on #52):**

- `broker.go`: hardcoded `capUser`/`capPass` replaced by `brokerCredentials()` reading `BROKER_USER`/`BROKER_PASS`
- `jwt_helpers.go`: HMAC signing secret loaded from `BROKER_JWT_SECRET` env var; hardcoded value is fallback only
- `handlers_server_settings.go`: `parseINIRaw` duplicate-key fix (null-byte dedup suffix)
- `server.go`: `r.RemoteAddr` replaces `r.Host` for origin check; CORS empty-origin fix

---

## Phase 1 — Standard Go layout ✅ PR #53

- All root `*.go` files → `cmd/dune-admin/`
- `Dockerfile`, `docker-compose.yml` → `deploy/`
- `deploy/Dockerfile` build target: `./cmd/dune-admin`
- `Makefile`: `CMD := ./cmd/dune-admin`; all targets updated
- `.air.toml`: build target updated; `deploy/` excluded from watcher

---

## Phase 2 — Embed market bot ✅ Implemented

### `internal/marketbot/` package

Copied from `../dune-market-bot`, `package main` → `package marketbot`.

**New files:**

- `bot.go` — `Run(ctx, BotConfig) (*Instance, error)`; `Instance.{Pause,Resume,Restart}`
- `logsink.go` — `LogSink`: ring buffer (1000 lines), fan-out to WS subscribers, implements `io.Writer`

**Modified:**

- `catalog.go` — `loadCatalog(path string)` replaces `flag.String` global
- all files — `package main` → `package marketbot`

### `cmd/dune-admin/` changes

**`main.go`:**

- `appConfig` gains: `MarketBotEnabled`, `MarketBotCacheDB`, `MarketBotItemData`, `MarketBotBuyInt`, `MarketBotListInt`, `MarketBotThresh`, `MarketBotMaxBuys`
- On startup: if `MarketBotEnabled`, calls `marketbot.Run(ctx, BotConfig{...})` and stores result in `embeddedBot`
- `BrokerUser`/`BrokerPass`/`BrokerJWTSecret` config fields added

**`handlers_market_bot.go`** — embedded routing:

- in-process calls for status/config/lifecycle/logs via `embeddedBot`
- external/proxy paths removed in Phase 3

### `go.mod`

- `modernc.org/sqlite` added

### Config additions (`config.yaml`)

```yaml
market_bot_enabled: false          # true = start embedded goroutine
market_bot_cache_db: ""            # defaults to ~/.dune-admin/market-bot-cache.db
market_bot_item_data: ""           # defaults to item-data.json in working dir
market_bot_buy_interval: 5m
market_bot_list_interval: 30m
market_bot_buy_threshold: 1.05
market_bot_max_buys: 50
broker_user: ""                    # AMQP username (env: BROKER_USER)
broker_pass: ""                    # AMQP password (env: BROKER_PASS)
broker_jwt_secret: ""              # HMAC key for CaptureJWT (env: BROKER_JWT_SECRET)
```

---

## Phase 3 — Unified deployment artifacts ⏳ In progress

### `deploy/Dockerfile` (update)

- Builder: `FROM golang:1.26.3`, compiles `./cmd/dune-admin` with `CGO_ENABLED=0`
- `COPY item-data.json ./` — needed by embedded bot
- Runtime: `FROM debian:bookworm-slim`

### `deploy/docker-compose.yml` (update)

- Single `dune-admin` service
- Volume for SSH key, config file
- Named volume `market-bot-cache` mounted at `/data`
- Remove any market-bot service remnants

### `deploy/k8s/dune-admin.yaml` (new)

- Namespace `dune-admin`
- Deployment + Service + ConfigMap + Secret + PVC
- ConfigMap: DB host/port/name, bot intervals, `MARKET_BOT_ENABLED`
- Secret: DB password, `BROKER_USER`, `BROKER_PASS`, `BROKER_JWT_SECRET`
- PVC: `market-bot-cache` (1Gi ReadWriteOnce) → `/data`
- Liveness/readiness: `GET /api/v1/status`

---

## Phase 4 — Cognitive complexity refactor ⏳ Pending

34 open issues (#18–51). Plan:

1. `make gocognit` to confirm current list
2. `/batch` skill — parallel agents grouped by file
3. Each agent: extract helpers until function is below threshold
4. `make verify` must pass clean

---

## Verification checklist

- [x] `make build` compiles
- [x] `make test-race` passes
- [x] `make lint` passes
- [ ] `docker build -f deploy/Dockerfile .` succeeds (Phase 3)
- [ ] `kubectl apply --dry-run=client -f deploy/k8s/dune-admin.yaml` (Phase 3)
- [ ] Smoke: `./bin/dune-admin` starts; `market_bot_enabled: true` starts bot loop (Phase 2 — testing now)
- [ ] Smoke: `market_bot_enabled: false` disables bot cleanly (Phase 2)

---

## Issues addressed

| # | Type | Phase | Status |
|---|------|-------|--------|
| #10 | chore: remove capture mode | Phase 0 | ✅ |
| #11 | bug: malformed INI block | Phase 0 | ✅ |
| #12 | bug: array lines duplicate | Phase 0 | ✅ |
| #13 | bug: WebSocket origin | Phase 0 | ✅ |
| #14 | bug: ampExecutor SSH | Phase 0 | ✅ |
| #18–51 | refactor: cognitive complexity | Phase 4 | ⏳ |
