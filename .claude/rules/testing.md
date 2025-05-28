---
paths: "**/*_test.go"
---

# Go Testing Standards

## Test-Driven Development (TDD) — CRITICAL

**These rules are NON-NEGOTIABLE:**

1. **Write tests first**: Never write implementation without tests first
2. **Tests define requirements**: Include full expectations and error cases before any implementation
3. **Test all error paths**: Cover every unhandled/uncaught case
4. **Test edge cases**: Nil values, empty inputs, boundary conditions
5. **Red-Green-Refactor**: Write failing test → Make it pass → Refactor
6. **Mock external dependencies**: Don't test the DB driver — mock it and test YOUR logic

## Test Organization

- Test files must have `_test.go` suffix
- Place test files alongside source files
- Use table-driven tests for multiple scenarios
- Test both success and error cases
- Test concurrent operations with `-race` flag

### Table-Driven Test Pattern

```go
func TestHandleGetPlayer_Success(t *testing.T) {
    tests := []struct {
        name       string
        playerID   string
        dbResult   *playerInfo
        dbErr      error
        wantStatus int
    }{
        {
            name:       "returns player JSON",
            playerID:   "42",
            dbResult:   &playerInfo{AccountID: 42, CharacterName: "Narisa"},
            wantStatus: http.StatusOK,
        },
        {
            name:       "returns 500 on db error",
            playerID:   "42",
            dbErr:      errors.New("connection refused"),
            wantStatus: http.StatusInternalServerError,
        },
        {
            name:       "returns 400 on bad id",
            playerID:   "not-a-number",
            wantStatus: http.StatusBadRequest,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // set up mock, call handler, assert status
        })
    }
}
```

## Mocking External Dependencies

- Use interfaces for all dependencies injected into handlers/functions
- Create mock implementations for tests
- Do NOT test pgx, net/http, or other standard library behaviour

### Simple Mock Pattern

```go
type mockDB interface {
    fetchPlayer(ctx context.Context, id int64) (*playerInfo, error)
}

type stubDB struct {
    result *playerInfo
    err    error
}

func (s *stubDB) fetchPlayer(_ context.Context, _ int64) (*playerInfo, error) {
    return s.result, s.err
}
```

## Time-Based Testing

- **Never use `time.Sleep` in tests** — flaky and slow
- Inject a clock interface for any time-dependent logic
- Use `quartz.NewMock(t)` from `github.com/coder/quartz` if the dependency is already present

## Coverage Goals

- Core business logic: >90%
- API handlers: >85%
- Storage/query layer: >85%

## Running Tests

```bash
make test           # go test ./...
make test-race      # go test -race ./...  (required for concurrent code)
```

## Test Requirements Checklist

Before considering a test complete:

- [ ] Test written BEFORE implementation
- [ ] All success paths tested
- [ ] All error paths tested
- [ ] Edge cases covered (nil, empty, boundaries)
- [ ] External dependencies mocked
- [ ] Concurrent code tested with `-race`
- [ ] Test names are descriptive
- [ ] Table-driven pattern used where multiple cases exist
