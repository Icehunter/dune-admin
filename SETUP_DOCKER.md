# Provider: docker

Use this provider when your Dune server runs as Docker containers (e.g. alongside a compose stack) and dune-admin can reach the Docker daemon directly — either co-located on the same host or SSH'd into a Docker host.

```
dune-admin
  ├─ docker CLI → container lifecycle + logs
  ├─ docker exec → RabbitMQ broker commands
  └─ TCP (Docker DNS) → PostgreSQL (database:15432)
```

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| **Go 1.21+** | `brew install go` or <https://go.dev/dl/> |
| **Docker CLI** | Must be in `$PATH` |
| **Docker access** | The user running dune-admin must be able to run `docker` (i.e. in the `docker` group or running as root) |

### If dune-admin runs on a different host

Add `SSH_HOST` to your config so all commands and DB connections tunnel through SSH:

```yaml
ssh_host: 192.168.0.72:22
ssh_user: dune
ssh_key: /home/you/.ssh/key
```

With SSH set, `docker` CLI commands run on the remote host and DB connections are tunnelled — no ports need to be exposed.

## One container per map

A Dune install runs **one game-server container per map/partition**, not a single
gameserver. A stock layout looks like this:

```text
dune-server-overmap          seabass-server:<tag>                game server
dune-server-deepdesert-1-8   seabass-server:<tag>                game server
dune-server-survival-1       seabass-server:<tag>                game server
dune-server-gateway          seabass-server-gateway:<tag>        not a game server
dune-text-router             seabass-server-text-router:<tag>    not a game server
dune-rmq-game / dune-rmq-admin                                   brokers
dune-director                                                    partition metadata
```

dune-admin auto-detects the game servers by image: containers whose image repository
is exactly `seabass-server` are game servers, which is what separates them from the
gateway and text-router. If no image matches (a retagged or privately built image), it
falls back to containers named `dune-server-*`.

Lifecycle actions apply to **every** game container, and each partition can be restarted
on its own.

## Quick start (wizard)

```bash
make setup
# Select: docker
# Confirm the auto-detected game-server containers when prompted
make build   # builds frontend + dune-admin binary
./dune-admin
```

The wizard runs `docker ps`, lists the containers it found, pre-selects the ones it
detected as game servers, and asks you to confirm. It also defaults the broker names and
the Director URL from the same listing.

## Manual config (`~/.dune-admin/config.yaml`)

```yaml
control: docker

# Container names — must match exactly what `docker ps` shows.
# Omit docker_gameservers entirely to let dune-admin auto-detect them:
docker_gameservers:
  - dune-server-overmap
  - dune-server-deepdesert-1-8
  - dune-server-survival-1

docker_broker_game: dune-rmq-game      # optional — for broker command path
docker_broker_admin: dune-rmq-admin    # optional — for broker command path

# Optional — fills in players, queue depth, and dimension labels per partition:
director_url: http://127.0.0.1:11717

# Database — use Docker DNS name or IP:
db_host: database       # service name in your compose file
db_port: 15432
db_user: dune
db_pass: yourpassword
db_name: dune
db_schema: dune

# Optional:
backup_dir: /backups
broker_game_addr: dune-rmq-game:5672   # defaults to docker_broker_game container DNS if omitted
broker_admin_addr: dune-rmq-admin:5672
broker_tls: false
listen_addr: :8080
scrip_currency: 1
```

> **Note:** `docker_*` and `cmd_*` fields are only read from `~/.dune-admin/config.yaml` — they have no env var equivalents. Use `make setup` or edit the file directly.

## Typical compose layout

Your compose file doesn't need to change. dune-admin just needs the container names:

```yaml
services:
  server-overmap:
    container_name: dune-server-overmap      # ← auto-detected game server
  server-survival-1:
    container_name: dune-server-survival-1   # ← auto-detected game server
  database:
    container_name: dune-db
  rmq-game:
    container_name: dune-rmq-game            # ← docker_broker_game
  rmq-admin:
    container_name: dune-rmq-admin           # ← docker_broker_admin
```

The legacy singular `docker_gameserver` key still works and is treated as a
one-entry list, so existing single-container configs keep running unchanged.

## What works

| Feature | Supported |
|---------|-----------|
| Battlegroup status | Yes — one row per game container, with partition and port |
| Player counts / dimension labels | Yes, when `director_url` is set |
| Start / stop / restart | Yes — applies to every game container |
| Restart a single partition | Yes — restarts just that container |
| Update / backup | Not supported (no `battlegroup.sh`) |
| Container list | Yes — `docker ps` |
| Log streaming | Yes — `docker logs -f` |
| DB access | Yes — direct TCP to `db_host:db_port` |
| RabbitMQ broker commands | Yes — `docker exec` into broker container |
| Backup download / upload | Yes — through executor file I/O |
| Backup restore | Yes — `pg_restore` run via executor |

## Troubleshooting

**"No game servers found" / empty server table** — auto-detection did not match any container. Run `docker ps` and set `docker_gameservers` explicitly to the game-server container names.

**A non-game container shows up as a game server** — set `docker_gameservers` explicitly; the explicit list always wins over auto-detection.

**Players / queue / dimension columns are empty** — set `director_url` (the `dune-director` container publishes port `11717`).

**"docker inspect failed"** — the container name is wrong or Docker is not running. Check with `docker ps` and update `docker_gameservers` in config.yaml.

**DB connection fails** — verify `db_host` matches the container's DNS name or IP. Inside a compose network, use the service/container name directly (e.g. `database`). Outside the network, use the host IP and a mapped port.

**Logs show nothing** — confirm the container names are correct. Container names are exact-match, not prefix.
