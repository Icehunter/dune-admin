---
paths: "**/*.go"
---

# Go Concurrency Standards

## Mutex Usage

- Use `sync.RWMutex` for read-heavy shared state
- Use `sync.Mutex` for write-heavy shared state
- Always `defer` the unlock

```go
type cache struct {
    mu   sync.RWMutex
    data map[int64]*playerInfo
}

func (c *cache) get(id int64) (*playerInfo, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    v, ok := c.data[id]
    return v, ok
}

func (c *cache) set(id int64, p *playerInfo) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.data[id] = p
}
```

## Goroutines and Error Handling

- Use `errgroup` for parallel work with error collection
- Never fire-and-forget a goroutine — always handle the result or error

```go
import "golang.org/x/sync/errgroup"

func fetchAllProviders(ctx context.Context, ids []int64) ([]*playerInfo, error) {
    g, ctx := errgroup.WithContext(ctx)
    results := make([]*playerInfo, len(ids))

    for i, id := range ids {
        i, id := i, id // capture loop variables
        g.Go(func() error {
            p, err := cmdFetchPlayer(ctx, globalDB, id)
            if err != nil {
                return err
            }
            results[i] = p
            return nil
        })
    }

    if err := g.Wait(); err != nil {
        return nil, err
    }
    return results, nil
}
```

## Context Usage

- Always pass `context.Context` as the first parameter
- Use context for cancellation and timeouts on external calls
- Never store context in a struct
- Create child contexts for operations needing their own deadline

```go
// Good
func cmdFetchPlayer(ctx context.Context, db *pgxpool.Pool, id int64) (*playerInfo, error) {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    // ...
}

// Bad — never store context
type service struct {
    ctx context.Context // ❌
}
```

## Journey Cache: Double-Check Locking

The journey node cache in `db.go` uses double-check locking to prevent thundering herd on expiry:

```go
func getJourneyNodes(ctx context.Context, accountID int64) ([]journeyNode, error) {
    // Fast path: read lock
    journeyCacheMu.RLock()
    if entry, ok := journeyCache[accountID]; ok && time.Since(entry.fetchedAt) < journeyCacheTTL {
        nodes := entry.nodes
        journeyCacheMu.RUnlock()
        return nodes, nil
    }
    journeyCacheMu.RUnlock()

    // Slow path: write lock + re-check
    journeyCacheMu.Lock()
    defer journeyCacheMu.Unlock()
    if entry, ok := journeyCache[accountID]; ok && time.Since(entry.fetchedAt) < journeyCacheTTL {
        return entry.nodes, nil
    }

    nodes, err := fetchJourneyNodesFromDB(ctx, accountID)
    if err != nil {
        return nil, err
    }
    journeyCache[accountID] = journeyCacheEntry{nodes: nodes, fetchedAt: time.Now()}
    return nodes, nil
}
```

## Concurrency Best Practices

1. Always test with race detector: `make test-race`
2. Use buffered channels when capacity is known
3. Capture loop variables before spawning goroutines: `id := id`
4. Use `errgroup` for managing multiple goroutines
5. Set timeouts on all context-bound operations
6. Use `sync.Once` for one-time initialisation

## Common Pitfalls

```go
// Bad — all goroutines share the loop variable's last value
for _, id := range ids {
    go func() { fetch(id) }() // ❌
}

// Good — capture before goroutine
for _, id := range ids {
    id := id
    go func() { fetch(id) }() // ✅
}

// Bad — mutex unlock might not run on panic
mu.Lock()
doWork()
mu.Unlock() // ❌

// Good
mu.Lock()
defer mu.Unlock() // ✅
doWork()
```
