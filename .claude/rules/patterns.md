---
paths: "**/*.go"
---

# Go Design Patterns

## Dependency Injection

Inject dependencies through function parameters, not global mutation. Global state is set
once at startup in `connectAll()` — handlers receive what they need via the request context
or direct parameter injection in tests.

```go
// Production handler uses globals
func handleGetPlayer(w http.ResponseWriter, r *http.Request) {
    id, err := parsePlayerID(r)
    if err != nil {
        jsonErr(w, err, http.StatusBadRequest)
        return
    }
    info, err := cmdFetchPlayer(r.Context(), globalDB, id)
    // ...
}

// Testable helper accepts explicit dependency
func cmdFetchPlayer(ctx context.Context, db playerQuerier, id int64) (*playerInfo, error) {
    // ...
}
```

## Journey Cache Invalidation

The journey node cache (`db.go`) has a 30-second TTL. After mutating player data, always invalidate:

```go
// After mutations scoped to a single account
invalidateJourneyCache(accountID)

// After mutations where only playerID is available (no account ID)
invalidateAllJourneyCache()
```

Forgetting this causes stale reads until cache expiry — the player appears unaffected until they relog.

## Functional Options

Use functional options for constructors with many optional settings (e.g. monitor/service types):

```go
type Option func(*ServiceConfig)

func WithTimeout(d time.Duration) Option {
    return func(c *ServiceConfig) { c.timeout = d }
}

func NewService(required string, opts ...Option) *Service {
    s := &Service{required: required, timeout: 5 * time.Second}
    for _, opt := range opts { opt(&s.ServiceConfig) }
    return s
}
```

Don't over-apply — simple constructors with 2-3 params don't need this.

## Avoid Global State Mutation Outside connectAll

All globals (`globalDB`, `globalSSH`, `globalExecutor`, `globalControl`) are set exactly
once in `connectAll()`. Never reassign them from handlers or helpers.

## Market Bot: Player Orders Are Inviolable

The bot must **never** delete, expire, or modify player (non-NPC) exchange orders. Every
`DELETE` or `UPDATE` on `dune_exchange_orders` or related tables must be scoped:

```sql
WHERE owner_id = <botOwnerID> AND is_npc_order = TRUE
```

The buy query must filter expired player orders with `AND expiration_time > gameNow` — not
by deleting them. Add this comment on every relevant block:

```go
// MUST NOT touch player orders — only bot NPC rows
```

## SQL Placement

All Postgres queries live in `db.go`. Handlers call `cmd*` functions from there. This keeps
query logic centralised, testable, and easy to find.

```go
// db.go
func cmdFetchPlayerByName(ctx context.Context, db *pgxpool.Pool, name string) (*playerInfo, error) {
    row := db.QueryRow(ctx, `SELECT account_id, character_name FROM dune.player_state WHERE character_name = $1`, name)
    var p playerInfo
    if err := row.Scan(&p.AccountID, &p.CharacterName); err != nil {
        return nil, fmt.Errorf("fetch player %q: %w", name, err)
    }
    return &p, nil
}

// handlers_players.go
func handleGetPlayerByName(w http.ResponseWriter, r *http.Request) {
    name := r.PathValue("name")
    p, err := cmdFetchPlayerByName(r.Context(), globalDB, name)
    // ...
}
```

Always use the `dune.` schema prefix in queries.
