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

Versioning: `make version-patch / version-minor / version-major` — bumps, tags, pushes.

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
- **`make verify` does NOT run gosec**: run `make gosec` separately before any push that touches `exec.Command`, SQL, or file paths. The pre-push git hook gates on it. Suppress false positives with `// #nosec G204,G702 -- <reason>` (both IDs required). Never `git push --no-verify`.
- **Market bot — player orders are inviolable**: never delete, expire, or modify non-NPC exchange orders. Every `DELETE`/`UPDATE` on exchange tables must include `WHERE … AND is_npc_order = TRUE AND owner_id = <botID>`. Buy query uses SELECT filter for expired player orders — not DELETE.

## Rules & Skills

Detailed standards in `.claude/rules/`:

| File | Content |
| --- | --- |
| `testing.md` | TDD, mocking, coverage |
| `architecture.md` | Flat package, handler/db/model patterns |
| `patterns.md` | DI, global state, cache invalidation, player-order safety |
| `error-handling.md` | Error wrapping, logging, HTTP status codes |
| `concurrency.md` | Goroutines, context, mutex |
| `api-design.md` | REST handlers, response helpers |
| `frontend.md` | Tab patterns, dune-ui, API client, theming |
| `amp.md` | AMP control plane topology, config keys, provider methods |
| `documentation.md` | Markdown standards |

Reusable skills in `.claude/skills/`:

| Skill | Trigger |
| --- | --- |
| `tdd-go` | add handler / fix bug / implement feature |
| `new-tab` | add tab / new tab / create tab |
| `pre-push-checklist` | ready to push / PR |

## CI / Workflows

| Workflow | Trigger | What it does |
| --- | --- | --- |
| `test.yml` | push/PR → main | `go vet` + `go test -race` |
| `sast.yml` | push/PR → main | `make gosec` |
| `sca.yml` | push/PR → main | `pnpm audit --audit-level=high` |
| `deploy.yml` | push → main | Build frontend + Cloudflare Pages deploy |
| `release.yml` | push tag `v*` | GoReleaser (multi-platform) + frontend deploy |

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
