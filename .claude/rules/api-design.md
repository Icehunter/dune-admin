---
paths: "cmd/dune-admin/handlers_*.go, cmd/dune-admin/server.go"
---

# API Design Standards

## Route Registration

Routes use Go 1.22+ stdlib pattern routing registered in `server.go`:

```go
mux.HandleFunc("GET /api/v1/players",          handleGetPlayers)
mux.HandleFunc("GET /api/v1/players/{id}",     handleGetPlayer)
mux.HandleFunc("POST /api/v1/players/{id}/give-items", handleGiveItems)
```

- All API routes are prefixed `/api/v1/`
- Path parameters extracted via `r.PathValue("id")`
- Static files and the SPA are served separately (not via these handlers)

## Response Helpers

Always use the helpers from `server.go` — never write to `http.ResponseWriter` directly:

```go
jsonOK(w, v)          // 200 + JSON-encoded v
jsonErr(w, err, code) // code + {"error": err.Error()}
decode(r, &v)         // JSON-decode request body into v; returns error on failure
```

## Standard Handler Pattern

```go
func handleGetFoo(w http.ResponseWriter, r *http.Request) {
    // 1. Guard globals
    if globalDB == nil {
        jsonErr(w, errors.New("database not connected"), http.StatusServiceUnavailable)
        return
    }

    // 2. Extract and validate input
    idStr := r.PathValue("id")
    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        jsonErr(w, fmt.Errorf("invalid id"), http.StatusBadRequest)
        return
    }

    // 3. Call db.go command
    result, err := cmdFetchFoo(r.Context(), globalDB, id)
    if err != nil {
        if errors.Is(err, errNotFound) {
            jsonErr(w, fmt.Errorf("not found"), http.StatusNotFound)
            return
        }
        log.Printf("handleGetFoo: %v", err)
        jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
        return
    }

    // 4. Return success
    jsonOK(w, result)
}
```

## Cmd Pattern (db.go)

Command functions in `db.go` return a typed result or error:

```go
func cmdFetchFoo(ctx context.Context, db *pgxpool.Pool, id int64) (*fooRow, error) {
    row := db.QueryRow(ctx,
        `SELECT id, name FROM dune.foos WHERE id = $1`, id)
    var f fooRow
    if err := row.Scan(&f.ID, &f.Name); err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, errNotFound
        }
        return nil, fmt.Errorf("fetch foo %d: %w", id, err)
    }
    return &f, nil
}
```

## Security Constraints

- **SQL endpoint**: guarded by `isReadOnlySQL` — only SELECT/EXPLAIN/SHOW/WITH allowed
- **K8s names**: validated by `isValidK8sName` before any shell/kubectl invocation
- **CORS**: enforced by `originAllowed`; origins configured via `ALLOWED_ORIGINS` env var
- **⚠️ NO backend auth today**: the SPA sends a Clerk `Bearer` token, but the Go backend does **not**
  verify it — there is no auth middleware and no per-endpoint authorization. Every endpoint
  (including destructive ones) is reachable by anyone who can hit the listen address. `jwt_helpers.go`
  only re-signs the *game* server's broker `ServiceAuthToken`; it is **not** admin auth. Closing this
  gap (verify the Clerk session JWT in middleware, add an admin/player role model) is the top
  security priority — see `.claude/rules/security.md`. When adding any new endpoint, do **not** assume
  the caller is authenticated.

## Middleware

CORS and recovery are applied globally in `server.go`. Handler-level guards handle
auth and input validation. Keep cross-cutting concerns out of individual handlers.

## API Design Checklist

- [ ] Route registered with correct method + path in `server.go`
- [ ] `globalDB == nil` guarded before use
- [ ] Input validated, bad input → 400 via `jsonErr`
- [ ] SQL lives in `db.go`, not in the handler
- [ ] `jsonOK` / `jsonErr` used exclusively (no direct `w.Write`)
- [ ] Error logged before returning 500
- [ ] Sensitive data not included in error response body
- [ ] Context passed through to all DB calls
