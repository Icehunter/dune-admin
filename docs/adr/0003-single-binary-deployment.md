# ADR 0003 — Ship a single binary and container image

**Status:** Accepted  
**Date:** 2025-05-27

## Context

Today two binaries are deployed: `dune-admin` (the admin API + web UI) and `market-bot` (the market-making loop). This requires two container images, two Dockerfiles, two k8s Deployments, and two separate build/push pipelines.

After ADR 0002 (embed as library), the market bot runs inside the dune-admin process. There is no longer any reason to build or ship a separate binary.

## Decision

Produce a single `dune-admin` binary from `cmd/dune-admin/` that embeds the market bot goroutine. The `deploy/Dockerfile` builds only this binary. One container image covers both functions.

The market bot loop is opt-in via `market_bot_enabled` in `config.yaml`; disabling it adds zero overhead (the goroutine is never started).

## Consequences

- Container image is slightly larger (adds `modernc.org/sqlite` and bot logic), but SQLite is a pure Go library so no CGO and no runtime C deps
- Image build time increases marginally (one extra dependency)
- External-bot proxy mode is removed; this remains a single-binary, embedded-bot deployment
- Release versioning is unified: one version tag covers both features
