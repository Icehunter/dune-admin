# ADR 0007 — Persistent volume for SQLite market-bot cache

**Status:** Implemented (Phase 3)  
**Date:** 2025-05-27

## Context

The market bot uses SQLite (via `modernc.org/sqlite`) to cache the category tree pulled from the game's Postgres database. The cache path is currently `--cachedb /data/market-bot-cache.db`.

When running in a container, the SQLite file must survive pod restarts; otherwise the bot re-fetches the entire category tree on every start (slow, and puts unnecessary load on the DB).

## Decision

In **k8s**: add a `PersistentVolumeClaim` (`market-bot-cache`, 1Gi, ReadWriteOnce) mounted at `/data` in the Deployment. The cache path defaults to `/data/market-bot-cache.db`.

In **docker-compose**: add a named volume `market-bot-cache` mounted at `/data`.

The cache path is configurable via `market_bot_cache_db` in `config.yaml` (and `MARKET_BOT_CACHE_DB` env var) so operators can use a different path or an NFS-backed volume.

Running **locally** (no container): the default path is `~/.dune-admin/market-bot-cache.db`, consistent with the existing config directory convention.

## Consequences

- Operators must ensure a StorageClass with `ReadWriteOnce` is available in their cluster (true for all standard k8s distributions)
- If the PVC is deleted, the cache is lost and the bot rebuilds it on next start — non-fatal but slow
- The SQLite file is not backed up by default; operators who want it included in backups should mount the same PVC in a backup job
