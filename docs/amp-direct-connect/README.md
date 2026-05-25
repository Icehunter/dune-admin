# AMP Direct-Connect Adaptation

This folder documents a set of changes that let `dune-admin` work against a
**Dune: Awakening** server hosted under [CubeCoders AMP](https://cubecoders.com/AMP)
instead of the upstream k3s + SSH topology assumed by Icehunter's original
design.

These docs describe **concepts and decisions**, not line-by-line code. If a
later refactor rewrites the UI, restructures handlers, or moves files around,
the intent captured here should be enough to rebuild the same capabilities on
the new shape.

## Why this exists

Upstream `dune-admin` assumes the canonical Funcom topology:

- A Linux host (the Battlegroup VM) reachable over SSH.
- A k3s cluster inside that VM running the game-server pods.
- Operations against the cluster shell out to `kubectl` over an SSH session.
- Database, RabbitMQ, and the Battlegroup Director are reached via
  cluster-internal IPs through SSH-tunnelled connections.

AMP-hosted setups don't look like that. AMP runs the Dune server inside a
**Podman container** that emulates parts of the Funcom stack (a "mock-k3s-go"
shim, a local PostgreSQL, RabbitMQ brokers, and the Battlegroup Director),
but there is no real Kubernetes API to talk to and no need to tunnel anything
— PostgreSQL is reachable on `127.0.0.1` and the Director responds on
`localhost:11717`.

The "direct-connect" adaptation teaches `dune-admin` to operate against this
flatter topology when configured to do so, without breaking the SSH path that
canonical Funcom deployments still depend on.

## What changed at a glance

| ADR | Concept | Backend file(s) | Frontend impact |
|---|---|---|---|
| [001](ADR-001-direct-connect-mode.md) | A connection-mode toggle that skips SSH and connects to Postgres directly | `direct.go`, `main.go`, `setup.go`, `.env.example` | None — backend mode flag only |
| [002](ADR-002-amp-battlegroup-adapter.md) | Battlegroup listing from `ps`/podman instead of `kubectl get pods` | `handlers_battlegroup.go` | Same JSON contract, extra fields available |
| [003](ADR-003-websocket-log-streaming.md) | Log streaming from container log files via WebSocket instead of `kubectl logs -f` | `handlers_logs.go` | Same WS message shape, "pod name" becomes "filename" |
| [004](ADR-004-director-reverse-proxy.md) | Battlegroup Director dashboard reverse-proxied at `/director/` | `server.go` | None — pure server-side routing |
| [005](ADR-005-rabbitmq-capture-mode.md) | RabbitMQ capture without SSH/kubectl, for game-protocol research | `capture.go` | None — CLI-only feature |
| [006](ADR-006-progression-presets.md) | Curated journey-completion bundles as one-click presets | `progression_presets.go` | New endpoint + UI buttons |

Plus a small but useful piece of plumbing in `server.go`: the Go process can
**serve the SPA frontend** from `./dist` when it exists, so a typical AMP
deployment is a single binary + a `dist/` directory behind one port.

## How the pieces fit together

```
┌─────────────────────────────────────────────────────────────┐
│  Browser  ─── http://host:9090 ───►  dune-admin (one Go bin)│
│                                       │                     │
│                            ┌──────────┼──────────┐          │
│                            ▼          ▼          ▼          │
│                       SPA assets   JSON API   /director/    │
│                       (./dist/)        │      (proxy)       │
│                                        │          │         │
│         ┌──────────────────────────────┼──────────┼─────┐   │
│         ▼                              ▼          ▼     ▼   │
│   PostgreSQL                      ps / podman     BGD  ...  │
│   127.0.0.1:15432                 (game procs)   :11717     │
│                                                             │
│         ▲                                                   │
│         └── direct TCP, no SSH tunnel                       │
└─────────────────────────────────────────────────────────────┘
```

In SSH mode the same backend instead tunnels DB, AMQP, and `kubectl`
operations through the existing SSH client. The `connectionMode` toggle is
the only thing that decides which path each handler takes.

## How to read these ADRs when porting to a new UI

Each ADR ends with two short sections:

- **Preserve when re-implementing** — the invariants that must hold on a
  rewrite. If the new code violates one of these, the adaptation is broken
  even if it compiles.
- **Implementation-incidental** — things that exist in the current files
  only because of how the old single-file tabs were structured. A new UI
  framework or component layout is free to discard or redesign these.

Use the "preserve" list as a checklist after any large refactor. Anything in
"incidental" is safe to throw away.

## See also

- [`deployment.md`](deployment.md) — VM layout, systemd unit concept,
  environment-variable contract.
- The repo-level `README.md` for upstream-aligned usage (SSH topology).
