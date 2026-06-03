---
name: tdd-go
description: TDD workflow for dune-admin Go handlers and db.go functions. Use when adding a handler, implementing a feature, or fixing a bug in the Go backend.
---

# TDD — dune-admin Go

## Law

Write the failing test **first**. No production code without a red test.

## Step-by-step

### 1. Define the interface (if needed)

If the function depends on a DB call or external I/O, extract an interface:

```go
type fooQuerier interface {
    fetchFoo(ctx context.Context, id int64) (*fooRow, error)
}
```

Production code uses `globalDB`; tests inject a stub.

### 2. Write the table-driven test

```go
func TestHandleFoo(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        stubResult *fooRow
        stubErr    error
        wantStatus int
    }{
        {"happy path", "42", &fooRow{ID: 42}, nil, http.StatusOK},
        {"db error → 500", "42", nil, errors.New("boom"), http.StatusInternalServerError},
        {"bad id → 400", "abc", nil, nil, http.StatusBadRequest},
        {"nil db → 503", "42", nil, nil, http.StatusServiceUnavailable},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // set up stub, call handler, assert tt.wantStatus
        })
    }
}
```

Run `make test-race` — confirm it **fails** before writing implementation.

### 3. Write minimal implementation

Handler structure:

```go
func handleGetFoo(w http.ResponseWriter, r *http.Request) {
    if globalDB == nil {
        jsonErr(w, errors.New("database not connected"), http.StatusServiceUnavailable)
        return
    }
    id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
    if err != nil {
        jsonErr(w, fmt.Errorf("invalid id"), http.StatusBadRequest)
        return
    }
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
    jsonOK(w, result)
}
```

DB function in `db.go`:

```go
func cmdFetchFoo(ctx context.Context, db *pgxpool.Pool, id int64) (*fooRow, error) {
    row := db.QueryRow(ctx, `SELECT id, name FROM dune.foos WHERE id = $1`, id)
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

### 4. Verify

```bash
make test-race   # all tests pass, no races
make gosec       # no new findings (not covered by make verify)
make verify      # full suite
```

## Checklist

- [ ] Test written and **red** before any implementation
- [ ] All HTTP status codes covered (200, 400, 503, 500, 404)
- [ ] External deps mocked (DB, executor, control plane)
- [ ] SQL lives in `db.go`, uses `dune.` schema prefix
- [ ] `globalDB == nil` guard present
- [ ] `make test-race` green
- [ ] `make gosec` clean
