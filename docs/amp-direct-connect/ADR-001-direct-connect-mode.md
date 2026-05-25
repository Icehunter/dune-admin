# ADR-001: Direct-connect mode

**Status:** accepted
**Date:** 2026-05-23

## Context

Upstream `dune-admin` reaches the game-server's PostgreSQL by:

1. Opening an SSH session to the Battlegroup VM.
2. Running `kubectl` over SSH to discover the database pod's cluster IP.
3. Dialing PostgreSQL through an SSH-tunnelled TCP connection.

This is correct for a real Funcom-style k3s deployment, but it imposes
real costs on operators who are running the game server in a container on
their own host (typical AMP setup):

- They have to expose SSH on the VM and manage a key just so a tool that
  could otherwise talk to `127.0.0.1:15432` directly can reach the DB.
- The SSH user needs `sudo kubectl` permission, which is a meaningful
  privilege handoff.
- The kubectl path is brittle against the AMP "mock-k3s-go" shim — it
  isn't a real Kubernetes API and not every kubectl verb behaves.
- Latency on every operation is dominated by SSH round-trips, even though
  the data is on the same host as the binary.

## Decision

Introduce a top-level connection-mode toggle (`CONNECTION_MODE=direct` or
`ssh`) that gates *every* operation requiring host access. In `direct`
mode:

- Database connections go straight to TCP via configured host/port/credentials.
- No SSH client is created; no SSH key is required.
- Handlers that fundamentally need SSH (cluster operations, backup file
  download via SCP, etc.) return **HTTP 501 Not Implemented** with a clear
  message rather than failing in confusing ways.
- A small set of replacement handlers (covered in ADR-002 / 003 / 005)
  provide the same *capabilities* against local resources.

The toggle is plumbed via:

- An environment variable (`CONNECTION_MODE`) for normal operation.
- A CLI flag (`-mode direct|ssh`) for overrides during development.
- A dedicated branch in the interactive `-setup` wizard so first-time
  configuration doesn't ask SSH questions when they don't apply.

The SSH path remains the default-compatible path for upstream users. The
toggle is the only thing dispatching the two code paths.

## Why this and not X

- **Why not delete the SSH path entirely?** Upstream's audience runs real
  Funcom topologies. Removing SSH would fork the project further than is
  warranted. The toggle keeps both audiences served from one binary.
- **Why not auto-detect?** Auto-detection ("is there an SSH key in the
  current dir?") guesses wrong in too many environments. An explicit
  toggle is observable and reproducible.
- **Why a single global toggle instead of per-handler config?** Mode
  selection affects connection setup, not just request handling — DB
  pool creation, optional SSH client, capture-tool wiring, even the
  setup wizard all branch on it. A single value keeps the branches in
  lockstep.

## Preserve when re-implementing

These invariants must hold on any rewrite:

1. **`connectionMode` is set exactly once at startup** and never changes
   for the life of the process. Mid-flight mode flips would invalidate
   the connection pool's assumptions.
2. **Every SSH-dependent handler must check the mode first** and return
   501 (not 500, not 503) in direct mode. The `requireSSH(w)` helper
   exists for this; new handlers MUST use it.
3. **The `/api/v1/status` endpoint must expose `connection_mode`** so the
   frontend can render mode-appropriate UI and so external monitors can
   tell which deployment they're talking to.
4. **The setup wizard must not require SSH credentials in direct mode.**
   Anything else makes the wizard worse than editing `.env` by hand.
5. **Default to `direct` mode when `CONNECTION_MODE` is unset** in the
   AMP-targeted build. The k3s topology is the minority case for this
   fork's intended audience.

## Implementation-incidental

These details are tied to the current file layout and are safe to change:

- The placement of `requireSSH` in `server.go`. A new layout might put it
  in a middleware package; the function shape (returns `bool`, writes the
  error response) is what matters.
- Whether `connectionMode` is a package-level `string` or a typed enum —
  a typed enum would actually be a small improvement.
- The exact wording of the 501 error body.
- Whether the CLI flag is `-mode` or `--connection-mode`.
- Where the direct-mode setup branch lives in `setup.go` (a separate
  `runSetupDirect` function is just convenience).

## Related

- [ADR-002](ADR-002-amp-battlegroup-adapter.md) — battlegroup discovery in
  direct mode.
- [ADR-003](ADR-003-websocket-log-streaming.md) — log streaming in direct
  mode.
- [ADR-005](ADR-005-rabbitmq-capture-mode.md) — capture tool in direct mode.
