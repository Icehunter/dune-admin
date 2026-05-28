# ADR 0004 — In-process bot lifecycle control

**Status:** Implemented (Phase 2)  
**Date:** 2025-05-27

## Context

`handlers_market_bot.go` previously implemented `start`, `stop`, and `restart` by shelling out to:

- `kubectl scale deployment/<name> --replicas=0/1`
- `docker start/stop/restart <container>`

After embedding the bot as a goroutine, there is no container or deployment to scale.

## Decision

`internal/marketbot.Instance` exposes three lifecycle methods:

```go
func (i *Instance) Pause()                          // sets Enabled=false in live Config
func (i *Instance) Resume()                         // sets Enabled=true
func (i *Instance) Restart(ctx context.Context) error // pauses, re-runs Init(), resumes
```

`Pause`/`Resume` apply a JSON patch to the bot's thread-safe `Config` via `Config.Apply`. The tick loop already reads `snap.Enabled` on every minute boundary — no goroutine restart required.

`Restart` calls `Exchange.Init(ctx, catalog)` to reload pricing, re-ping the DB, and repopulate the category cache, then re-enables the loop.

`handleMarketBotExec` routes directly to these methods. External kubectl/docker lifecycle paths were removed with external mode.

## Consequences

- `start`/`stop` in embedded mode is immediate and in-memory — no latency, no partial failure
- `restart` flushes any in-flight tick and re-runs `Init`; catalog changes (e.g. updated `item-data.json`) are NOT picked up by restart — process restart is required for that
- The API surface seen by the frontend (`POST /api/v1/market-bot/exec`) is unchanged
- Embedded lifecycle is the only supported market-bot mode
