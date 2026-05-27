# Phase 3 — Unified Deployment Artifacts

**Status:** In progress ⏳  
**Branch:** `refactor/phase-3-deploy`  
**PR base:** Phase 2 PR

---

## Scope

Replace the current `deploy/Dockerfile` and `deploy/docker-compose.yml` (minimal stubs from Phase 1) with production-ready versions that cover all deployment targets. Create the unified `deploy/k8s/dune-admin.yaml`.

---

## Files to create / update

### `deploy/Dockerfile`

```dockerfile
FROM golang:1.26.3 AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o dune-admin ./cmd/dune-admin

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /build/dune-admin .
COPY --from=builder /build/item-data.json .
COPY --from=builder /build/tags-data.json .
EXPOSE 8080
ENTRYPOINT ["./dune-admin"]
```

Key change: add `item-data.json` and `tags-data.json` to the runtime stage.

### `deploy/docker-compose.yml`

```yaml
services:
  dune-admin:
    build:
      context: ..
      dockerfile: deploy/Dockerfile
    ports:
      - "8080:8080"
    volumes:
      - ./sshKey:/app/sshKey:ro
      - ${HOME}/.dune-admin:/root/.dune-admin
      - market-bot-cache:/data
    environment:
      SSH_KEY: /app/sshKey
      LISTEN_ADDR: :8080

volumes:
  market-bot-cache:
```

### `deploy/k8s/dune-admin.yaml`

Single file containing: Namespace → ConfigMap → Secret → PVC → Deployment → Service.

`make render-k8s` should render an inline-values manifest (`deploy/k8s/dune-admin.rendered.yaml`)
from `~/.dune-admin/config.yaml` so setup-discovered values (e.g. DB credentials from kubectl setup)
flow straight into the deploy artifact.

**ConfigMap keys:** `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_SCHEMA`, `LISTEN_ADDR`, `CONTROL`, `MARKET_BOT_ENABLED`, `MARKET_BOT_BUY_INTERVAL`, `MARKET_BOT_LIST_INTERVAL`

**Secret keys:** `DB_PASS`, `BROKER_USER`, `BROKER_PASS`, `BROKER_JWT_SECRET`

**PVC:** `market-bot-cache` — 1Gi ReadWriteOnce, mounted at `/data`

**Probes:** `GET /api/v1/status` (liveness delay 15s period 30s, readiness delay 5s period 10s)

**Image:** `ghcr.io/icehunter/dune-admin:latest` (or `image: dune-admin:local` for local builds)

---

## Checklist

- [ ] `docker build -f deploy/Dockerfile .` succeeds from repo root
- [ ] `docker compose -f deploy/docker-compose.yml build` succeeds
- [ ] `kubectl apply --dry-run=client -f deploy/k8s/dune-admin.yaml` passes
- [ ] Container starts, serves `/api/v1/status` → 200
- [ ] `market_bot_enabled: true` starts bot loop inside container
- [ ] PVC correctly persists SQLite cache across pod restarts
- [x] External market-bot mode removed (embedded-only runtime/API/UI/docs)
