# Provider: amp (CubeCoders AMP)

Use this provider when your Dune server is managed by AMP (`ampinstmgr`) and RabbitMQ/Postgres live in the AMP stack (host-native, or in a podman- or docker-backed container). Set `amp_container_runtime` to match (`podman` default, or `docker`).

```
dune-admin
  ├─ ampinstmgr / podman exec → lifecycle + logs + broker ops
  ├─ elevated INI writes as amp user
  └─ TCP → PostgreSQL (host or SSH-tunnelled host)
```

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| **Go 1.21+** | `brew install go` or <https://go.dev/dl/> |
| **AMP host access** | Run dune-admin on the AMP host, or set `ssh_host` to run remotely over SSH |
| **Sudoers grant** | dune-admin user must run `ampinstmgr`, the container runtime (`podman`, or `docker` when `amp_container_runtime: docker`), and `tee` as AMP user without prompts |

Example sudoers entry (adjust user/path names as needed):

```bash
dune-admin ALL=(amp) NOPASSWD: /usr/bin/ampinstmgr, /usr/bin/podman, /usr/bin/tee
```

## Quick start (wizard)

```bash
make setup
# Select: amp
# Fill AMP instance/container/user details
make build   # builds frontend + dune-admin binary
./dune-admin
```

## Manual config (`~/.dune-admin/config.yaml`)

```yaml
control: amp

# Optional if running dune-admin from a different machine:
# ssh_host: 192.168.0.72:22
# ssh_user: dune-admin
# ssh_key: /home/you/.ssh/amp-host

db_host: 127.0.0.1
db_port: 15432
db_user: postgres
db_pass: yourpassword
db_name: dune
db_schema: dune

amp_instance: DuneAwakening01
amp_container: AMP_DuneAwakening01
amp_user: amp
amp_log_path: /AMP/duneawakening/logs
server_ini_dir: /home/amp/.ampdata/instances/DuneAwakening01/duneawakening/server/state

# Optional:
amp_use_container: true
amp_container_runtime: docker   # podman (default) | docker — match your AMP container backend
amp_data_root: /AMP/duneawakening
director_url: http://127.0.0.1:11717
broker_exec_prefix: "sudo -i -u amp podman exec AMP_DuneAwakening01"
listen_addr: :18080   # avoids collision with AMP web panel on :8080
```

## Embedded market bot (recommended in AMP)

```yaml
market_bot_enabled: true
market_bot_cache_db: /home/amp/.dune-admin/market-bot-cache.db
market_bot_item_data: /path/to/dune-admin/item-data.json
market_bot_buy_interval: 5m
market_bot_list_interval: 30m
market_bot_buy_threshold: 1.05
market_bot_max_buys: 50
```

External market-bot mode is removed; use embedded mode for AMP deployments.

## What works

| Feature | Supported |
|---------|-----------|
| Battlegroup status | Yes |
| Start / stop / restart | Yes (`ampinstmgr`) |
| Process list | Yes |
| Log streaming | Yes |
| DB access | Yes (direct or SSH tunnel) |
| Broker command path | Yes (`broker_exec_prefix`) |
| INI read/write | Yes (`ampExecutor` writes as AMP user) |

## Troubleshooting

**`sudo` prompts or permission denied** — fix `/etc/sudoers.d/*` for the dune-admin user and AMP user.

**INI changes fail** — verify `server_ini_dir` and that AMP user owns `UserGame.ini` / `UserEngine.ini`.

**Broker commands fail** — set `broker_exec_prefix` to the exact `podman exec` (or `docker exec`) wrapper used on your AMP host.
