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
| **AMP host access** | Run dune-admin on the AMP host, or set `ssh_host` to run remotely over SSH |
| **Sudoers grant** | dune-admin user must run `ampinstmgr`, the container runtime (`podman`, or `docker` when `amp_container_runtime: docker`), and `tee` as AMP user without prompts. The container-runtime grant covers both `exec` (logs/broker) and `restart` (applying server settings). |

Example sudoers entry (adjust user/path names as needed) — **podman backend (default)**:

```bash
dune-admin ALL=(amp) NOPASSWD: /usr/bin/ampinstmgr, /usr/bin/podman, /usr/bin/tee
```

If your AMP container runs on **docker** (`amp_container_runtime: docker`), grant `docker` instead:

```bash
dune-admin ALL=(amp) NOPASSWD: /usr/bin/ampinstmgr, /usr/bin/docker, /usr/bin/tee
```

The same runtime grant is what lets dune-admin's **Restart** action cycle the container
(`<runtime> restart <container>`) — see [Server settings](#server-settings-gameplay-config) for why a
real container restart is required.

## Quick start

Install with the one-liner from the [README](README.md#quick-install), then run the setup wizard:

```bash
cd /opt/dune-admin
./dune-admin -setup
# Select: amp
# Fill AMP instance/container/user details
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

# AMP Web API — required to manage gameplay settings under AMP (see "Server settings" below).
# These are an AMP panel login for the instance; the API is reached in-container at
# 127.0.0.1:<amp_api_port>, so no host port needs to be exposed.
amp_api_user: admin
amp_api_pass: yourpassword
amp_api_port: 8081   # instance ADS API port (default 8081)
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

## Server settings (gameplay config)

The **Server Settings** tab manages gameplay knobs — mining/vehicle output, PvP/security zones,
sandstorm and sandworm toggles, building limits, item deterioration, server name/password, and so on.

Under AMP this path is different from every other provider, and it matters:

- **AMP owns the game INI files.** AMP regenerates `UserEngine.ini` / `UserGame.ini` from its own
  config on every start, so editing those files directly is silently clobbered. dune-admin therefore
  writes settings through **AMP's Web API** (`Core/SetConfig`), which persists them in AMP's config and
  survives restarts. This is why the `amp_api_user` / `amp_api_pass` / `amp_api_port` credentials above
  are required — without them, saving a setting under AMP returns an error rather than failing silently.
- **A restart is required to apply.** Saving writes the value to AMP's config; the running game only
  picks it up on the next start. dune-admin's **Restart** action recycles the AMP **container**
  (`<runtime> restart <container>`), which is the action that actually reaps the
  `DuneSandboxServer` processes — `ampinstmgr` alone leaves them running, so the change never takes
  effect. The container restart also briefly cycles the in-container PostgreSQL and broker; dune-admin
  reconnects to the database automatically afterwards.

Typical flow: change a value in **Server Settings** → **Save** → **Restart** (Operations) → the new
value is live in-game.

Other providers (docker / kubectl / local) have no AMP layer to clobber the files, so they write the
INIs directly and do **not** need the `amp_api_*` credentials.

## What works

| Feature | Supported |
|---------|-----------|
| Battlegroup status | Yes |
| Start / stop | Yes (`ampinstmgr`) |
| Restart | Yes — cycles the container (`<runtime> restart`) so game processes actually recycle |
| Process list | Yes |
| Log streaming | Yes |
| DB access | Yes (direct or SSH tunnel) |
| Broker command path | Yes (`broker_exec_prefix`) |
| Server settings (gameplay) | Yes — written via AMP Web API (`amp_api_*`); restart to apply |
| INI read/write | Yes (`ampExecutor` writes as AMP user; non-gameplay/raw sections) |

## Troubleshooting

**`sudo` prompts or permission denied** — fix `/etc/sudoers.d/*` for the dune-admin user and AMP user.

**INI changes fail** — verify `server_ini_dir` and that AMP user owns `UserGame.ini` / `UserEngine.ini`.

**Director page (Operations) says "could not read director config" while the Web Interfaces link works** — the page reads the amp-owned `director_config.ini` on the host, while the link is just an HTTP proxy, so `director_url` being correct doesn't help. The file read needs read access: either run the dune-admin service **as the `amp` user** (it owns the `0700` file, no sudo needed), or add `/usr/bin/cat` to the AMP sudoers grant for the service user. `director.log` shows `read …/director_config.ini: exit status 1` when neither is in place.

**Broker commands fail** — set `broker_exec_prefix` to the exact `podman exec` (or `docker exec`) wrapper used on your AMP host.

**Saving a server setting returns an error (502)** — dune-admin could not reach or authenticate to the AMP Web API. Check `amp_api_user` / `amp_api_pass` (an AMP panel login) and `amp_api_port` (default `8081`), and that the instance ADS is running inside the container.

**A server setting saved but did nothing in-game** — settings apply on the next game start. Use dune-admin's **Restart** (which does `<runtime> restart <container>`), not just `ampinstmgr`; confirm the container-runtime sudoers grant above is in place so the restart can run.
