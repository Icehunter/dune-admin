# ADR-003: WebSocket log streaming for AMP setups

**Status:** accepted
**Date:** 2026-05-23

## Context

Upstream `dune-admin` streams pod logs by opening a WebSocket to the
backend, which runs `sudo kubectl logs -f` over the existing SSH session
and pipes stdout back through the WS connection.

On AMP, there are no k3s pods to `kubectl logs` against. The game-server
logs are plain files inside the Podman container at
`/AMP/duneawakening/logs/`:

```
DuneSandbox-Survival.log
DuneSandbox-DeepDesert.log
DuneSandbox-Overmap.log
...
```

A typical Dune-server admin still wants the same experience: pick a log,
get a tailing live stream in the browser. The change is the **data
source**, not the protocol or the UI.

## Decision

In `direct` mode, the existing `/api/v1/logs/pods` and
`/api/v1/logs/stream` endpoints transparently switch to filesystem
operations inside the container:

- **Listing** (`/api/v1/logs/pods`): `podman exec` into the container,
  `ls -la /AMP/duneawakening/logs/`, parse the entries, return each
  `.log` file as a `{namespace: "logs", name: "<filename>.log"}` entry.
- **Streaming** (`/api/v1/logs/stream?pod=<filename>`): `podman exec`
  with `tail -n 200 -f /AMP/duneawakening/logs/<filename>`, capture
  stdout line by line, send each line as a WS text message.

The frontend doesn't need a new code path. From its perspective the
endpoint still returns a list of `{namespace, name}` entries and the
streamer still emits one text line per WS frame. Direct mode reuses the
"namespace" field as a constant (`"logs"`) and treats the "name" as a
filename.

A filename allowlist regex (`^[a-zA-Z0-9._-]+\.log$`) guards the streaming
endpoint against shell injection — the parameter ends up inside a
`podman exec ... tail -f /path/<name>` command line, so it must not be
trusted directly.

## Why this and not X

- **Why podman exec instead of reading the log files directly from the
  host filesystem?** The container's filesystem mounts may be ephemeral
  or use an overlayfs path that's awkward from outside; `podman exec`
  always sees what the container sees, in the container's own view of
  paths. It also gives the streamer the container's locale and TZ for
  any timestamp formatting downstream.
- **Why `tail -n 200 -f` rather than a more sophisticated streamer?**
  It's a one-line command that handles file rotation reasonably
  (`tail -F` would be slightly better but is less portable). Anything
  more complex earns its complexity only if we see a need.
- **Why keep the WS protocol identical to SSH mode?** So a single
  frontend file can stream logs regardless of the backend's mode. Mode
  selection is a backend concern.

## Preserve when re-implementing

1. **The WS message contract**: one text frame per log line. No JSON,
   no framing — the frontend already accumulates lines.
2. **The filename allowlist** must validate before the filename touches a
   shell command. This is a security boundary; removing or weakening
   the regex re-introduces command injection.
3. **The listing endpoint must return only `.log` files**, never
   directories, dotfiles, or arbitrary container files. Treat the
   handler's output as user-selectable; whatever you return becomes a
   potential streaming target.
4. **Stream cancellation must kill the child process.** A user who
   closes the WS or navigates away should not leave a zombie
   `podman exec ... tail -f` consuming the WS write buffer forever.
   The current implementation does this with a `cancel func()` returned
   alongside the channel.
5. **The `namespace` field exists in the JSON for frontend
   compatibility** — the upstream UI uses it to render a namespace
   column. Keep it as a constant value (`"logs"`) in direct mode.

## Implementation-incidental

- The exact buffer size (`1 MB max line`) on the stdout scanner. This
  was sized for Dune's verbose lines but is not a contract.
- The hard-coded `tail -n 200` initial-history count. A query parameter
  would be a reasonable enhancement.
- Whether the namespace constant is `"logs"`, `"local"`, or anything
  else — the frontend currently displays it but does not act on it.
- The fact that listing uses `ls -la` parsed text vs `find -printf`.
  Anything that produces filename + size works.
- The file lives in `direct.go` (helper functions) and
  `handlers_logs.go` (HTTP handlers). A new layout might colocate
  these or move the regex to a shared validation module.

## Related

- [ADR-001](ADR-001-direct-connect-mode.md) — mode toggle that selects
  this path.
- [ADR-002](ADR-002-amp-battlegroup-adapter.md) — same adapter pattern
  applied to battlegroup discovery.
