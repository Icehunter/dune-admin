# ADR-002: AMP battlegroup adapter

**Status:** accepted
**Date:** 2026-05-23

## Context

The Battlegroup tab in upstream `dune-admin` shows the running game-server
"pods" — one per map (Survival, Overmap, DeepDesert, Arrakeen, etc.). On a
real Funcom deployment those are k3s pods, and the data comes from `sudo
kubectl get pods -n funcom-seabass-<bg>`.

Under AMP, there are no real k3s pods. The Dune game servers are plain
processes inside a Podman container (`AMP_MehDune01`), parented by the
`amp` user, each invoked with arguments like:

```
DuneSandboxServer-Linux-Shipping DuneSandbox Survival -Port=10000 -PartitionIndex=0
```

`kubectl` either doesn't exist or — through the "mock-k3s-go" shim — doesn't
return useful data. But everything the Battlegroup tab actually needs is
discoverable from `ps`:

- PID and per-process CPU / RSS
- The map name (positional argument)
- The port (`-Port=NNNN`)
- The partition index (`-PartitionIndex=N`)

## Decision

When `connectionMode == "direct"`, the `/api/v1/battlegroup/status` handler
short-circuits to a Podman/`ps`-backed implementation that:

1. Shells out to `ps -eo pid,pcpu,rss,args` and filters for
   `DuneSandboxServer-Linux-Shipping`.
2. Parses each line into a structured row: `{pid, map, port, partition,
   cpu, mem_mb}`.
3. Returns the **same JSON envelope** the original handler would have
   returned, plus the extra process-level fields that the AMP path can
   provide (CPU%, memory, PID, port).

The frontend doesn't need to know which path produced the data. The
"extra" fields are optional in the JSON contract — a UI that ignores them
behaves identically to upstream; a UI that surfaces them gets a richer
view at no cost.

Command-execution endpoints (`/api/v1/battlegroup/exec`,
`/api/v1/battlegroup/pods`, backup file download/restore/upload) all
require kubectl and are not adapted. Each is gated behind `requireSSH(w)`
and returns 501 in direct mode. The Battlegroup *visibility* feature is
adapted; the *administration* feature stays SSH-only.

## Why this and not X

- **Why parse `ps` instead of polling the Director?** The Battlegroup
  Director also knows about game servers, but its API is unauthenticated
  on `localhost:11717` and the shape changes between versions. `ps` is
  stable, requires no AMP-specific knowledge, and works whether or not
  the Director is running.
- **Why not query Podman directly (`podman ps`)?** That would only show
  the container, not the per-game-server processes inside it. `ps` on the
  host sees all processes in the container's PID namespace because the
  container is not in a separate one (that's how AMP runs it).
- **Why preserve the JSON shape?** So a single frontend can talk to either
  backend mode. Mode is a server concern; the UI shouldn't have two code
  paths.

## Preserve when re-implementing

1. **Same JSON keys as the SSH/k8s response** for the fields that
   overlap (`map`, `partition`, `phase`, `ready`, `players`). Renaming
   any of these breaks the frontend.
2. **Optional extra fields** (`pid`, `cpu`, `mem_mb`, `port`) — they may
   be `0`/empty in SSH mode and populated in direct mode. The UI must
   tolerate missing or zero values.
3. **The handler must not crash if there are no game-server processes.**
   Return an empty list, not an error. AMP can be running with no
   game servers spun up yet.
4. **Use the existing argument-parsing logic** (regex for `-Port=` and
   `-PartitionIndex=`) — these match how AMP actually invokes the
   server. A new implementation must keep up with any change to the
   game's argv convention.

## Implementation-incidental

- The exact shape of the `ps` command (`-eo pid,pcpu,rss,args`). Any
  command that returns the same six values works.
- The `bash -c "ps ... | grep ..."` invocation. A direct
  `exec.Command("ps", ...)` would be cleaner.
- The frontend's single-file `BattlegroupTab.tsx` layout. The upstream
  v0.6.0 refactor splits this into a directory; the JSON contract above
  is what the new layout must consume.
- The `serverRow` Go struct's exact field order.

## Related

- [ADR-001](ADR-001-direct-connect-mode.md) — the mode toggle that gates
  this handler.
- [ADR-003](ADR-003-websocket-log-streaming.md) — same shape of adapter,
  applied to log streaming.
- [ADR-004](ADR-004-director-reverse-proxy.md) — what to use instead of
  the SSH-mode admin commands for AMP setups.
