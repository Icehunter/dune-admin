---
paths: "**/*.go"
---

# Go Architecture Standards

## CRITICAL: Flat Package main

**dune-admin's Go backend is a single `package main`.** All `.go` files live directly in
`cmd/dune-admin/`. Do NOT create sub-packages or `internal/` sub-directories. This is an
explicit project constraint — don't refactor it away.

## File Responsibilities

| File pattern | Purpose |
| --- | --- |
| `main.go` | Config loading, flag parsing, startup wiring |
| `server.go` | HTTP mux registration, CORS, `jsonOK`/`jsonErr`/`decode` |
| `connection.go` | Global state: `globalDB`, `globalSSH`, `globalExecutor`, `globalControl` |
| `db.go` | **All** Postgres queries (pgx/v5) — nothing else |
| `model.go` | Shared domain types |
| `handlers_*.go` | HTTP handlers, one file per feature area |
| `security_test.go` | `isReadOnlySQL`, `isValidK8sName`, `originAllowed` tests |

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

Even though the package is flat, use interfaces for anything that needs to be mocked in tests:

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
