# ADR-005: RabbitMQ capture mode for direct setups

**Status:** accepted
**Date:** 2026-05-23

## Context

`dune-admin` ships with a `-capture` flag that connects to the game's
internal RabbitMQ brokers (an "admin" broker and a "game" broker), binds
queues to every relevant exchange, and prints every message it sees to
stdout. This is a **research / reverse-engineering tool**, not a normal
operational feature — it's how operators figure out what message shapes
the game uses for things like grants, notifications, and travel events.

The upstream capture path assumes the canonical Funcom topology:

- Both brokers live on cluster-internal IPs, reachable only through an
  SSH-tunnelled TCP dialer.
- JWT credentials for the game broker are extracted by execing into the
  Battlegroup Director pod via `kubectl exec`.
- Exchange discovery uses `kubectl exec` into broker pods to run
  `rabbitmqctl list_exchanges`.

None of those operate on AMP. Both brokers are on `127.0.0.1` on the
host (admin at `:5672`, game at `:5673` with TLS); there is no
"Director pod"; there is no `kubectl exec`. But the AMP host *does*
have `rabbitmqctl` available as a direct binary and the Director's
HTTP API responds on `localhost:11717`.

## Decision

When `-capture` is used in direct mode, an alternate code path
(`runCaptureDirect`) replaces every SSH/kubectl operation with a
direct equivalent:

- **AMQP dialing** uses the standard URL form
  (`amqp://user:pass@host:port/`, plus TLS for the game broker) — no
  SSH tunnel.
- **JWT extraction** reads the `ServiceAuthToken=...` argument from a
  game-server process's command line via `ps aux`. The token's
  embedded `HostId` claim is what would otherwise come from the BGD
  pod.
- **Exchange discovery** calls the Director's `/v0/battlegroup` HTTP
  endpoint and pulls `broadcastExchange` values out of each map config,
  augmented by a hard-coded list of well-known exchange names that
  every Dune server exposes.
- **Auth backend re-application** uses `rabbitmqctl` directly (no SSH,
  no kubectl).

The shape of the captured output and the queue-binding semantics are
unchanged. Anything that worked downstream of the existing capture path
continues to work; the change is purely in how the tool **gets** to the
brokers.

The two paths are dispatched by `connectionMode` inside `dialAMQP`,
`runCapture`, and the various discovery helpers. The SSH-mode capture
path is untouched.

## Why this and not X

- **Why not unify the two paths behind a dialer abstraction?** Tempting,
  but the operational steps before dialing (JWT extraction, exchange
  discovery, auth-backend management) are different enough that the
  unification ends up shallow — a dialer interface and three or four
  hooks. Two readable functions are clearer than one parameterized one.
- **Why pull JWT from `ps aux` instead of asking the Director?** The
  Director exposes server state but not credentials. The token lives in
  the game-server process's argv because that's where the game server
  itself was given it at launch; reading it back is the most reliable
  way to obtain a valid, current token without re-implementing auth.
- **Why hard-code a known-exchanges list?** The Director's
  `broadcastExchange` enumeration misses a few of the auxiliary
  exchanges (`notifications`, `grants`, `server_state`, `travel`,
  `player_state`). Until there's an authoritative source for the
  complete list, hard-coding the known names is the practical fallback.
- **Why is this a CLI mode, not a UI feature?** Capture output can be
  thousands of messages per minute and the workflow is "tee to a file
  and grep later". A UI would add latency without adding value to the
  research use case.

## Preserve when re-implementing

1. **`-capture` must work without modifying any AMP/game configuration.**
   Operators run capture against a live server; the tool must not
   restart broker auth backends in a way that disrupts the running
   game. (The current implementation re-applies auth backends on a
   refresh loop because RabbitMQ holds them in memory only; this is
   non-destructive but timing matters.)
2. **The message format printed to stdout must remain stable.** Captures
   are diffed across runs and across game versions; changing the output
   shape silently invalidates years of accumulated traces.
3. **The same `binding` slice must be honored regardless of mode.** A
   binding is `{exchange, routing_key}`; discovery in either mode must
   yield this shape so downstream code can stay mode-agnostic.
4. **TLS handshake skips verification on the game broker** (self-signed
   internal cert). This is acceptable for an admin-only research tool
   on the same host; a different deployment shape would need to be
   re-evaluated.
5. **The capture user (`dune_cap`) creds must be applied in memory only**
   if at all — never persist them to the broker's config files. A
   restart of RabbitMQ should leave no trace of the capture tool's
   existence.

## Implementation-incidental

- Whether JWT extraction uses `ps aux | grep | grep -oP` or a more
  structured proc walk. Anything that returns a current
  `ServiceAuthToken=` value works.
- The two broker addresses (`127.0.0.1:5672` admin and `127.0.0.1:5673`
  game) are AMP-specific defaults — different setups may use different
  ports.
- The username/password constants for the capture user are baked into
  the binary today. Putting them behind env vars is a reasonable
  cleanup.
- The split between `capture.go` (entry point + SSH path) and the new
  `*Direct` helpers within the same file is convenience. A future
  layout might extract `capture_direct.go`.

## Related

- [ADR-001](ADR-001-direct-connect-mode.md) — mode gate.
- See `memory/rabbitmq-research.md` (private notes, not in this repo)
  for the broader story of what capture revealed about Dune's message
  shapes.
