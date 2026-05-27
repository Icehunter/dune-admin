# dune-admin

---

## 🙏 Thank You

A huge thank you to **[@adainrivers](https://github.com/adainrivers)** and the [**dune-dedicated-server-manager**](https://github.com/adainrivers/dune-dedicated-server-manager) project.

The RabbitMQ server command integration in dune-admin - the envelope format, auth token, AMQP publish path, and the complete catalogue of working server commands - was made possible by the research and live-testing work done in that project. Without it, we would not have known which commands actually work over MQ, what the correct field names are, or that the outer envelope must be base64-encoded before publishing via `rabbitmqctl eval`.

If you run a Dune Awakening private server, check out their project - it is a full-featured dedicated server manager with a Tauri desktop frontend.

---

Web-based admin panel for a Dune Awakening private server. Supports any deployment topology - K8s/k3s over SSH, Docker containers, AMP/LGSM, or bare metal.

## Providers

Choose the provider that matches your setup:

| Provider | Use when | Guide |
|----------|----------|-------|
| **kubectl** | Server runs in k3s/K8s on a remote VM | [SETUP_KUBECTL.md](SETUP_KUBECTL.md) |
| **docker** | Server runs as Docker containers (compose or standalone) | [SETUP_DOCKER.md](SETUP_DOCKER.md) |
| **local** | Server runs on the same machine - AMP, LGSM, bare metal | [SETUP_LOCAL.md](SETUP_LOCAL.md) |

---

## Quick start

```bash
git clone https://github.com/Icehunter/dune-admin
cd dune-admin

make setup   # interactive wizard - detects provider, discovers config, writes ~/.dune-admin/config.yaml
make build
./dune-admin
```

Open the hosted frontend at **<https://dune-admin.layout.tools>** and point it at `http://localhost:8080`.

See the provider guide above for provider-specific prerequisites (SSH key, Docker access, etc.).

---

## Setup wizard

`make setup` (or `go run . -setup`) runs an interactive wizard that:

1. Asks which control plane to use: `kubectl`, `docker`, or `local`
2. Collects the required settings for that provider
3. Tests connectivity (SSH, Docker, DB)
4. Writes `~/.dune-admin/config.yaml` (chmod 600, never committed)

Re-run `make setup` any time your configuration changes.

---

## Configuration

Config is loaded in this order (first match wins for each field):

1. `~/.dune-admin/config.yaml` - written by `make setup`
2. `.env` in the working directory - legacy fallback for existing installs
3. Environment variables
4. Command-line flags

### Common fields

| Env var | Flag | Default | Description |
|---------|------|---------|-------------|
| `CONTROL` | `-control` | *(auto)* | Control plane: `kubectl`, `docker`, or `local` |
| `SSH_HOST` | `-host` | - | VM host:port - when set, all connections tunnel through SSH |
| `SSH_USER` | `-user` | `dune` | SSH user |
| `SSH_KEY` | `-key` | *(auto-detected)* | SSH private key path |
| `DB_HOST` | `-dbhost` | `127.0.0.1` | PostgreSQL host or Docker DNS name |
| `DB_PORT` | `-dbport` | `15432` | PostgreSQL port |
| `DB_USER` | `-dbuser` | `dune` | PostgreSQL user |
| `DB_PASS` | `-dbpass` | - | PostgreSQL password |
| `DB_NAME` | `-dbname` | `dune` | PostgreSQL database name |
| `DB_SCHEMA` | `-schema` | `dune` | PostgreSQL schema |
| `CONTROL_NAMESPACE` | `-control-ns` | *(auto-discovered)* | K8s namespace (kubectl only) |
| `BROKER_GAME_ADDR` | `-broker-game` | - | mq-game broker `host:port` |
| `BROKER_ADMIN_ADDR` | `-broker-admin` | - | mq-admin broker `host:port` |
| `BACKUP_DIR` | `-backup-dir` | - | Backup directory path |
| `LISTEN_ADDR` | `-addr` | `:8080` | HTTP listen address |
| `SCRIP_CURRENCY` | `-scripcurrency` | `1` | Scrip currency ID |

Provider-specific fields (`docker_gameserver`, `cmd_start`, etc.) have no env var equivalents and must be set via the wizard or config.yaml directly. See the provider guides for the full field list.

### SSH key lookup order

When `SSH_KEY` / `-key` is not set, dune-admin checks these paths in order:

1. `./sshKey`
2. `~/.dune-admin/sshKey`
3. `~/.ssh/dune`
4. `~/.ssh/id_ed25519`
5. `~/.ssh/id_rsa`

---

## Makefile targets

| Target | Description |
|--------|-------------|
| `make setup` | Run the interactive setup wizard |
| `make build` | Build the Go binary |
| `make linux` | Cross-compile a Linux amd64 binary |
| `make dev-server` | Run without building (`go run .`) |
| `make web` | Build the frontend only |
| `make deploy-web` | Build + deploy frontend to Cloudflare Pages |
| `make test` | Run tests |
| `make verify` | Run all quality checks (fmt, vet, tests, vuln, gosec) |

---

## Hosted frontend (optional)

The frontend can be deployed to Cloudflare Pages and pointed at a locally-running binary.

```bash
cd web && npm ci && npm run build
wrangler pages deploy dist --project-name dune-admin
```

On first load the app prompts for the backend URL (e.g. `http://localhost:8080`), saved in `localStorage`. The binary adds CORS headers automatically - no extra config needed.

> Modern browsers allow HTTPS pages to reach HTTP localhost without mixed-content errors, so `https://your-site.pages.dev` → `http://localhost:8080` works out of the box.

---

## Item data (optional)

`item-data.json` provides friendly item names, stack limits, volume, tier, and rarity. It ships with the repo but can be regenerated from game PAK files using [dune-item-data](https://github.com/Icehunter/dune-item-data).

Without it the panel still works - inventory items show raw template IDs.

---

## Tabs

| Tab | What it does |
|-----|-------------|
| **Players** | Browse players; view/edit inventory, specs, currency, XP, faction rep; journey nodes; teleport; history |
| **Battlegroup** | Start/stop the game server; stream logs; manage backups |
| **Database** | Run raw SQL against the game DB; browse tables |
| **Blueprints** | View all unlockable blueprint definitions |
| **Storage** | Browse server-side storage containers |
| **Logs** | Stream live logs; view cheat detection events |
