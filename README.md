# dune-admin

<img width="2172" height="724" alt="image" src="https://github.com/user-attachments/assets/facd62b8-3c06-4f92-8d2c-cf06b46e983e" />

Web-based admin panel for a Dune Awakening private server. Works against any deployment topology: CubeCoders AMP (podman or docker), k3s/k8s over SSH, Docker containers, or a bare-metal install.

---

## 🙏 Thank You

**[@dakkinbyte](https://github.com/dakkinbyte)** deserves special recognition for substantial ongoing contributions: bug fixes, feature work across the dashboard, issue triage, and keeping the project moving. The breadth and consistency of that work has made a real difference.

---

A huge thank you to **[@adainrivers](https://github.com/adainrivers)** and the [**dune-dedicated-server-manager**](https://github.com/adainrivers/dune-dedicated-server-manager) project.

The RabbitMQ server command integration in dune-admin (the envelope format, auth token, AMQP publish path, and the complete catalogue of working server commands) was made possible by the research and live-testing work done in that project. Without it, we would not have known which commands actually work over MQ, what the correct field names are, or that the outer envelope must be base64-encoded before publishing via `rabbitmqctl eval`.

If you run a Dune Awakening private server, check out their project, a full-featured dedicated server manager with a Tauri desktop frontend.

---

## Quick install

On a fresh Ubuntu 22.04 / 24.04 host with passwordless sudo and your game-server stack already running:

```bash
curl -fsSL https://raw.githubusercontent.com/Icehunter/dune-admin/main/scripts/install.sh \
  | bash
```

The script detects your OS and architecture, downloads the matching binary from the latest GitHub release, and installs it into `/opt/dune-admin/`. No build toolchain required — the release binary already embeds the frontend SPA. It refuses to overwrite a running service and leaves a `.prev` backup for one-step rollback. See `--help` for flags (`--version`, `--install-dir`, `--service-user`).

When the script finishes it prints the next manual steps: run the setup wizard, apply the sudoers entry it generates, drop a systemd unit, and start the service.

For development or non-Ubuntu hosts, see [Manual build](#manual-build).

---

## Providers

Pick the provider that matches your game-server topology. Each guide covers prerequisites, wizard answers, and provider-specific config keys.

| Provider | Use when | Guide |
|----------|----------|-------|
| **amp** | Game server runs under CubeCoders AMP (host, or a podman/docker container) | [SETUP_AMP.md](SETUP_AMP.md) |
| **kubectl** | Game server runs in k3s/K8s on a remote VM | [SETUP_KUBECTL.md](SETUP_KUBECTL.md) |
| **docker** | Game server runs as Docker containers (compose or standalone) | [SETUP_DOCKER.md](SETUP_DOCKER.md) |
| **local** | Game server runs on the same machine (bare metal, LGSM, custom) | [SETUP_LOCAL.md](SETUP_LOCAL.md) |

---

## Setup wizard

After install, configure dune-admin with the built-in wizard:

```bash
cd /opt/dune-admin
./dune-admin -setup
```

It asks which control plane to use, then prompts for the settings that provider needs (instance name, paths, DB credentials, etc.). When you select `amp`, the wizard auto-detects instances via `ampinstmgr -l` and pre-fills prompts with discovered values, so you can typically accept the defaults straight through. For container topology it also probes the container to discover the actual game install path, so the wizard isn't pinned to any one AMP module's directory layout.

When done it writes `~/.dune-admin/config.yaml` (mode 600, never committed) and prints a sudoers entry to copy into `/etc/sudoers.d/dune-admin`.

Re-run the wizard any time your configuration changes.

---

## Deploy modes

The same binary supports three deployment shapes:

**Single-binary on a host (AMP, local Go, k3s port-forward)**

The binary serves both the API and the SPA from `./dist` next to itself. `scripts/install.sh` lays this out for you. The simplest model: one process, one port, no CDN.

**k3s / k8s cluster**

A unified manifest deploys dune-admin into a cluster with PostgreSQL and RabbitMQ alongside. Helper scripts handle kubeconfig pull, image build/import, manifest render/apply, and port-forward:

```bash
./deploy.sh        # macOS / Linux
./deploy.ps1       # Windows
```

See [SETUP_KUBECTL.md](SETUP_KUBECTL.md) for full options and troubleshooting.

**Hosted SPA + local backend**

Run the binary as an API-only process and serve the SPA from Cloudflare Pages (or any static host). The SPA prompts for a backend URL on first load, stored in localStorage. The backend adds CORS headers automatically, no extra config needed.

```bash
make deploy-web    # builds and pushes the SPA to Cloudflare Pages
```

Modern browsers allow HTTPS pages to reach HTTP localhost without mixed-content errors, so `https://your-site.pages.dev` → `http://localhost:8080` works out of the box.

---

## Configuration

Config is loaded in this order (first match wins per field):

1. `~/.dune-admin/config.yaml`, written by `dune-admin -setup`
2. `.env` in the working directory, legacy fallback for existing installs
3. Environment variables
4. Command-line flags

### Common fields

| Env var | Flag | Default | Description |
|---------|------|---------|-------------|
| `CONTROL` | `-control` | *(auto)* | Control plane: `amp`, `kubectl`, `docker`, or `local` |
| `SSH_HOST` | `-host` | - | VM `host:port`, tunnels all connections through SSH when set |
| `SSH_USER` | `-user` | `dune` | SSH user |
| `SSH_KEY` | `-key` | *(auto-detected)* | SSH private key path (library mode only) |
| `SSH_MODE` | `-ssh-mode` | `library` | SSH transport: `library` (golang.org/x/crypto/ssh) or `command` (shells out to the OS `ssh` client — honours `~/.ssh/config`, ProxyJump and ssh-agent; never reads a private key) |
| `SSH_EXTRA_OPTS` | `-ssh-extra-opts` | - | Extra `ssh -o` options for command mode (space-separated) |
| `DB_HOST` | `-dbhost` | `127.0.0.1` | PostgreSQL host or Docker DNS name |
| `DB_PORT` | `-dbport` | `15432` | PostgreSQL port |
| `DB_USER` | `-dbuser` | `dune` | PostgreSQL user |
| `DB_PASS` | `-dbpass` | - | PostgreSQL password |
| `DB_NAME` | `-dbname` | `dune` | PostgreSQL database name |
| `DB_SCHEMA` | `-schema` | `dune` | PostgreSQL schema |
| `CONTROL_NAMESPACE` | `-control-ns` | *(auto-discovered)* | K8s namespace (kubectl only) |
| `AUTO_DISCOVER` | `-auto-discover` | `false` | Fill empty DB/RMQ/Director fields from the running game-server process args at connect time (command mode + kubectl). Explicit config always wins |
| `BROKER_GAME_ADDR` | `-broker-game` | - | mq-game broker `host:port` |
| `BROKER_ADMIN_ADDR` | `-broker-admin` | - | mq-admin broker `host:port` |
| `BACKUP_DIR` | `-backup-dir` | - | Backup directory path |
| `LISTEN_ADDR` | `-addr` | `:8080` | HTTP listen address |
| `SCRIP_CURRENCY` | `-scripcurrency` | `1` | Scrip currency ID |

Provider-specific fields (`docker_gameserver`, `amp_instance`, `cmd_start`, etc.) have no env var equivalents and are set via the wizard or `config.yaml` directly. See the provider guides for the full list.

### Dashboard authentication

Off by default — without it, **anyone who can reach the dashboard port has full control of your
game server**. If you expose dune-admin over a public domain or IP, enable it:

```yaml
auth_enabled: true

# Method 1 — local username/password (set the hash with: dune-admin --set-password)
auth_local_username: admin
auth_local_password_hash: "$2a$10$..."   # bcrypt; never store the plaintext

# Method 2 — Discord OAuth (bring your own Discord application)
auth_discord_enabled: true
auth_discord_client_id: "your-app-client-id"
auth_discord_client_secret: "your-app-client-secret"
# Reuses discord_bot_token + discord_guild_id for membership/role lookup.
# Add scheme://host/api/v1/auth/discord/callback as a redirect in the Discord app.

# Owners always get full access: the guild owner, the local account, plus:
auth_owner_discord_ids: ""        # comma-separated Discord user IDs
auth_owner_role_ids: ""           # comma-separated Discord role IDs
auth_session_ttl_hours: 24
```

```yaml
auth_guest_enabled: false         # optional: anonymous read-only guest sessions
```

How it works:

- All login methods (local, Discord, guest) issue a signed `HttpOnly` session cookie; no
  credentials are stored in the browser. Local and Discord login can be enabled together.
- Discord users must be members of `discord_guild_id`. Their roles map to ~28 fine-grained
  **capabilities** (e.g. `players:write`, `broadcast:send`, `battlepass:manage`) via the
  **Permissions** tab; the matrix persists to `~/.dune-admin/permissions.yaml`. The special
  **Guests** and **Default member** rows extend what anonymous guests / unmapped members get.
- Additional **local users** (username/password, no Discord) live in `dune-admin.db` and are
  managed from the Permissions tab with per-user capability assignments.
- The Permissions tab is editable by owners and anyone granted `auth:manage`.
- Every session starts from a minimal floor: player/world/market read access, server status,
  and the battlepass track. Everything else — events, welcome kits, bot status, schedules,
  exports, config, database, logs, backups — requires an explicit grant.
- Role changes in Discord are picked up within 15 minutes; kicked members lose access on the
  same schedule.
- Every authenticated mutation is recorded in `~/.dune-admin/audit.log` (JSON lines).
- Locked out (Discord misconfigured, password lost)? Run `dune-admin --set-password` on the
  host — it works offline and writes straight to `config.yaml`.
- The login endpoint is rate-limited (5/min per IP) and security headers (CSP, X-Frame-Options,
  HSTS behind TLS) are emitted while auth is enabled.

TLS is still your reverse proxy's job (Caddy, nginx, Cloudflare Tunnel) — dune-admin marks
cookies `Secure` automatically when it sees `X-Forwarded-Proto: https`.

### Market bot

dune-admin runs the market bot **embedded**, an in-process goroutine that shares the main DB pool. Enable in `config.yaml`:

```yaml
market_bot_enabled: true
market_bot_cache_db:       ~/.dune-admin/market-bot-cache.db   # auto-created (SQLite, pure Go)
market_bot_item_data:      ./item-data.json                    # falls back to standard search paths
market_bot_buy_interval:   5m
market_bot_list_interval:  30m
market_bot_buy_threshold:  1.05
market_bot_max_buys:       50
```

The Market tab's lifecycle buttons map to in-process actions:

- **Start** -> `Resume()`: flips the bot's enabled flag back on
- **Stop** -> `Pause()`: flips the bot's enabled flag off (goroutine stays resident)
- **Restart** -> `Restart()`: pauses, reinitializes the exchange, resumes

Config edits in the Market tab apply directly to the live runtime config. A full process restart only matters when changing the cache DB path or item-data path.

See [ADR 0002](docs/adr/0002-embed-market-bot-as-library.md) and [ADR 0004](docs/adr/0004-in-process-bot-lifecycle.md) for the design rationale.

### Welcome Kits

An opt-in feature that auto-grants a configured item package to every player **once, on first login**. Manage it in the **Welcome Kits** tab, or in `config.yaml`:

```yaml
welcome_package_enabled: true
welcome_package_scan_interval_secs: 30
welcome_package_active_version: v1
welcome_packages:
  - version: v1
    items:
      - { template: AluminiumBar, qty: 5, quality: 0 }
```

dune-admin keeps a library of named packages plus an active-version pointer. An in-process scanner grants the active package once per `(player, version)`, tracked in a persistent SQLite ledger at `~/.dune-admin/welcome-package.db` (so a restart never re-grants). Bumping the active version re-issues to everyone. It defaults **off** (it mutates every player's inventory) and delivers items through the same live-RMQ + DB-fallback path as manual give-items.

**Message of the Day (MOTD).** The same tab (**Welcome Kits → Config**) also has a separate **Message of the Day** card: an in-game message whispered to a player from the GM persona **every time they join** — every session, unlike the once-per-version welcome above. It's decoupled from the package system, so it works even with no active package. Use `{player}` in the message for the player's character name, and leave the sender blank to use the seeded GM persona. A presence tracker diffs the online set on each scan tick to detect joins, and seeds a silent baseline on startup so a restart never re-messages players already in-game. Defaults **off**; configure and toggle it live (no restart) in the UI.

### Server settings

The **Server Settings** tab manages gameplay config (mining/vehicle output, PvP & security zones, sandstorm/sandworm toggles, building limits, item deterioration, server name/password, …). How it writes depends on the provider:

- **amp**: settings are written through AMP's Web API, because AMP regenerates the game INIs from its own config on every start (a direct file edit would be clobbered). This requires AMP API credentials in `config.yaml`:

  ```yaml
  amp_api_user: admin        # an AMP panel login for the instance
  amp_api_pass: yourpassword
  amp_api_port: 8081         # instance ADS API port (default 8081)
  ```

  A game restart applies them; dune-admin's **Restart** recycles the AMP container (`<runtime> restart`, matching `amp_container_runtime`), which is what actually cycles the game processes. The container-runtime sudoers grant must allow this. See [SETUP_AMP.md](SETUP_AMP.md#server-settings-gameplay-config).
- **docker / kubectl / local**: settings are written straight to `UserGame.ini` / `UserEngine.ini`; no API credentials needed.

Either way, a server restart is required for changes to take effect.

### SSH key lookup order

When `SSH_KEY` / `-key` is not set, dune-admin checks these paths in order:

1. `./sshKey`
2. `~/.dune-admin/sshKey`
3. `~/.ssh/dune`
4. `~/.ssh/id_ed25519`
5. `~/.ssh/id_rsa`

For building your `~/.ssh/config` itself — modular `config.d` layout, jumphost (ProxyJump) chains, per-platform ssh-agent setup, and connection multiplexing — see [SETUP_SSH.md](SETUP_SSH.md).

---

## Tabs

Tabs are organised into a grouped left sidebar.

**Operations**

| Tab | What it does |
|-----|--------------|
| **Battlegroup** | Start/stop game-server pods; stream container logs; manage backups |
| **Logs** | Stream live logs; view cheat detection events |
| **Database** | Run raw SQL against the game DB; browse tables |
| **Server Settings** | Edit UE5 server settings; writes go to `UserGame.ini` in a managed block |

**Player World**

| Tab | What it does |
|-----|--------------|
| **Players** | Browse players; view/edit inventory, specs, currency, XP, faction rep; journey nodes; teleport; session history |
| **Storage** | Browse server-side storage containers |
| **Bases** | Browse and export player base placements |
| **Blueprints** | View all unlockable blueprint definitions |

**Economy**

| Tab | What it does |
|-----|--------------|
| **Market Bot** | View live market listings; control the embedded market bot |
| **Welcome Kits** | Auto-grant a configured item package to every player once, on first login — plus an optional Message-of-the-Day whispered on every join |

---

## Manual build

For development or if you'd rather not use `scripts/install.sh`:

```bash
git clone https://github.com/Icehunter/dune-admin
cd dune-admin

# Frontend (Vite + Rolldown, needs node-linker=hoisted for the native binding)
echo 'node-linker=hoisted' > web/.npmrc
cd web && pnpm install --frozen-lockfile && pnpm build && cd ..

# Backend
make linux         # cross-compile a Linux amd64 binary (dune-admin-linux)
# or
make build         # host OS binary (bin/dune-admin), plus the frontend build
```

Then run the setup wizard:

```bash
./dune-admin -setup
```

Prerequisites: Go 1.26+, Node 20.19+ or 22.12+, pnpm 10.28+, `make`.

> **Windows note:** `make build` works from PowerShell or `cmd.exe` as long as
> [GNU Make](https://gnuwin32.sourceforge.net/packages/make.htm) is installed (e.g.
> `winget install GnuWin32.Make` or `choco install make`). The binary is named
> `dune-admin.exe`. For `make dev`, `make verify`, and the `make version-*` targets,
> run from a **Git Bash** shell (right-click the folder -> "Git Bash Here"); those
> recipes use POSIX shell features that cmd.exe can't run.

### Building without a HeroUI Pro license

The frontend uses `@heroui-pro/react`, which requires a `HEROUI_AUTH_TOKEN` to install. If
you only need to run the app (not modify the frontend), each release ships a prebuilt
`dune-admin-web-dist.tar.gz` asset. A single make target downloads it and produces a fully
embedded binary:

```bash
make build-prebuilt
```

This fetches the dist that matches the local `VERSION` file (falling back to the latest
release), extracts it, and compiles the Go binary with the frontend embedded. No Node, pnpm,
or Pro license required — only Go and `make`.

**Backend-only development** is also possible without a license: run `make dev-backend` for
hot-reload and point a browser at the prebuilt `dist` from `make build-prebuilt` (served by
the binary at `http://localhost:8080`). Frontend hot-reload (`make dev`) requires the Pro
license.

---

## Development

dune-admin ships pre-commit and pre-push hooks that mirror the CI quality gate. Set them up once:

```bash
make hooks   # wires git's core.hooksPath to .githooks/
make tools   # caches golangci-lint, govulncheck, gocognit, gosec, air (via Go's go tool mechanism)
```

After that, every `git commit` runs `gofmt -w` + `go vet` + `golangci-lint` on staged Go files (and `markdownlint-cli2` on `.md` files), and every `git push` adds `gosec`, `govulncheck`, and `go test -race`. Bypass with `--no-verify` if you really need to.

You can run the full suite by hand at any time:

```bash
make verify   # fmt-check + vet + test-race + vulncheck + lint + gocognit
```

> **Windows note:** the race detector needs cgo. Either install MinGW (e.g. `winget install BrechtSanders.WinLibs.POSIX.UCRT`) or skip race testing locally and rely on the pre-push hook on a Linux dev box or CI.

---

## Makefile targets

| Target | Description |
|--------|-------------|
| `make setup` | Run the interactive setup wizard |
| `make build` | Build frontend + Go binary for the host OS |
| `make build-prebuilt` | Download prebuilt dist from a release + embed-build (no Pro license needed) |
| `make linux` | Cross-compile a Linux amd64 binary (`dune-admin-linux`) |
| `make dev` | Run backend + frontend in live mode (Air + Vite HMR) |
| `make dev-server` | Run backend only (`go run ./cmd/dune-admin`) |
| `make web` | Build the frontend only |
| `make dist-archive` | Package `cmd/dune-admin/dist` as a release asset tarball (CI use) |
| `make deploy-web` | Build and deploy the SPA to Cloudflare Pages |
| `make render-k8s` | Render `deploy/k8s/dune-admin.rendered.yaml` from `~/.dune-admin/config.yaml` |
| `make k8s-dry-run` | Render and run `kubectl apply --dry-run=client` |
| `make test` | Run tests |
| `make verify` | Run all quality checks (fmt-check, vet, test-race, vulncheck, lint, gocognit) |
| `make hooks` | Install the pre-commit + pre-push hooks |
| `make tools` | Cache the dev toolchain (`golangci-lint`, `gosec`, etc.) |

---

## Item data (optional)

`item-data.json` provides friendly item names, stack limits, volume, tier, and rarity. It ships with the repo.

Without it the panel still works; inventory items show raw template IDs.

---

## Architecture

dune-admin is a single Go binary that exposes a REST API over the game's PostgreSQL + RabbitMQ stack. The React SPA can be served by the same binary (from `./dist`) or hosted independently (Cloudflare Pages). A control-plane abstraction (`amp` / `kubectl` / `docker` / `local`) drives lifecycle operations, log streaming, INI editing, and broker access against whatever topology you're running.

For design rationale and trade-offs, see the architecture decision records in [`docs/adr/`](docs/adr/):

- [0001 - Standard Go project layout](docs/adr/0001-standard-go-layout.md)
- [0002 - Embed market bot as `internal/marketbot` library](docs/adr/0002-embed-market-bot-as-library.md)
- [0003 - Ship a single binary and container image](docs/adr/0003-single-binary-deployment.md)
- [0004 - In-process bot lifecycle control](docs/adr/0004-in-process-bot-lifecycle.md)
- [0005 - Ring-buffer for embedded bot log streaming](docs/adr/0005-ring-buffer-log-streaming.md)
- [0006 - Replace per-project k8s manifests with one unified manifest](docs/adr/0006-unified-k8s-manifest.md)
- [0007 - Persistent volume for SQLite market-bot cache](docs/adr/0007-sqlite-cache-storage.md)
- [0008 - Extend config.yaml for embedded-bot settings](docs/adr/0008-config-yaml-extensions.md)

## Guides

- [Discord topologies](docs/discord-topologies.md) — how game servers, Discord guilds, channels, and per-server characters map together (single guild, shared guild, guild-per-server, and mixed).
