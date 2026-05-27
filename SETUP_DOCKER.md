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
| **Go 1.21+** | `brew install go` or https://go.dev/dl/ |
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

## Quick start (wizard)

```bash
make setup
# Select: docker
# Enter container names when prompted
make build
./dune-admin
```

The wizard asks for container names, tests them with `docker inspect`, and asks for DB connection details.

## Manual config (`~/.dune-admin/config.yaml`)

```yaml
control: docker

# Container names — must match exactly what `docker ps` shows:
docker_gameserver: dune-gameserver
docker_broker_game: dune-mq-game      # optional — for capture mode
docker_broker_admin: dune-mq-admin    # optional — for capture mode

# Database — use Docker DNS name or IP:
db_host: database       # service name in your compose file
db_port: 15432
db_user: dune
db_pass: yourpassword
db_name: dune
db_schema: dune

# Optional:
backup_dir: /backups
broker_game_addr: dune-mq-game:5672   # defaults to docker_broker_game container DNS if omitted
broker_admin_addr: dune-mq-admin:5672
broker_tls: false
listen_addr: :8080
scrip_currency: 1
```

> **Note:** `docker_*` and `cmd_*` fields are only read from `~/.dune-admin/config.yaml` — they have no env var equivalents. Use `make setup` or edit the file directly.

## Typical compose layout

Your compose file doesn't need to change. dune-admin just needs the container names:

```yaml
services:
  gameserver:
    container_name: dune-gameserver   # ← docker_gameserver
  database:
    container_name: dune-db
  mq-game:
    container_name: dune-mq-game      # ← docker_broker_game
  mq-admin:
    container_name: dune-mq-admin     # ← docker_broker_admin
```

## What works

| Feature | Supported |
|---------|-----------|
| Battlegroup status | Partial — shows container state, not K8s CRD fields |
| Start / stop / restart | Yes — `docker start/stop/restart` |
| Update / backup | Not supported (no `battlegroup.sh`) |
| Container list | Yes — `docker ps` |
| Log streaming | Yes — `docker logs -f` |
| DB access | Yes — direct TCP to `db_host:db_port` |
| RabbitMQ capture | Yes — `docker exec` into broker container |
| Backup download / upload | Yes — through executor file I/O |
| Backup restore | Yes — `pg_restore` run via executor |

## Troubleshooting

**"docker inspect failed"** — the container name is wrong or Docker is not running. Check with `docker ps` and update `docker_gameserver` in config.yaml.

**DB connection fails** — verify `db_host` matches the container's DNS name or IP. Inside a compose network, use the service/container name directly (e.g. `database`). Outside the network, use the host IP and a mapped port.

**Logs show nothing** — confirm `docker_gameserver` is the correct container name. Container names are exact-match, not prefix.
