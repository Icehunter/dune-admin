---
paths: "**/*.go"
---

# Go Architecture Standards

## CRITICAL: Flat HTTP backend, libraries under internal/

**dune-admin's HTTP backend is one flat `package main` in `cmd/dune-admin/`** — keep the server
flat; do NOT split it into sub-packages or nest server files in sub-directories. The ONE exception
is genuinely reusable, standalone libraries with their own lifecycle, which go under `internal/`
(today: `internal/marketbot`, the embedded market bot, run in-process via `marketbot.Run`). Default
to `cmd/dune-admin/`; only reach for a new `internal/` package for a cohesive library, and record
the decision in an ADR. See `docs/adr/0001-standard-go-layout.md` and
`docs/adr/0002-embed-market-bot-as-library.md`.

## File Responsibilities

| File pattern | Purpose |
| --- | --- |
| `main.go` | Config loading, flag parsing, startup wiring |
| `server.go` | HTTP mux registration, CORS, `jsonOK`/`jsonErr`/`decode` |
| `connection.go` | Global state: `globalDB`, `globalSSH`, `globalExecutor`, `globalControl` |
| `db.go` | **All** Postgres queries (pgx/v5) — nothing else |
| `model.go` | Shared domain types |
| `compat.go` | `type Msg = any` and `type Cmd = func() Msg` aliases — legacy Bubble Tea signatures used in db.go; do not remove or change |
| `handlers_*.go` | HTTP handlers, one file per feature area |
| `security_test.go` | `isReadOnlySQL`, `isValidK8sName`, `originAllowed` tests |

### internal/ libraries

`internal/marketbot` is the embedded market bot — its own Go package (not `package main`), run
in-process via `marketbot.Run` from `main.go`, exposing its own HTTP API. It has its own
`*_test.go` coverage. New cohesive libraries with their own lifecycle may live under `internal/`
with an ADR in `docs/adr/`; everything that is part of the HTTP server stays flat in
`cmd/dune-admin/`.

## Go Conventions

- Standard Go naming (exported/unexported, no `Impl` suffix)
- Keep functions focused and testable (single responsibility)
- Meaningful variable names — avoid single letters except loop indices
- Cognitive complexity ≤15 per function (enforced by `make gocognit`)
- Use early returns to reduce nesting

## Global State Pattern

Global state is set once in `connectAll()` (`connection.go`) and never mutated elsewhere.
Handlers must guard before use:

```go
func handleGetPlayers(w http.ResponseWriter, r *http.Request) {
    if globalDB == nil {
        jsonErr(w, errors.New("database not connected"), http.StatusServiceUnavailable)
        return
    }
    // proceed...
}
```

## Interfaces for Testability

Within the flat `cmd/dune-admin` package, use interfaces for anything that needs to be mocked in tests:

```go
// Executor and ControlPlane are already interfaces — extend the same pattern.
type playerStore interface {
    fetchPlayer(ctx context.Context, id int64) (*playerInfo, error)
}
```

Accept interfaces as function parameters; return concrete types from constructors.

## Configuration

- Config loaded via YAML (`~/.dune-admin/config.yaml`), `.env`, env vars, CLI flags (first match wins)
- Validate at startup — fail fast on missing required values
- Key env vars: `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, `DB_NAME`, `LISTEN_ADDR`, `ALLOWED_ORIGINS`

## Cognitive Complexity

Target ≤15 per function. Use extraction and early returns to keep functions readable:

```go
// Before: deep nesting
func processRequest(r *http.Request) (*result, error) {
    if r.Method == "POST" {
        if body != nil {
            // 30 lines
        }
    }
}

// After: extracted helpers, early return
func processRequest(r *http.Request) (*result, error) {
    if r.Method != "POST" {
        return nil, errMethodNotAllowed
    }
    return processPostRequest(r)
}
```
