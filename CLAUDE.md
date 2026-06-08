# dune-admin — AI Assistant Rules

Web-based admin panel for a Dune Awakening private server. Go HTTP backend (`package main`)
paired with a React/TypeScript SPA in `web/`.

> **This repo is a fork.** `origin` → `Jenko-J1/dune-admin` (this fork), `upstream` →
> `Icehunter/dune-admin` (original). Open PRs against `origin/main`. Note that README install
> commands, the `scripts/install.sh` raw URL, and `docs/` (ADRs, plans) were authored upstream and
> still reference `Icehunter` — verify URLs/ownership before relying on them. To pull in upstream
> changes: `git fetch upstream && git merge upstream/main`.

## Core Principles

These four principles are non-negotiable and apply to **every** change and **every** surface
(existing admin UI and the planned player view). They are the lens for all work here.

1. **Security first, in all things.** Authorization is enforced server-side, never by the UI. Validate
   all input; parameterise all SQL; validate everything interpolated into shell/exec/paths; keep
   secrets out of logs/errors/responses; `make gosec` clean before push. ⚠️ The backend currently has
   **no authentication** — see `.claude/rules/security.md`. Treat every endpoint as unauthenticated
   until real auth lands. Security wins over convenience.
2. **Accessibility (WCAG 2.2 AA) applies to all surfaces.** Semantic HTML, full keyboard operability,
   visible focus, accessible names, sufficient contrast (via tokens), never color-only,
   `prefers-reduced-motion`. Build it in, don't defer it. See `.claude/rules/frontend.md`.
3. **Responsive design (mobile / tablet / desktop) on all surfaces.** Mobile-first; no fixed widths
   that overflow; tables and nav degrade gracefully; touch targets ≥ 44px. See
   `.claude/rules/frontend.md`.
4. **Comprehensive testing + lints, built as we progress.** Every change ships with tests; every bug
   fix lands a regression test; coverage ratchets up, not down. The frontend test/a11y harness is
   currently unwired — wire it up as we touch the UI. See `.claude/rules/testing.md` and
   `.claude/rules/testing-web.md`.

## Project Direction (this fork)

This fork is evolving dune-admin along four goals. Keep these in mind so changes move toward them:

1. **First-class Docker fleet management.** Progress: `dockerControl.GetStatus` now discovers game
   processes inside the container (`docker exec … ps`, reusing AMP's arg parser) so docker servers +
   maps/partitions appear and are administrable in Battlegroup; `DiscoverIniDir` now locates
   UserGame.ini layout-agnostically (configured `server_ini_dir` or `docker inspect` mount sources)
   so the Server Settings view works; both have tests in `control_docker_test.go`. Still outstanding
   for docker: server-settings **writes** reaching inside the container for non-bind setups (no
   `serverSettingsWriter`/overrides like AMP), scoped log/process listing, `StreamLog` input
   validation (gosec), `CaptureJWT` from live process args, and `update`/`backup` verbs.
2. **UI/UX rework to align with the `webui` sibling project** (`C:\Users\james\Documents\open-source\webui`
   — a TrueNAS React app: React 19 + Vite + Tailwind v4 + react-router route manifest + shadcn/Radix +
   ⌘K palette + mobile drawer + enforced a11y; design north star "consumer-grade polish — Tailscale,
   Vercel, Linear"). Shared ground (React 19 / Vite / Tailwind v4 / react-router v7 / lucide /
   CSS-var theming) makes alignment feasible; the big divergences are HeroUI v3 → shadcn/Radix, the
   tab-state `App.tsx` → a Shell + route manifest, and dune-admin's a11y/responsive maturity. **Keep
   the Dune thematic branding/fonts.** That repo is read-only reference — make no changes there.
3. **A separate player-facing view** (distinct from admin): players log in, link their character via
   an in-game verify code (delivered over the existing single-player whisper rail,
   `POST /api/v1/chat/whisper`), and view their own character stats (account-scoped, never by
   arbitrary id). Requires real backend auth + an admin/player role model first (see principle 1).
4. **Admin-toggleable player controls** for the player view (e.g. give items / give currency on/off),
   enforced **server-side** as a persisted permissions config — not a cosmetic UI toggle.

**Attribution requirement:** the UI must always retain attribution to the original creator,
**Icehunter**, and the upstream repo <https://github.com/Icehunter/dune-admin>. Do not remove it.

## Mandatory Workflow

**Follow these steps for EVERY code change. No exceptions.**

1. **Write tests FIRST** — Define expectations and error cases in tests BEFORE implementation
2. **Mock external dependencies** — Use interfaces for DB, executor, control plane
3. **Implement minimal code** — Write only what's needed to pass the tests
4. **Run verification** — `make verify` (must pass before done)

### TDD is Required

- ALWAYS write tests first. Never write implementation without tests.
- Tests define requirements. All error paths must be tested.
- Red-Green-Refactor: Write failing test → Make it pass → Refactor

See `.claude/rules/testing.md` for complete testing standards.

### Makefile Commands

**Always use `make` commands instead of raw `go` commands.**

```bash
make verify       # Run ALL checks — USE THIS BEFORE FINISHING
make test-race    # go test -race ./...  (used in CI)
make lint         # golangci-lint + markdownlint
make lint-go      # golangci-lint only
make fmt          # gofmt -s -w .
make fmt-check    # verify formatting (used in CI)
make gosec        # high-severity static security analysis
make vulncheck    # govulncheck dependency scan
make gocognit     # cognitive complexity gate (>15 flags)
make build        # compile → bin/dune-admin + ./dune-admin
make dev          # air (backend) + vite (frontend) in parallel
make dev-backend  # air hot-reload only
make dev-web      # cd web && pnpm dev
make setup        # interactive config wizard → ~/.dune-admin/config.yaml
make linux        # cross-compile for linux/amd64
```

Frontend commands (run from `web/`):

```bash
pnpm install      # install deps
pnpm dev          # Vite dev server :5173 → proxy :8080
pnpm build        # tsc -b && vite build → dist/
pnpm lint         # ESLint
pnpm preview      # preview production build
```

Versioning:

```bash
make version-patch  # bump x.y.Z, tag, push (triggers release workflow)
make version-minor  # bump x.Y.0, tag, push
make version-major  # bump X.0.0, tag, push
```

## Critical Gotchas

- **⚠️ No backend auth (yet)**: the SPA sends a Clerk `Bearer` token but the Go backend never verifies
  it — no auth middleware, no authorization, every endpoint open on the listen address. Frontend
  `isSignedIn` gates are cosmetic. `jwt_helpers.go` is game-broker token signing, not admin auth.
  Enforce anything security-sensitive server-side; don't add endpoints that assume a trusted caller.
  See `.claude/rules/security.md`.
- **Backend is one flat `package main`**: the entire HTTP backend lives in `cmd/dune-admin/` as
  `package main` — keep it flat, don't split the server into sub-packages. The ONE exception is
  genuinely reusable, standalone libraries, which go under `internal/` (today: `internal/marketbot`,
  the embedded market bot). This is a deliberate decision — see `docs/adr/0001-standard-go-layout.md`
  and `docs/adr/0002-embed-market-bot-as-library.md`. Default to `cmd/dune-admin/`; only reach for a
  new `internal/` package for a cohesive library with its own lifecycle.
- **No framework router**: uses Go 1.22+ stdlib pattern routing (`GET /api/v1/players/{id}`).
- **Guard globals**: always check `if globalDB == nil` before querying.
- **SQL in `db.go`**: all Postgres queries live there with the `dune.` schema prefix.
- **Journey cache**: `db.go` has a 30-second cache. Call `invalidateJourneyCache(accountID)` after
  player mutations; use `invalidateAllJourneyCache()` when only playerID is available.
- **DB writes need restart for some data**: backup procs and vehicle state require a game server
  restart. Don't expose as one-click actions without a restart flow.
- **Live game state lag**: DB writes aren't reflected until the player relogs (inventory) or the
  server restarts (storage/vehicles). This is a game cache issue, not a bug.
- **`display:none` for disabled UI**: use conditional rendering or `display:none` — don't remove
  state/code for features being temporarily hidden.
- **Returning-player NULL trick**: save originals before NULLing welcome-back timestamps. The login
  modal becomes sticky if originals aren't restored afterward.
- **Container/placeable names**: live on `dune.permission_actor.actor_name`. Strip `'None'` and
  `'##<Type>_Placeable'` defaults before displaying.
- **FLS item grants**: go via Funcom Live Services → PlayFab, not directly. `ServiceAuthToken` is
  the only credential.
- **pnpm required**: `web/` uses pnpm (pinned to `10.28.1`). Never use npm or yarn in `web/`.
- **No commits without permission**: make changes + run build/test, then stop for user review.
- **`make verify` does NOT run gosec**: run `make gosec` separately before any push that touches `exec.Command`, SQL, or file paths. The pre-push git hook gates on it. Suppress false positives with `// #nosec G204,G702 -- <reason>` (both IDs required). Never `git push --no-verify`.
- **Market bot — player orders are inviolable**: never delete, expire, or modify non-NPC exchange orders. Every `DELETE`/`UPDATE` on exchange tables must include `WHERE … AND is_npc_order = TRUE AND owner_id = <botID>`. Buy query uses SELECT filter for expired player orders — not DELETE.

## Modular Rules

Detailed standards in `.claude/rules/`:

| File | Applies To | Content |
| --- | --- | --- |
| `security.md` | `**/*` | Security-first: backend authz, parameterised SQL, exec/path validation, gosec, secrets, current no-auth gap |
| `testing.md` | `*_test.go` | TDD, mocking, coverage, regression-test-on-every-fix |
| `testing-web.md` | `web/**` | Frontend testing: Vitest + Testing Library + a11y assertions |
| `architecture.md` | `*.go` | Flat HTTP backend in `cmd/dune-admin`; libraries under `internal/`; handler/db/model patterns |
| `patterns.md` | `*.go` | DI, global state, cache invalidation, player-order safety |
| `error-handling.md` | `*.go` | Error wrapping, logging, HTTP status codes |
| `concurrency.md` | `*.go` | Goroutines, context, mutex |
| `api-design.md` | `handlers_*.go`, `server.go` | REST handlers, response helpers |
| `frontend.md` | `web/**` | Tab patterns, dune-ui, API client, WCAG accessibility, responsive design |
| `documentation.md` | `*.md` | Markdown standards |

Reusable skills in `.claude/skills/`:

| Skill | Trigger | What it does |
| --- | --- | --- |
| `tdd-go` | add handler / fix bug / implement feature | TDD checklist for Go handlers + db.go functions |
| `new-tab` | add tab / new tab / create tab | Full-stack scaffold: route → handler → db.go → api client → React tab |
| `pre-push-checklist` | ready to push / PR | make gosec + vulncheck + user approval gate |

---

## Project Structure

```
cmd/dune-admin/             — entire Go backend (package main, flat)
  main.go                   — config loading, flag parsing, startup
  server.go                 — HTTP mux, CORS middleware, jsonOK/jsonErr/decode
  connection.go             — globalDB, globalSSH, globalExecutor, globalControl
  executor.go               — Executor interface (local vs SSH)
  control.go                — ControlPlane interface
  control_docker.go / control_kubectl.go / control_local.go / control_amp.go
  executor_amp.go           — ampExecutor: localExecutor with sudo-elevated WriteFile
  db.go                     — all DB queries (pgx/v5); journey cache
  model.go                  — shared domain types (playerInfo, itemInfo, etc.)
  handlers_*.go             — one file per feature area (players, bases, logs, etc.)
  helpers.go                — shared utility functions
  security_test.go          — isReadOnlySQL, isValidK8sName, originAllowed
internal/marketbot/         — embedded market bot (the only non-main Go package; run in-process
                              via marketbot.Run from main.go, exposes its own HTTP API)
docs/
  adr/                      — architecture decision records (the "why" behind layout/embedding)
  plans/, superpowers/      — design plans and specs
  swagger.json / .yaml      — generated API spec (served at /swagger)
web/
  src/
    App.tsx                 — root component, tab routing, Clerk auth shell
    api/client.ts           — typed fetch wrapper (ApiError, req<T>, api.* namespaces)
    tabs/                   — one entry per top-level tab (file or directory)
    components/             — tab-local components (not globally shared)
    dune-ui/                — project component library (wraps HeroUI v3)
    hooks/                  — useStatus.ts, useTableSort.ts
    data/                   — static JSON lookups
```

---

## Go Backend Patterns

### Handler Structure

All handlers follow the same call-through pattern:

```go
func handleGetFoo(w http.ResponseWriter, r *http.Request) {
    if globalDB == nil {
        jsonErr(w, fmt.Errorf("database not connected"), http.StatusServiceUnavailable)
        return
    }
    result, err := cmdFetchFoo(r.Context(), globalDB, ...)
    if err != nil {
        log.Printf("handleGetFoo: %v", err)
        jsonErr(w, fmt.Errorf("internal error"), http.StatusInternalServerError)
        return
    }
    jsonOK(w, result)
}
```

- Query functions (`cmdFetch*`) live in `db.go`
- Use `jsonOK` / `jsonErr` from `server.go` — never write to `w` directly
- Pass `r.Context()` through to all DB calls

### Response Helpers (`server.go`)

```go
jsonOK(w, v)              // 200 + JSON-encoded v
jsonErr(w, err, code)     // code + {"error": err.Error()}
decode(r, &v)             // decode request body JSON into v
```

### Global State

| Global | Type | Purpose |
| --- | --- | --- |
| `globalDB` | `*pgxpool.Pool` | Postgres connection pool |
| `globalSSH` | `*ssh.Client` | SSH connection (nil when local) |
| `globalExecutor` | `Executor` | local or SSH executor |
| `globalControl` | `ControlPlane` | kubectl / docker / local / amp |

All globals set once in `connectAll()` (`connection.go`). Never reassign from handlers.

### SQL Queries

All Postgres queries live in `db.go`. Always use the `dune.` schema prefix. Use pgx v5
named parameters; scan results with `rows.Scan(...)`. Wrap errors — never panic.

### Security Constraints

- `isReadOnlySQL` — only SELECT/EXPLAIN/SHOW/WITH allowed on the admin SQL endpoint
- `isValidK8sName` — validates pod/namespace names before any shell/kubectl invocation
- CORS — strict allowlist via `ALLOWED_ORIGINS` env var; tests in `security_test.go`
- `gosec` runs in CI at `-severity high -confidence high`; G702 suppressed where known false positive

---

## Frontend Patterns

### Tab Components

Each tab is either a single `.tsx` file (simple) or a directory (complex):

```
tabs/FooTab.tsx             — simple tab
tabs/PlayersTab/
  index.tsx                 — root component
  types.ts                  — local types
  components/               — tab-local components
  modals/                   — modal components
  views/                    — sub-views (if needed)
```

**`BasesTab.tsx` is the canonical reference pattern** for new simple tabs.

Minimal tab structure:

```tsx
export default function FooTab() {
  const [data, setData] = useState<FooRow[]>([])
  const [loading, setLoading] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      setData(await api.foo.list())
    } catch (e) {
      toast.danger(`Failed: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])
  // ...
}
```

### API Client (`web/src/api/client.ts`)

All backend calls go through `req<T>(method, path, body?)`. Import the `api` namespace:

```ts
import { api, ApiError } from '../api/client'

const result = await api.bases.list()
```

Backend URL is runtime-configurable via `localStorage('dune_admin_backend')` (default `http://localhost:8080`).
Vite dev proxies `/api` and WebSocket `/api/v1/logs/stream` → `:8080`.

### Component Library (`dune-ui/`)

Import shared components from `../dune-ui` when a wrapper exists — not directly from `@heroui/react`:

```ts
import { DataTable, Icon, PageHeader, Panel, SectionDivider, SectionLabel,
         InfoCard, StatusChip, Dropzone, SideNav } from '../dune-ui'
import type { Column } from '../dune-ui'
```

Use `@heroui/react` directly only for primitives not wrapped in `dune-ui` (Button, Card, Spinner, toast).

### Theming

All colours are CSS custom properties in `web/src/index.css`. **Never use raw Tailwind colour utilities**
(`bg-amber-900`, `text-zinc-400`, etc.) — use semantic tokens:

- `bg-background`, `bg-surface`, `bg-surface-secondary`
- `text-foreground`, `text-muted`, `text-accent`
- `border-border`

Inline `style={{ color: '...' }}` overrides are a sign the semantic token approach wasn't used.

### Auth

`hasClerk = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY`. Absent key → app renders without auth
(local dev). The `isSignedIn` prop gates destructive features (Bases, Blueprints export).

---

## Configuration

Config loaded in order (first match per field wins):

1. `~/.dune-admin/config.yaml` — written by `make setup`
2. `.env` in working directory — legacy fallback
3. Environment variables
4. CLI flags

Key env vars: `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, `DB_NAME`, `DB_SCHEMA`,
`SSH_HOST`, `SSH_USER`, `SSH_KEY`, `CONTROL` (kubectl/docker/local/amp),
`LISTEN_ADDR` (default `:8080`), `ALLOWED_ORIGINS`.

---

## CI / Workflows

| Workflow | Trigger | What it does |
| --- | --- | --- |
| `test.yml` | push/PR → main | `go vet` + `go test -race` |
| `sast.yml` | push/PR → main | `make gosec` |
| `sca.yml` | push/PR → main | `pnpm audit --audit-level=high` |
| `deploy.yml` | push → main | Build frontend + Cloudflare Pages deploy |
| `release.yml` | push tag `v*` | GoReleaser (multi-platform) + frontend deploy |

---

## AMP Control Plane

The `amp` control plane targets CubeCoders AMP installations. Selected via `control: amp` in config.

### Topology

```
host (e.g. Ubuntu VM)
 └── AMP web panel (port 8080)
      └── podman container "AMP_<instance>"  (cubecoders/ampbase)
           ├── ampinstmgr (lifecycle)
           ├── RabbitMQ broker (admin + game vhosts)
           ├── Postgres
           └── 1..N DuneSandboxServer-Linux-Shipping processes (one per partition)
```

`dune-admin` runs **on the host**. Uses `localExecutor` for shell and `ampExecutor` to write INI files
as the AMP user.

### Config Keys

```yaml
control: amp
amp_instance:   DuneAwakening01
amp_container:  AMP_DuneAwakening01       # default: AMP_<instance>
amp_container_runtime: podman             # podman (default) | docker — game-server container CLI
amp_user:       amp
amp_log_path:   /AMP/duneawakening/logs   # in-container log dir
amp_api_user:   admin                     # AMP panel login — enables gameplay-settings writes via the AMP Web API
amp_api_pass:   yourpassword
amp_api_port:   8081                       # instance ADS API port (default 8081)
director_url:   http://127.0.0.1:11717    # optional — enables /director/ proxy
broker_exec_prefix: "sudo -i -u amp podman exec AMP_DuneAwakening01"
server_ini_dir: /home/amp/.ampdata/instances/DuneAwakening01/duneawakening/server/state
db_host: 127.0.0.1
db_port: 15432
```

### Sudoers

```
dune-admin ALL=(amp) NOPASSWD: /usr/bin/ampinstmgr, /usr/bin/podman, /usr/bin/tee
```

Use `/usr/bin/docker` instead of `/usr/bin/podman` when `amp_container_runtime: docker`. The
runtime-binary grant covers both `exec` (logs/broker) and `restart` (cycling the container to apply
server settings). Narrow `tee` to specific INI paths under `server_ini_dir` in production.

### Provider Behaviour

| Method | Implementation |
| --- | --- |
| `GetStatus` | Lists `DuneSandboxServer-Linux-Shipping` host processes; reports container DB phase |
| `ExecCommand` | start/stop: `ampinstmgr -s/-q <amp_instance>`. restart (container mode): `<runtime> restart <container>` — `ampinstmgr` does NOT reap the game procs; container restart is the only thing that cycles them. restart (native): `ampinstmgr -q && -s` |
| `writeServerSettings` | AMP Web API `Core/Login` + `Core/SetConfig` (node `Meta.GenericModule.<FieldName>`) via in-container curl; needs `amp_api_*`. Curated gameplay settings only |
| `ListProcesses` | Host `ps` for game-server processes, decorated with map/port/partition |
| `ListLogSources` | `<runtime> exec <container> ls <amp_log_path>` (runtime per `amp_container_runtime`) |
| `StreamLog` | `<runtime> exec <container> tail -F <amp_log_path>/<name>` |
| `CaptureJWT` | Extracts `ServiceAuthToken` from game-server process args on host |
| `ListExchanges` / `EnsureCaptureUser` | `rabbitmqctl` via `broker_exec_prefix` |
| `DiscoverIniDir` | Returns `server_ini_dir` (or derives conventional AMP path) |
| `ReadDefaultINI` | `<runtime> exec <container> find / -name <file>` then `cat` |

**Server settings under AMP go through the AMP Web API, not INI writes.** AMP regenerates
`UserEngine.ini` / `UserGame.ini` from its own config on every start, so a direct file edit is
clobbered. The curated gameplay schema (`serverSettingsSchema` — real CVars + `/Script` UPROPERTYs,
keyed by `FieldName`) is written via `ampControl.writeServerSettings` → AMP `Core/SetConfig`. Non-AMP
planes (docker/kubectl/local) and raw-INI-section edits still write files directly via
`ampExecutor.WriteFile` (`sudo -i -u <amp_user> tee <path> > /dev/null`). Either way, settings only
take effect after a game restart via `ExecCommand("restart")`.

`ampControl.startEnsureCaptureUserLoop` re-applies the `dune_cap` user+permissions every 15s
so capture survives broker restarts without manual intervention.

---

## Code Review Checklist

- [ ] Tests written FIRST (TDD)
- [ ] All error paths tested
- [ ] External dependencies mocked (DB, executor, control plane)
- [ ] Tests pass with race detector (`make test-race`)
- [ ] Regression test added for any bug fix (fails before, passes after)
- [ ] Server (`cmd/dune-admin/`) kept flat — no sub-packages; any new `internal/` library is justified with an ADR
- [ ] SQL lives in `db.go`, uses `dune.` schema prefix, parameterised (no string-built queries)
- [ ] Global state guarded (`if globalDB == nil`)
- [ ] Journey cache invalidated after mutations
- [ ] Security: authz enforced server-side; exec/path/SQL input validated; no secrets leaked; `make gosec` clean
- [ ] Frontend (if touched): WCAG 2.2 AA, responsive at mobile/tablet/desktop
- [ ] `make verify` passes
