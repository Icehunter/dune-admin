# Provider: local

Use this provider when dune-admin runs on the same machine as the Dune server and there is no Kubernetes or Docker involved — e.g. LGSM, bare-metal, or any setup where the game server is managed by shell commands.

> If you run CubeCoders AMP, prefer `control: amp` and follow [SETUP_AMP.md](SETUP_AMP.md).

```
dune-admin (same machine)
  ├─ shell commands → server lifecycle (start/stop/restart/status)
  └─ TCP (127.0.0.1) → PostgreSQL (localhost:15432)
```

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| **Go 1.21+** | `brew install go` or <https://go.dev/dl/> |
| **PostgreSQL reachable** | `db_host` must be accessible from where dune-admin runs |
| **Shell commands** | Optional — only needed for start/stop/restart/status in the Battlegroup tab |

## Quick start (wizard)

```bash
make setup
# Select: local
# Enter DB host, port, credentials
# Enter optional shell commands for server control
make build   # builds frontend + dune-admin binary
./dune-admin
```

## Manual config (`~/.dune-admin/config.yaml`)

```yaml
control: local

db_host: 127.0.0.1
db_port: 15432
db_user: dune
db_pass: yourpassword
db_name: dune
db_schema: dune

# Shell commands for Battlegroup tab (all optional):
cmd_start:   "amp start DuneAwakening"
cmd_stop:    "amp stop DuneAwakening"
cmd_restart: "amp restart DuneAwakening"
cmd_status:  "amp status DuneAwakening"

# If RabbitMQ runs inside a container (e.g. AMP uses Podman internally),
# set this prefix and it will be prepended to all rabbitmqctl calls:
# broker_exec_prefix: "podman exec AMP_MehDune01"
# broker_exec_prefix: "docker exec my-broker"

# Optional:
backup_dir: /home/dune/backups
listen_addr: :8080
scrip_currency: 1
```

### AMP example

```yaml
control: local
db_host: 127.0.0.1
db_port: 15432
db_user: postgres
db_pass: yourpassword
db_name: dune
db_schema: dune
cmd_start:   "ampinstmgr start DuneAwakening01"
cmd_stop:    "ampinstmgr stop DuneAwakening01"
cmd_restart: "ampinstmgr restart DuneAwakening01"
cmd_status:  "ampinstmgr status DuneAwakening01"
```

### LGSM example

```yaml
control: local
db_host: 127.0.0.1
db_port: 15432
db_user: dune
db_pass: yourpassword
db_name: dune
db_schema: dune
cmd_start:   "/home/dune/duneserver start"
cmd_stop:    "/home/dune/duneserver stop"
cmd_restart: "/home/dune/duneserver restart"
cmd_status:  "/home/dune/duneserver status"
```

### DB only (no server control)

Leave all `cmd_*` fields empty. The Battlegroup tab will show an error for start/stop/restart but everything else — players, inventory, DB browser, etc. — works normally.

> **Note:** `cmd_*` fields are only read from `~/.dune-admin/config.yaml` — they have no env var equivalents. Use `make setup` or edit the file directly.

## What works

| Feature | Supported |
|---------|-----------|
| Battlegroup status | Partial — runs `cmd_status` and shows raw output |
| Start / stop / restart | Yes — runs the configured shell commands |
| Update / backup | Not supported |
| Pod/process list | Not supported |
| Log streaming | Not supported — tail your own log files |
| DB access | Yes — direct TCP to `db_host:db_port` |
| RabbitMQ broker commands | Yes — runs `rabbitmqctl` directly if available in `$PATH` |
| Backup download / upload | Yes — through local file I/O |
| Backup restore | Yes — `pg_restore` run locally |

## Troubleshooting

**Start/stop does nothing** — the shell commands run as the same user that launched dune-admin. Make sure that user has permission to run the AMP/LGSM commands. Test manually first: `ampinstmgr start DuneAwakening01`.

**DB connection fails** — verify PostgreSQL is listening on the configured `db_host:db_port`. For AMP, the DB is usually on `127.0.0.1:5432` or a custom port; check your AMP instance settings.

**RabbitMQ broker command fails** — `rabbitmqctl` must be in `$PATH` for the user running dune-admin. Run `which rabbitmqctl` to verify.
