# dune-admin — AI Assistant Rules

Web-based admin panel for a Dune Awakening private server. Go HTTP backend (`package main`)
paired with a React/TypeScript SPA in `web/`.

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

- **Single Go package**: everything is `package main` in `cmd/dune-admin/`. Never create sub-packages.
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

## Modular Rules

Detailed standards in `.claude/rules/`:

| File | Applies To | Content |
| --- | --- | --- |
| `testing.md` | `*_test.go` | TDD, mocking, coverage |
| `architecture.md` | `*.go` | Flat package, handler/db/model patterns |
| `patterns.md` | `*.go` | DI, global state, cache invalidation |
| `error-handling.md` | `*.go` | Error wrapping, logging, HTTP status codes |
| `concurrency.md` | `*.go` | Goroutines, context, mutex |
| `api-design.md` | `handlers_*.go`, `server.go` | REST handlers, response helpers |
| `frontend.md` | `web/**` | Tab patterns, dune-ui, API client |
| `documentation.md` | `*.md` | Markdown standards |

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
amp_user:       amp
amp_log_path:   /AMP/duneawakening/logs   # in-container log dir
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

Narrow `tee` to specific INI paths under `server_ini_dir` in production.

### Provider Behaviour

| Method | Implementation |
| --- | --- |
| `GetStatus` | Lists `DuneSandboxServer-Linux-Shipping` host processes; reports container DB phase |
| `ExecCommand` | `sudo -i -u <amp_user> ampinstmgr -s/-q <amp_instance>` |
| `ListProcesses` | Host `ps` for game-server processes, decorated with map/port/partition |
| `ListLogSources` | `podman exec <container> ls <amp_log_path>` |
| `StreamLog` | `podman exec <container> tail -F <amp_log_path>/<name>` |
| `CaptureJWT` | Extracts `ServiceAuthToken` from game-server process args on host |
| `ListExchanges` / `EnsureCaptureUser` | `rabbitmqctl` via `broker_exec_prefix` |
| `DiscoverIniDir` | Returns `server_ini_dir` (or derives conventional AMP path) |
| `ReadDefaultINI` | `podman exec <container> find / -name <file>` then `cat` |

`ampExecutor.WriteFile` pipes content through `sudo -i -u <amp_user> tee <path> > /dev/null`.
Changes require a Dune instance restart via `ExecCommand("restart")`.

`ampControl.startEnsureCaptureUserLoop` re-applies the `dune_cap` user+permissions every 15s
so capture survives broker restarts without manual intervention.

---

## Code Review Checklist

- [ ] Tests written FIRST (TDD)
- [ ] All error paths tested
- [ ] External dependencies mocked (DB, executor, control plane)
- [ ] Tests pass with race detector (`make test-race`)
- [ ] No new sub-packages created
- [ ] SQL lives in `db.go`, uses `dune.` schema prefix
- [ ] Global state guarded (`if globalDB == nil`)
- [ ] Journey cache invalidated after mutations
- [ ] `make verify` passes
