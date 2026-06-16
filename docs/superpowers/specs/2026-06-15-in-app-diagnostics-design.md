# In-app Diagnostics — Design Spec

**Date:** 2026-06-15
**Status:** Approved (design), pending implementation plan
**Scope:** Sub-project B of the docs/diagnostics initiative. Sub-project A (documentation
platform) is designed separately.

## Summary

Add a dune-admin self-observability feature: an owner-only **Diagnostics** tab that streams
dune-admin's own application logs live, shows an environment summary, and offers a **Report an
issue** action that opens a prefilled GitHub issue and downloads a redacted diagnostics bundle for
the user to attach.

Today dune-admin's own logs (zerolog) go only to stderr with no in-memory capture. The existing
log streaming (`handleLogStream` / `LogsTab`) targets *game server* logs via the control plane, not
dune-admin itself. This feature adds self-log capture and surfaces it, reusing the existing stream
and tab patterns.

## Goals

- Operators can see dune-admin's own recent logs in the UI without shell access to the host.
- One-click path from "something is wrong" to a well-formed GitHub issue with the context
  maintainers need to triage (version, control plane, provider, recent logs).
- Nothing sensitive leaves the machine without a human review gate and hard redaction.

## Non-goals

- No persistent log storage, rotation, or shipping to external aggregators (stderr already covers
  that for operators who want it).
- No automated/tokened posting to GitHub. The user always reviews and submits.
- No config.yaml or secrets in any exported artifact.
- No changes to the existing game-server `LogsTab` / `logs:read` flow.

## Key decisions

1. **Prefilled issue link, not auto-post.** Backend returns a redacted title/body; the frontend
   opens `github.com/Icehunter/dune-admin/issues/new?title=...&body=...`. The user reviews and
   submits. No GitHub token in dune-admin.
2. **Hard redaction by allowlist, fail closed.** The environment summary emits only explicitly
   allowlisted fields. Log lines are masked by an ordered set of rules. Unknown/unmatched
   sensitive-looking content is masked or omitted, never passed through by default.
3. **Raw live view, redacted export.** The owner-only live viewer streams *raw* logs so debugging
   is actually useful. Redaction applies only to artifacts that leave the machine (the issue body
   and the downloadable bundle).
4. **Owner-gated.** A new backend capability `capDiagnosticsRead` (`"diagnostics:read"`) gates all
   diagnostics endpoints; the frontend tab uses the `'owner'` pseudo-capability so only owners see
   it. This mirrors the existing Permissions tab (frontend `'owner'` + a real backend cap). Owners
   bypass the capability matrix (`auth_middleware.go:184`), so they are always allowed; the named
   cap keeps the route self-describing in the Permissions matrix and leaves room to grant it later.
5. **Bundle for manual attach.** GitHub issue URLs cap around 8KB, so the issue body carries the
   environment summary plus the last ~50 redacted log lines; the full redacted log + environment
   ship as a downloaded `diagnostics.zip` the user attaches.

## Architecture

Five units, each independently testable.

### 1. Log capture — ring buffer sink (`logging.go`)

- Introduce a `logRing` type: a fixed-capacity, mutex-guarded circular buffer of structured
  entries. Each entry retains `level`, `timestamp`, `component`, `server_id` (when present), and
  `message`.
- Wire it as a second zerolog sink via `zerolog.MultiLevelWriter(stderrWriter, ring)` in
  `initLogging()`. Every entry already written to stderr is also captured.
- The ring never blocks logging: on overflow it drops the oldest entry. Writes are O(1).
- Capacity is configurable via env `DIAG_LOG_BUFFER` (default **2000** entries). Precedent:
  ADR-0005 uses a ring buffer for embedded-bot log streaming.
- Expose read access: `ring.Snapshot() []logEntry` (copy under lock) and a subscription channel for
  live tailing (`ring.Subscribe() (<-chan logEntry, func())`), mirroring the control-plane
  `StreamLog` contract of `(<-chan T, cancel, error)`.

### 2. Redaction — security-critical unit (`redact.go`)

Pure functions, no I/O, heaviest test coverage.

- `redactLine(s string) string` applies an ordered rule set masking:
  - IPv4/IPv6 addresses and `host:port` pairs
  - bearer/authorization tokens, `ServiceAuthToken`, generic `key=`/`token=`/`password=` values
  - SSH targets (`user@host`)
  - filesystem paths containing a home directory / username
  - FLS IDs and numeric account/player IDs
- `buildEnvironment() environmentSummary` returns an **allowlist-only** struct:
  - `version`, `commit`, `os`, `arch`, `go_version`
  - `control_plane` (amp/kubectl/docker/local), `provider`
  - `auth_enabled` (bool), `market_bot_enabled` (bool)
  - `active_server_count` (int)
  - Nothing else. New fields require an explicit code change to the allowlist.
- Redaction is applied centrally so every export path (issue body + bundle) shares one
  implementation. The live stream path does **not** call it (raw, owner-only).

### 3. API — `handlers_diagnostics.go` (all gated by `capDiagnosticsRead`)

Add `capDiagnosticsRead capability = "diagnostics:read"` to `auth_capabilities.go` (constant +
`allCapabilities` description). Routes registered in `server.go` via
`handleAPI(..., capDiagnosticsRead, ...)`. Owners are always allowed (matrix bypass); non-owners
need the cap granted.

- `GET /api/v1/diagnostics/environment` → redacted `environmentSummary` JSON (summary card).
- `GET /api/v1/diagnostics/logs/stream` → WebSocket. Replays `ring.Snapshot()` then tails live via
  `ring.Subscribe()`. **Raw.** Reuses the `handleLogStream` WebSocket pattern (upgrade, write
  TextMessage per line, close on context cancel / write error).
- `GET /api/v1/diagnostics/report` → JSON `{ "title": string, "body": string }`. Body is redacted
  markdown: environment block + last ~50 redacted log lines, trimmed to stay within the GitHub
  issue-URL budget.
- `GET /api/v1/diagnostics/bundle` → `application/zip` containing redacted `app.log` (full ring
  snapshot) and `environment.txt`. `Content-Disposition: attachment; filename="diagnostics.zip"`.

The upstream repo slug (`Icehunter/dune-admin`) is a backend constant so the issue URL is fixed
regardless of which fork is running.

### 4. Frontend — `DiagnosticsTab.tsx`

- Registered in `web/src/components/app/nav.ts`: new `diagnostics` tab id, Lucide icon,
  `TAB_CAPABILITIES['diagnostics'] = 'owner'` (tab visible to owners only). Lazy-loaded in
  `AppRoutes.tsx`.
- New `api.diagnostics` namespace in `web/src/api/client.ts`: `environment()`, `report()`, and the
  WebSocket URL via `getWsBase()`.
- Reuses LogsTab's streaming machinery: line ref + ~200ms flush to state, autoscroll toggle, export
  to `.txt`. Adds a **level filter**, **pause**, and **clear**.
- Environment summary card at the top.
- **Report an issue** button: calls `api.diagnostics.report()`, opens
  `https://github.com/Icehunter/dune-admin/issues/new` with `title`/`body` query params in a new
  tab (`noopener,noreferrer`), and triggers the `diagnostics/bundle` download for manual attach.

### 5. Config

- `DIAG_LOG_BUFFER` env var (default 2000) for ring capacity.
- No new config.yaml keys. No GitHub token. Upstream repo slug is a constant.

## Data flow

```
zerolog event
  ├─> stderr (unchanged)
  └─> logRing (capture, drop-oldest on overflow)

Live viewer:   WS connect → ring.Snapshot() replay → ring.Subscribe() tail → raw lines to UI
Report issue:  report endpoint → ring.Snapshot() → redactLine x N + buildEnvironment()
                 → { title, body }  ──> frontend opens prefilled issues/new
                 → bundle endpoint → redacted zip ──> browser download → user attaches
                 → USER reviews and submits on GitHub
```

## Error handling

- Ring writes never block or error the logging path; overflow silently drops oldest.
- WebSocket closes cleanly on context cancellation or write failure (matches `handleLogStream`).
- All four endpoints fail closed: a caller lacking `capDiagnosticsRead` (and not an owner) → 403 via
  the capability middleware.
- `buildEnvironment()` omits any field it cannot resolve rather than emitting a placeholder that
  might leak; redaction defaults to masking on ambiguity.
- Report body is hard-capped in length; if logs exceed the budget they are truncated with an
  explicit `... (truncated, see attached bundle)` marker.

## Testing (TDD, per CLAUDE.md)

Tests written first, external deps mocked, race detector clean.

- **`logRing`**: append under capacity, overflow drops oldest in order, concurrent
  writers/readers (`-race`), snapshot is a copy (no aliasing), subscribe delivers live entries and
  cancel stops delivery.
- **`redact.go`** (highest coverage): each masking rule (IP, host:port, token, SSH target, home
  path, FLS/account id) with positive and negative cases; `buildEnvironment` emits only allowlisted
  fields; a struct with an injected sensitive field is **not** leaked (fail-closed assertion).
- **report assembly**: body stays within the URL budget; truncation marker present when oversized;
  bundle contains exactly `app.log` + `environment.txt` and no secret patterns (assert redaction
  ran).
- **handlers**: `capDiagnosticsRead` enforced (403 for a caller without it / non-owner; owner
  allowed) on all four routes; `environment` returns the expected shape; stream upgrade rejects
  non-WebSocket requests.

## Affected files

New:

- `cmd/dune-admin/redact.go` + `redact_test.go`
- `cmd/dune-admin/handlers_diagnostics.go` + `handlers_diagnostics_test.go`
- `web/src/tabs/DiagnosticsTab.tsx`

Modified:

- `cmd/dune-admin/logging.go` (ring sink + `logRing` type, or a sibling `logring.go` with tests)
- `cmd/dune-admin/server.go` (register four routes under `capDiagnosticsRead`)
- `cmd/dune-admin/auth_capabilities.go` (add `capDiagnosticsRead` constant + description)
- `web/src/api/client.ts` (`api.diagnostics` namespace + types)
- `web/src/components/app/nav.ts` (tab id, icon, capability)
- `web/src/components/app/AppRoutes.tsx` (lazy route)

## Open questions

None blocking. Capacity default (2000), the raw-live/redacted-export split, and the
`capDiagnosticsRead` + frontend `'owner'` gating are decided.
