# Phase 1 — Standard Go Project Layout

**Status:** Complete ✅  
**Branch:** `refactor/phase-1-go-layout`  
**PR:** #53  
**Base:** `chore/phase-0-bug-fixes` (#52)

---

## Scope

Move all Go source files to `cmd/dune-admin/`, move deployment artifacts to `deploy/`, and update all tooling to use the new paths. No behaviour changes — pure structural move.

See [ADR 0001](../adr/0001-standard-go-layout.md) for the rationale.

---

## Changes

### Source files

All root-level `*.go` files (35 source + 1 test) moved to `cmd/dune-admin/`:

```
*.go  →  cmd/dune-admin/*.go
```

`package main` declarations unchanged. Module name stays `dune-admin`. Import paths within the package are unaffected (same module, same package name).

### Deployment artifacts

```
Dockerfile        →  deploy/Dockerfile
docker-compose.yml  →  deploy/docker-compose.yml
```

`deploy/Dockerfile` build target updated: `go build ./cmd/dune-admin` (was `go build .`).

### Makefile

```makefile
CMD := ./cmd/dune-admin   # new variable

build:    $(GO) build ... -o $(BIN) $(CMD)
linux:    GOOS=linux ... go build -o dune-admin-linux $(CMD)
dev-server: go run $(CMD)
setup:    go run $(CMD) -setup
test:     go test $(PKG)         # PKG := ./...  (already covered ./... including cmd/)
vet:      go vet $(PKG)
```

### `.air.toml`

```toml
cmd = "go build -o ./tmp/dune-admin ./cmd/dune-admin"
exclude_dir = [..., "deploy"]
```

### `.gocognit-ignore`

Removed two stale entries referencing the now-deleted `capture.go`:

- `captureBroker  capture.go:191:1`
- `runCapture     capture.go:98:1`

---

## Verification

```bash
make build          # compiles cleanly
make test-race      # all tests pass
make lint           # no issues
docker build -f deploy/Dockerfile .   # image builds
```

Git correctly detected all file movements as renames (100% similarity) in the commit diff.
