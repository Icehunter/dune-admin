# ADR 0001 — Adopt standard Go project layout

**Status:** Accepted  
**Date:** 2025-05-27

## Context

All Go source files currently live in the repository root alongside `go.mod`, the `Makefile`, `Dockerfile`, and deployment config. As the project grows (adding `internal/marketbot/`, potentially other sub-packages), the flat layout makes it hard to:

- Distinguish entry-point code from library code
- Prevent external import of internal helpers
- Keep deployment artifacts clearly separated from source

The Go community's de-facto standard layout places `main` packages under `cmd/<name>/` and shared/internal code under `internal/<pkg>/`. The `go` toolchain enforces the `internal/` import restriction natively.

## Decision

Move all root-level `*.go` files into `cmd/dune-admin/` (package declaration stays `package main`). New shared code goes into `internal/`. Deployment artifacts (`Dockerfile`, `docker-compose.yml`, k8s manifests) move into `deploy/`.

The `go.mod` module name remains `dune-admin`. Build target changes from `./` to `./cmd/dune-admin`.

## Consequences

- `make build`, `make test`, all `go tool` invocations must be updated to use `./cmd/dune-admin` or `./...`
- The existing Dockerfile `COPY` and `WORKDIR` paths need updating
- `internal/` enforces that `marketbot` can only be imported by this module — prevents accidental external re-use
- One-time migration commit with no behaviour change; straightforward to review
