# ADR 0002 — Embed market bot as `internal/marketbot` library

**Status:** Implemented (Phase 2)  
**Date:** 2025-05-27

## Context

`dune-market-bot` was a separate Go module and binary. dune-admin controlled it via HTTP proxy (`botProxy`) and container lifecycle commands (`kubectl scale`, `docker restart`). This required two images, two build pipelines, two k8s Deployments, and a live network dependency between the admin API and the bot.

Three integration models were considered:

| Model | Description | Rejected reason |
|-------|-------------|-----------------|
| **Embed as goroutine library** | Pull source into `internal/marketbot/`; call `Run(ctx, cfg)` | — (chosen) |
| Child-process supervisor | dune-admin spawns the bot binary | Still two binaries; adds process management complexity |
| Go workspace | Separate modules in one repo | Doesn't give single-process or single-image benefit |

## Decision

Copy all `*.go` files from `dune-market-bot` into `internal/marketbot/`. Change `package main` → `package marketbot`. Expose:

```go
// Run starts the market bot. Blocks until ctx is cancelled.
// Returns (*Instance, error) — Instance is valid immediately after startup.
func Run(ctx context.Context, cfg BotConfig) (*Instance, error)

// Instance exposes live handles for the host process.
type Instance struct {
    API  *APIServer  // in-process HTTP handler (nil if APIAddr == "")
    Sink *LogSink    // ring-buffer log fan-out
    // unexported: cfg, catalog, ex, pool
}

func (i *Instance) Pause()
func (i *Instance) Resume()
func (i *Instance) Restart(ctx context.Context) error
func (i *Instance) StatusSnapshot() any
```

`flag.Parse()` and `os.Exit` removed from the package; the caller owns lifecycle.

The old `dune-market-bot` repository is archived once this integration ships.

## Consequences

- `modernc.org/sqlite` added to dune-admin's `go.mod`
- `item-data.json` must be present in the container image (already exists at repo root)
- Bot tests (`pricing_test.go`, `config_test.go`, `api_test.go`) are now part of `make test` in dune-admin
- External/proxy mode was removed in Phase 3; dune-admin now runs market-bot embedded only
- `catalog.go`: `loadCatalog()` becomes `loadCatalog(path string)` — no `flag` dependency
