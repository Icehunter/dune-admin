# ADR 0008 — Extend config.yaml for embedded-bot settings

**Status:** Implemented (Phase 2), updated (Phase 3)  
**Date:** 2025-05-27

## Context

Embedding the bot (ADR 0002) requires dedicated in-process settings plus moving hardcoded broker credentials to config.

## Decision

New fields added to `appConfig` in `cmd/dune-admin/main.go`:

```yaml
# Embedded market bot (in-process)
market_bot_enabled: false           # bool; true starts embedded bot
market_bot_cache_db: ""             # SQLite path; default: ~/.dune-admin/market-bot-cache.db
market_bot_item_data: ""            # item-data.json path; default: ./item-data.json
market_bot_buy_interval: 5m         # time.Duration string
market_bot_list_interval: 30m
market_bot_buy_threshold: 1.05      # float64
market_bot_max_buys: 50             # int

# Broker credentials (moved from hardcoded constants)
broker_user: ""                     # AMQP username (env: BROKER_USER); default: dune_cap
broker_pass: ""                     # AMQP password (env: BROKER_PASS)
broker_jwt_secret: ""               # base64 HMAC key for CaptureJWT (env: BROKER_JWT_SECRET)
```

`market_bot_enabled: true` activates the embedded goroutine via `marketbot.Run(ctx, BotConfig{...})` in `main()`.

`broker_user`/`broker_pass`/`broker_jwt_secret` are loaded through `setEnvIfMissing` so the env-var path also works for containerised deployments.

## Consequences

- Existing installs with no new fields default to `market_bot_enabled: false`
- The `BotConfig` struct in `internal/marketbot` exactly mirrors these fields
- `handlers_market_bot.go` routes all market-bot endpoints to the embedded instance
- `broker_user`/`broker_pass` default to `dune_cap`/`DuneCap2026!` via `brokerCredentials()` for backward compat
- External proxy fields (`market_bot_addr`, `market_bot_token`, `market_bot_container`, `market_bot_namespace`) were removed in Phase 3
