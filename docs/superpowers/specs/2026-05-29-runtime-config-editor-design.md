# Runtime Config Editor — Design Spec

**Issue:** #86  
**Date:** 2026-05-29  
**Status:** Approved

## Context

dune-admin loads its configuration from `~/.dune-admin/config.yaml` at startup. Operators currently edit this file by hand or re-run the interactive `-setup` wizard. All the backend plumbing for runtime editing already exists (`handlers_config.go`: `handleGetConfig`, `handleSaveConfig`, `applyConfig`, `writeConfigFile`, `preserveMaskedDBPass`), and an orphaned `AppConfigTab.tsx` covers most of the form fields — but neither is wired up to the UI. This spec closes that gap by embedding a full, searchable config editor inside the Settings modal introduced in PR #95.

## Goal

Operators can view and edit `config.yaml` from the running app. Changes take effect immediately via the existing `resetRuntimeConnections()` + `connectAll()` hot-reload path. Fields that require a full restart (e.g. `listen_addr`) are clearly flagged.

---

## Architecture

### Files changed

| File | Change |
|---|---|
| `web/src/App.tsx` | Modal grows to `size="cover"` scroll; imports and renders `SettingsConfigForm` |
| `web/src/components/SettingsConfigForm.tsx` | **New** — self-contained config form (fetch, render, save) |
| `web/src/api/client.ts` | Extend `AppConfig` type with all missing fields; add masking constants |
| `cmd/dune-admin/handlers_config.go` | Extend masking to `BrokerPass` and `BrokerJWTSecret` |
| `web/src/tabs/AppConfigTab.tsx` | **Deleted** — orphaned, never wired, superseded by `SettingsConfigForm` |

No new backend routes are needed. `GET /api/v1/config` and `POST /api/v1/config` are already registered in `server.go`.

---

## Modal Layout

```
Settings  [×]
──────────────────────────────────────
About
  Version │ Commit │ Control │ Built
──────────────────────────────────────
Server Configuration
  [Search fields…]

  Panel: Database
  Panel: SSH
  Panel: Control Plane   ← sections within vary by selected control value
  Panel: Broker
  Panel: Market Bot
  Panel: Advanced

  [Save & Reconnect]
──────────────────────────────────────
Backend URL Override
  (existing inline Save & Reload / Reset)
──────────────────────────────────────
                    [Close]
```

Modal size changes from `sm` to `cover` with `scroll="outside"` to accommodate the full form.

---

## Component: SettingsConfigForm

**Location:** `web/src/components/SettingsConfigForm.tsx`

**Responsibilities:**

- Fetch config via `api.config.get()` on mount; display a spinner while loading
- Render all config sections as `Panel` components from `dune-ui`
- Filter visible fields via a `SearchField` above the panels
- Submit via `api.config.save(form)`; show spinner on the Save button, emit toast on success/error

**Props:** none — fully self-contained

**State:**

- `config: AppConfig | null` — current form values
- `loading: boolean` — initial fetch in progress
- `saving: boolean` — save in progress
- `query: string` — search filter text

### Search behaviour

The `SearchField` filters fields client-side by matching the query string against each field's label and description (case-insensitive). Fields that don't match are hidden. A `Panel` whose every field is hidden collapses entirely. The query resets when the modal closes.

---

## Panel Sections & Fields

Each field renders: label + input + short `text-muted text-xs` description below.

### Panel: Database

| Field | yaml key | Description |
|---|---|---|
| Host | `db_host` | PostgreSQL host the game database is running on |
| Port | `db_port` | PostgreSQL port (default 5432) |
| User | `db_user` | Database user |
| Password | `db_pass` | Masked — send placeholder `••••••••` to keep existing value |
| Database | `db_name` | Database name |
| Schema | `db_schema` | Postgres schema prefix (typically `dune`) |

### Panel: SSH

Always visible. Filling in `ssh_host` enables SSH tunnelling for all DB connections and executor commands.

| Field | yaml key | Description |
|---|---|---|
| Host | `ssh_host` | SSH host (and optional `:port`). Leave blank for local operation |
| User | `ssh_user` | SSH user on the remote host |
| Private key | `ssh_key` | Absolute path to the private key file on this machine |

### Panel: Control Plane

| Field | yaml key | Description |
|---|---|---|
| Control | `control` | How dune-admin manages the game server: `kubectl`, `docker`, `local`, or `amp` |

Conditional sub-fields rendered below based on the selected `control` value:

**kubectl:**

| Field | yaml key | Description |
|---|---|---|
| Namespace | `control_namespace` | Kubernetes namespace where the Dune workloads run |

**docker:**

| Field | yaml key | Description |
|---|---|---|
| Game server | `docker_gameserver` | Container name for the game server process |
| Broker (game) | `docker_broker_game` | Container name for the game RabbitMQ vhost broker |
| Broker (admin) | `docker_broker_admin` | Container name for the admin RabbitMQ vhost broker |
| Database | `docker_db` | Container name for the PostgreSQL instance |

**local:**

| Field | yaml key | Description |
|---|---|---|
| Start command | `cmd_start` | Shell command to start the game server |
| Stop command | `cmd_stop` | Shell command to stop the game server |
| Restart command | `cmd_restart` | Shell command to restart the game server |
| Status command | `cmd_status` | Shell command to query server status |

**amp:**

| Field | yaml key | Description |
|---|---|---|
| Instance | `amp_instance` | ampinstmgr instance name, e.g. `DuneAwakening01` |
| Container | `amp_container` | Podman container name (default: `AMP_<instance>`) |
| User | `amp_user` | OS user that runs AMP (typically `amp`) |
| Log path | `amp_log_path` | In-container log directory |
| Use container | `amp_use_container` | Toggle between containerised (podman exec) and native host mode |
| Data root | `amp_data_root` | Per-game data root inside the container (default `/AMP/duneawakening`) |

### Panel: Broker

Optional. Leave all fields blank to disable broker features (capture, notifications).

| Field | yaml key | Description |
|---|---|---|
| Game addr | `broker_game_addr` | RabbitMQ management address for the game vhost |
| Admin addr | `broker_admin_addr` | RabbitMQ management address for the admin vhost |
| TLS | `broker_tls` | Use TLS for broker connections |
| User | `broker_user` | RabbitMQ user |
| Password | `broker_pass` | Masked — RabbitMQ password |
| JWT secret | `broker_jwt_secret` | Masked — base64 HMAC key for re-signing ServiceAuthTokens. Leave blank to use the built-in default |
| Exec prefix | `broker_exec_prefix` | Prepended to all rabbitmqctl calls, e.g. `podman exec AMP_DuneAwakening01` |

### Panel: Market Bot

| Field | yaml key | Description |
|---|---|---|
| Enabled | `market_bot_enabled` | Run the market bot in-process alongside dune-admin |
| Remote URL | `market_bot_remote_url` | Forward market bot API calls to a standalone bot at this URL instead of running one in-process |
| Remote token | `market_bot_remote_token` | Masked — bearer token for the remote bot |
| Cache DB | `market_bot_cache_db` | Path to the SQLite cache database used by the embedded bot |
| Item data | `market_bot_item_data` | Path to item-data.json used by the embedded bot |
| State path | `market_bot_state` | Path to the JSON file where the bot persists its runtime state |
| Buy interval | `market_bot_buy_interval` | How often the bot checks for buy opportunities (e.g. `5m`) |
| List interval | `market_bot_list_interval` | How often the bot refreshes its listings (e.g. `10m`) |
| Buy threshold | `market_bot_buy_threshold` | Minimum discount ratio before the bot buys |
| Max buys | `market_bot_max_buys` | Maximum concurrent buy orders the bot will place |

### Panel: Advanced

| Field | yaml key | Description |
|---|---|---|
| Listen address ⚠ | `listen_addr` | HTTP listen address — requires a full server restart to change (e.g. `:8080`) |
| Backup directory | `backup_dir` | Path the executor accesses for game backup files |
| Server INI dir | `server_ini_dir` | Directory containing `UserGame.ini` and `UserOverrides.ini` |
| Default INI dir | `default_ini_dir` | Path to `DefaultGame.ini` / `DefaultEngine.ini` base layer |
| Director URL | `director_url` | Optional Battlegroup Director URL — proxied at `/director/` |
| Scrip currency | `scrip_currency` | Item ID used as the scrip currency in the game economy |

---

## Save Flow

1. User edits fields, presses **Save & Reconnect**
2. Button shows spinner, disabled. `api.config.save(form)` called (POST `/api/v1/config`)
3. Backend: `preserveMaskedDBPass` / `preserveMaskedBrokerSecrets` restores real secrets if placeholders sent; writes `config.yaml`; calls `applyConfig` + `resetRuntimeConnections` + `connectAll`
4. **Success:** toast "Config saved — reconnecting…"; `useStatus` poll will reflect new connection state within 5 s
5. **Error:** toast with error message; form stays open with values intact

Masked fields (`db_pass`, `broker_pass`, `broker_jwt_secret`, `market_bot_remote_token`) display the placeholder `••••••••` when the stored value is non-empty. Submitting the placeholder preserves the stored secret; clearing the field and submitting an empty string removes it.

---

## Backend Changes

### `handlers_config.go`

Extend `preserveMaskedDBPass` (or extract a shared helper) to also preserve:

- `BrokerPass` when the client sends `"••••••••"`
- `BrokerJWTSecret` when the client sends `"••••••••"`
- `MarketBotRemoteToken` when the client sends `"••••••••"`

`handleGetConfig` already masks `DBPass`; extend it to mask the three new fields with the same placeholder before responding.

### Tests

Add/extend `main_config_test.go` (or equivalent) with table-driven tests for:

- Masked `BrokerPass` is preserved on save (not overwritten with placeholder)
- Masked `BrokerJWTSecret` is preserved on save
- Masked `MarketBotRemoteToken` is preserved on save
- All three are masked in the GET response when non-empty

---

## Frontend Type Extension (`client.ts`)

`AppConfig` gains all missing fields matching the Go `appConfig` struct. New fields:

```ts
// Docker
docker_gameserver?: string
docker_broker_game?: string
docker_broker_admin?: string
docker_db?: string
// Local
cmd_start?: string
cmd_stop?: string
cmd_restart?: string
cmd_status?: string
// Broker (extended)
broker_user?: string
broker_pass?: string        // masked
broker_jwt_secret?: string  // masked
broker_exec_prefix?: string
// Server INI
server_ini_dir?: string
default_ini_dir?: string
// AMP
amp_instance?: string
amp_container?: string
amp_user?: string
amp_log_path?: string
amp_use_container?: boolean
amp_data_root?: string
director_url?: string
// Market Bot
market_bot_enabled?: boolean
market_bot_cache_db?: string
market_bot_item_data?: string
market_bot_state?: string
market_bot_buy_interval?: string   // Duration rendered as string (e.g. "5m0s")
market_bot_list_interval?: string
market_bot_buy_threshold?: number
market_bot_max_buys?: number
market_bot_remote_url?: string
market_bot_remote_token?: string   // masked
```

The masked placeholder constant `MASKED = "••••••••"` is exported from `client.ts` for use in the form component.

---

## Verification

1. `make verify` passes (Go tests + lint + vulncheck)
2. `pnpm lint && pnpm build` passes
3. Open Settings modal → config form loads with current values
4. Search "password" → only password fields visible, other panels collapse
5. Search "amp" → only AMP panel visible
6. Change `db_host` to a bad value → Save & Reconnect → error toast; DB badge goes red
7. Change back → Save & Reconnect → DB badge goes green
8. Send masked placeholder for `db_pass` → real password preserved in config.yaml
9. Masked fields (`broker_pass`, `broker_jwt_secret`, `market_bot_remote_token`) display `••••••••` when set; GET response confirms masking
10. `listen_addr` field shows restart warning label
