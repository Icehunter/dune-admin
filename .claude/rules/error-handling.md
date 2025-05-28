---
paths: "**/*.go"
---

# Go Error Handling Standards

## Core Principles

- Return errors explicitly — never panic for recoverable errors
- Wrap errors with context: `fmt.Errorf("operation failed: %w", err)`
- Log errors with context using stdlib `log`
- Return appropriate HTTP status codes in handlers
- Always check error returns — never use `_` to discard errors

## Basic Error Handling

```go
result, err := doSomething()
if err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}
```

## Error Wrapping

Build a clear error chain so failures are traceable:

```go
func cmdFetchPlayer(ctx context.Context, db *pgxpool.Pool, id int64) (*playerInfo, error) {
    row := db.QueryRow(ctx, `SELECT account_id FROM dune.player_state WHERE account_id = $1`, id)
    var p playerInfo
    if err := row.Scan(&p.AccountID); err != nil {
        return nil, fmt.Errorf("fetch player %d: %w", id, err)
    }
    return &p, nil
}
```

## Sentinel Errors

Define sentinel errors for specific conditions:

```go
var (
    errNotFound     = errors.New("resource not found")
    errInvalidInput = errors.New("invalid input")
    errUnauthorized = errors.New("unauthorized")
)

func getPlayer(id int64) (*playerInfo, error) {
    p, err := db.fetch(id)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, errNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("get player %d: %w", id, err)
    }
    return p, nil
}
```

## HTTP Error Responses

Use `jsonErr` from `server.go` — never write directly to `http.ResponseWriter`:

```go
func handleGetPlayer(w http.ResponseWriter, r *http.Request) {
    idStr := r.PathValue("id")
    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        jsonErr(w, fmt.Errorf("invalid player ID"), http.StatusBadRequest)
        return
    }

    p, err := cmdFetchPlayer(r.Context(), globalDB, id)
    if err != nil {
        if errors.Is(err, errNotFound) {
            jsonErr(w, fmt.Errorf("player not found"), http.StatusNotFound)
            return
        }
        log.Printf("handleGetPlayer: %v", err)
        jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
        return
    }

    jsonOK(w, p)
}
```

## HTTP Status Codes

| Code | When to use |
| --- | --- |
| 200 | Success |
| 201 | Resource created |
| 204 | Success with no body |
| 400 | Validation / bad input |
| 401 | Authentication required |
| 403 | Authenticated but forbidden |
| 404 | Resource not found |
| 500 | Unexpected server error |
| 503 | Dependency unavailable (e.g. `globalDB == nil`) |

## Error Handling Checklist

- [ ] All errors checked (no `_` to ignore)
- [ ] Errors wrapped with context (`%w`)
- [ ] Sentinel errors used for specific conditions
- [ ] HTTP handlers return appropriate status codes via `jsonErr`
- [ ] Sensitive data not included in error messages or logs
- [ ] Error chains preserve original error for `errors.Is`/`errors.As`

## Anti-Patterns

```go
// Bad — ignores error
result, _ := doSomething()

// Bad — loses context
if err != nil { return err }

// Bad — panic for recoverable error
if err != nil { panic(err) }

// Good
result, err := doSomething()
if err != nil {
    return fmt.Errorf("context here: %w", err)
}
```
