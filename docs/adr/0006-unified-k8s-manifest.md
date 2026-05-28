# ADR 0006 — Replace per-project k8s manifests with one unified manifest

**Status:** Implemented (Phase 3)  
**Date:** 2025-05-27

## Context

There are currently two separate Kubernetes manifests:

- `dune-admin/docker-compose.yml` — local Docker development
- `dune-market-bot/k8s/market-bot.yaml` — k8s Deployment, Service, ConfigMap, Secret for the bot

After ADR 0003 (single binary), both functions ship in one image. Maintaining two manifests targeting different images creates confusion and drift risk.

## Decision

Create `deploy/k8s/dune-admin.yaml` as the single authoritative k8s manifest. It includes:

- **Namespace** — `dune-admin`
- **Deployment** — single replica, single container (`dune-admin` image), all env vars wired from ConfigMap and Secret
- **Service** — ClusterIP, port 8080
- **ConfigMap** — non-secret config: DB host/port/name, bot intervals, listen addr, `MARKET_BOT_ENABLED`
- **Secret** — DB password and broker credentials (`BROKER_USER`, `BROKER_PASS`, `BROKER_JWT_SECRET`)
- **PersistentVolumeClaim** — `market-bot-cache`, used for SQLite (see ADR 0007)
- Liveness probe: `GET /api/v1/status` → 200
- Readiness probe: same

Both old manifests (`docker-compose.yml` at repo root, `dune-market-bot/k8s/market-bot.yaml`) are removed. Migration notes are added to `SETUP_DOCKER.md`.

## Consequences

- Operators who previously ran two pods now run one; the market bot is not a separately addressable network endpoint in-cluster
- The PVC must be provisioned by the cluster's default StorageClass (or operators set `storageClassName` explicitly)
- `kubectl apply --dry-run=client` is part of Phase 3 verification
