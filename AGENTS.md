# AGENTS.md — dune-admin

Web-based admin panel for a Dune Awakening private server. The repo is a Go HTTP backend (`package main`, all in root) paired with a React/TypeScript SPA in `web/`.

---

## Essential Commands

### Backend (Go)

```bash
make build          # compile → bin/dune-admin + ./dune-admin
make dev-server     # go run . (hot-reload via external watcher if needed)
make setup          # interactive config wizard → ~/.dune-admin/config.yaml
make test           # go test ./...
make test-race      # go test -race ./...  (used in CI)
make vet            # go vet ./...
make fmt            # gofmt -s -w .
make fmt-check      # verify formatting (used in CI)
make gosec          # high-severity static security analysis
make vulncheck      # govulncheck dependency scan
make verify         # fmt-check + vet + test-race + vulncheck + gosec (full pre-merge gate)
make linux          # cross-compile for linux/amd64
```

### Frontend (web/)

```bash
cd web
pnpm install        # install deps
pnpm dev            # Vite dev server on :5173 with proxy to :8080
pnpm build          # tsc -b && vite build → dist/
pnpm lint           # ESLint
pnpm preview        # preview production build
```

The root `Makefile` also exposes:
```bash
make web            # cd web && npm ci && npm run build
make deploy-web     # build + wrangler pages deploy
```

### Versioning

```bash
make version-patch  # bump x.y.Z, tag, push (triggers release workflow)
make version-minor  # bump x.Y.0, tag, push
make version-major  # bump X.0.0, tag, push
```

---

## Project Structure

```
/                   — Go backend (package main, single package)
  main.go           — config loading, flag parsing, startup
  server.go         — HTTP mux, CORS middleware, JSON helpers (jsonOK/jsonErr/decode)
  connection.go     — global state: globalDB, globalSSH, globalExecutor, globalControl
  executor.go       — Executor interface (local vs SSH)
  control.go        — ControlPlane interface
  control_docker.go / control_kubectl.go / control_local.go / control_amp.go
  executor_amp.go   — ampExecutor: localExecutor with sudo-elevated WriteFile
  db.go             — all DB queries (pgx/v5); journey cache
  model.go          — shared domain types (playerInfo, itemInfo, etc.)
  handlers_*.go     — one file per feature area (players, bases, logs, etc.)
  helpers.go        — shared utility functions
  security_test.go  — tests for isReadOnlySQL, isValidK8sName, originAllowed
  web/              — React SPA
    src/
      App.tsx       — root component, tab routing, Clerk auth shell
      api/client.ts — typed fetch wrapper (ApiError, req<T>, api.* namespaces)
      tabs/         — one entry per top-level tab (may be a file or a directory)
      components/   — tab-local components (not globally shared)
      dune-ui/      — project component library (wraps HeroUI v3)
      hooks/        — useStatus.ts, useTableSort.ts
      data/         — static JSON lookups
```

---

## Go Backend Patterns

### Handler structure

All handlers follow the same call-through pattern:

```go
func handleGetFoo(w http.ResponseWriter, r *http.Request) {
    msg, ok := cmdFetchFoo().(msgFoo)
    if !ok {
        jsonErr(w, fmt.Errorf("internal error"), 500)
        return
    }
    if msg.err != nil {
        jsonErr(w, msg.err, 500)
        return
    }
    jsonOK(w, msg.rows)
}
```

- `cmdFetchFoo()` lives in `db.go` and returns a `Msg` interface value.
- Cast with type assertion; always check `ok`.
- Use `jsonOK` / `jsonErr` from `server.go` — never write to `w` directly.

### Response helpers (server.go)

```go
jsonOK(w, v)              // 200 + JSON-encoded v
jsonErr(w, err, code)     // code + {"error": err.Error()}
decode(r, &v)             // decode request body JSON into v
```

### Global state

- `globalDB` — `*pgxpool.Pool` (pgx v5)
- `globalSSH` — `*ssh.Client` (nil when running locally)
- `globalExecutor` — `Executor` interface (local or ssh)
- `globalControl` — `ControlPlane` interface (kubectl/docker/local)

All globals are set in `connectAll()` (`connection.go`). Handlers must guard `if globalDB == nil` before querying.

### SQL queries

Use named parameters with pgx v5. Query in `db.go`, always use the `dune.` schema prefix. SQL results are scanned with `rows.Scan(...)`. Return errors wrapped in `msg` structs, not panics.

### Security constraints

- `isReadOnlySQL` guards the admin SQL endpoint — only SELECT/EXPLAIN/SHOW/WITH allowed.
- `isValidK8sName` validates pod/namespace names to prevent shell injection.
- CORS is strict: only allowlisted origins (env `ALLOWED_ORIGINS`). Tests live in `security_test.go`.
- `gosec` runs in CI at `-severity high -confidence high`. G702 is suppressed with `//nolint` where it's a known false positive.

---

## Frontend Patterns

### API client (`web/src/api/client.ts`)

All backend calls go through `req<T>(method, path, body?)`. Import `api` namespace for typed wrappers:

```ts
import { api, ApiError } from '../api/client'

const result = await api.bases.list()
```

The backend URL is configurable at runtime via `localStorage('dune_admin_backend')`. Default: `http://localhost:8080`.

Vite dev server proxies `/api` → `:8080` and `/api/v1/logs/stream` (WebSocket) → `:8080` — no CORS issues in dev.

### Tab components

Each tab is either:
- A single `.tsx` file (e.g. `BasesTab.tsx`) for simpler tabs
- A directory with `index.tsx`, `types.ts`, `modals/`, `components/`, and sometimes `views/` for complex tabs (e.g. `PlayersTab/`, `BattlegroupTab/`)

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

### Component library (`dune-ui/`)

Import shared components from `../dune-ui`, not directly from `@heroui/react`, when a wrapper exists:

```ts
import { DataTable, Icon, PageHeader, Panel, SectionDivider, SectionLabel,
         InfoCard, StatusChip, Dropzone, SideNav } from '../dune-ui'
import type { Column } from '../dune-ui'
```

Use `@heroui/react` directly for primitives not wrapped in `dune-ui` (Button, Card, Spinner, toast, etc.).

### Theming

The design uses a Dune desert dark theme. All colors are defined as CSS custom properties in `web/src/index.css`, overriding HeroUI v3 semantic tokens. Use the semantic utilities:

- `bg-background`, `bg-surface`, `bg-surface-secondary`
- `text-foreground`, `text-muted`, `text-accent`
- `border-border`

Never use raw Tailwind color utilities (e.g. `bg-amber-900`) — use the semantic tokens so the palette stays consistent. Inline `style={{}}` overrides for colors are a sign that the semantic token approach wasn't used.

### Auth

Clerk is optional. `hasClerk = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY`. When the key is absent the app renders without auth (useful for local dev). The `isSignedIn` prop gates destructive features in certain tabs (Bases, Blueprints export).

---

## Configuration

Config is loaded in order (first match wins per field):
1. `~/.dune-admin/config.yaml` — written by `make setup`
2. `.env` in working directory — legacy fallback
3. Environment variables
4. CLI flags

Key env vars: `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, `DB_NAME`, `DB_SCHEMA`, `SSH_HOST`, `SSH_USER`, `SSH_KEY`, `CONTROL` (kubectl/docker/local), `LISTEN_ADDR` (default `:8080`), `ALLOWED_ORIGINS`.

---

## CI / Workflows

| Workflow | Trigger | What it does |
|----------|---------|-------------|
| `test.yml` | push/PR → main | `go vet` + `go test -race` |
| `sast.yml` | push/PR → main | `make gosec` |
| `sca.yml` | push/PR → main | `pnpm audit --audit-level=high` |
| `deploy.yml` | push → main | Build frontend + Cloudflare Pages deploy |
| `release.yml` | push tag `v*` | GoReleaser (multi-platform) + frontend deploy |

Release is triggered by `make version-patch/minor/major` which bumps `VERSION`, tags, and pushes.

---

## Gotchas

- **Single Go package**: everything is `package main`. There are no sub-packages. Don't create sub-packages.
- **No framework router**: uses Go 1.22+ stdlib pattern routing (`GET /api/v1/players/{id}`).
- **Live game state lag**: DB writes aren't reflected in the running game server until the player relogs (inventory) or the server restarts (storage/vehicles). This is a game cache issue, not a bug.
- **Journey cache**: `db.go` has a 30-second in-memory cache for journey nodes. Call `invalidateJourneyCache(accountID)` after mutations. For player-ID-keyed mutations without account ID, use `invalidateAllJourneyCache()`.
- **DB writes need restart for some data**: backup procs and some vehicle state require a game server restart to take effect. Don't expose these as one-click actions without a restart flow.
- **`display:none` for disabled UI**: when disabling a feature in the frontend, use `display:none` (or conditional rendering with the component still mounted if state must persist). Don't remove state or code for features being temporarily hidden.
- **Returning-player NULL trick**: saving originals before NULLing welcome-back timestamps is critical. The login modal becomes sticky if originals aren't restored afterward.
- **Container/placeable names**: custom placeable names live on `dune.permission_actor.actor_name`. Strip `'None'` and `'##<Type>_Placeable'` defaults before displaying.
- **FLS item grants**: item grants go via Funcom Live Services → PlayFab, not directly. `ServiceAuthToken` is the only credential.
- **pnpm required**: `web/` uses `pnpm` (pinned to `10.28.1`). Don't use `npm` or `yarn` in `web/`.
- **No commits without permission**: never commit code changes; make changes + run build/test, then stop for user review.

---

## AMP control plane

The `amp` control plane targets CubeCoders AMP installations where the Dune game server runs inside a podman container managed by `ampinstmgr`. It is a sibling of `kubectl`/`docker`/`local` — selected via `control: amp` in `~/.dune-admin/config.yaml` or the `-control amp` flag.

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

`dune-admin` runs **on the host**, not inside the container. It uses `localExecutor` for shell commands and an `ampExecutor` wrapper to write INI files as the AMP user.

### Config keys

```yaml
control: amp
amp_instance:  DuneAwakening01           # ampinstmgr instance name
amp_container: AMP_DuneAwakening01       # podman container (default: AMP_<instance>)
amp_user:      amp                       # OS user that owns AMP (default: amp)
amp_log_path:  /AMP/duneawakening/logs   # in-container log dir
director_url:  http://127.0.0.1:11717    # optional — enables /director/ proxy
broker_exec_prefix: "sudo -i -u amp podman exec AMP_DuneAwakening01"
server_ini_dir: /home/amp/.ampdata/instances/DuneAwakening01/duneawakening/server/state
db_host: 127.0.0.1
db_port: 15432
```

### Sudoers grants

`dune-admin` typically runs as a non-AMP user (e.g. `dune-admin`). The following grants make AMP-side operations work without prompts:

```
dune-admin ALL=(amp) NOPASSWD: /usr/bin/ampinstmgr, /usr/bin/podman, /usr/bin/tee
```

Narrow this further in production (e.g. lock `tee` to specific INI paths under `server_ini_dir`).

### Provider behaviour

| Method | Implementation |
|---|---|
| `GetStatus` | Lists `DuneSandboxServer-Linux-Shipping` host processes; reports container DB phase. |
| `ExecCommand` | `sudo -i -u <amp_user> ampinstmgr -s/-q <amp_instance>`. |
| `ListProcesses` | Host `ps` for game-server processes, decorated with map/port/partition. |
| `ListLogSources` | `podman exec <container> ls <amp_log_path>`. |
| `StreamLog` | `podman exec <container> tail -F <amp_log_path>/<name>`. |
| `CaptureJWT` | Extracts `ServiceAuthToken` from game-server process args on the host. |
| `ListExchanges` / `EnsureCaptureUser` | `rabbitmqctl` via `broker_exec_prefix`. |
| `DiscoverIniDir` | Returns `server_ini_dir` (or derives the conventional AMP path). |
| `ReadDefaultINI` | `podman exec <container> find / -name <file>` then `cat`. |

### Writing UserGame.ini

The AMP user owns `UserGame.ini`/`UserEngine.ini`. `ampExecutor.WriteFile` pipes content through `sudo -i -u <amp_user> tee <path> > /dev/null`. Writes follow the same upstream `patchINI` semantics as the other providers — no delimiter blocks, no AMP-marker handling. Changes require a Dune instance restart via `ExecCommand("restart")` to take effect.

### Capture mode self-heal

AMP can restart the broker container without warning, which resets the in-memory `dune_cap` user. `ampControl.startEnsureCaptureUserLoop` re-applies the user+permissions every 15s so capture survives broker restarts without manual intervention. Other providers don't need this; their brokers are externally managed.
