# ADR-004: Reverse-proxy the Battlegroup Director at `/director/`

**Status:** accepted
**Date:** 2026-05-23

## Context

The **Battlegroup Director (BGD)** is a Funcom-supplied service that
runs alongside the game servers. It exposes a small HTML dashboard
(metrics, state, server roster) and a JSON API at `:11717`. On AMP
deployments it listens on `127.0.0.1:11717` inside the container's
network namespace.

Out of the box, the dashboard is only accessible to whoever can reach
`127.0.0.1` on the VM — typically the AMP operator over SSH. To make it
accessible to remote admins of the Dune server, the choices are:

1. **Expose port 11717** on the VM (network rule + container port
   publishing + maybe a firewall change).
2. **Front it with a separate reverse proxy** (nginx/Caddy/Traefik), which
   means another moving part to install, configure, and keep secured.
3. **Have `dune-admin` proxy it** under a path the operator is already
   willing to expose.

Option 3 is the lowest-friction choice for the AMP target audience: they
already have to expose the `dune-admin` HTTP port to use the tool at all,
so reusing that surface costs zero new attack surface and zero new
operational dependencies.

## Decision

When `connectionMode == "direct"` and a Director URL is configured (env
`DIRECTOR_URL`, default `http://127.0.0.1:11717`), `dune-admin` mounts a
**single-host reverse proxy** at `/director/` that:

- Strips the `/director` prefix from inbound paths.
- Forwards the rewritten request to the Director's listen address.
- Rewrites the `Host` header to the Director's host so it can route
  internally if it cares.
- Returns the proxied response unchanged.

The route is **only** mounted in direct mode. In SSH mode, the Director
is on a cluster-internal address and proxying it would require an
SSH-tunnelled HTTP client; rather than build that, SSH-mode operators
keep using their existing access path.

A consequence: any path beginning with `/director/` is reserved by this
feature and is not available for other server-side routes or SPA URLs.

## Why this and not X

- **Why a built-in reverse proxy and not a recommendation to run nginx?**
  This fork's target audience is AMP operators who don't already run a
  reverse-proxy stack. Adding one to the install instructions raises the
  bar; building one in keeps the tool a single-binary deployment.
- **Why not also proxy in SSH mode?** Doable, but requires an SSH-tunnelled
  HTTP transport. The benefit doesn't pay for the complexity for users who
  already have the SSH session to the VM in their day-to-day operations.
- **Why `/director/` instead of a subdomain?** Subdomains require DNS work
  and possibly a separate TLS cert. A path prefix works on any deployment
  shape, including localhost.
- **Why no auth in front of the proxy?** Same authorization story as the
  rest of `dune-admin` — the caller is whoever can reach the port. If the
  tool grows real auth (Clerk is already in the frontend), the proxy
  inherits it because it sits on the same mux.

## Preserve when re-implementing

1. **Only mount in direct mode and only when `directorURL` is set.** A
   non-direct deployment, or one without a configured Director, must not
   expose the route at all. Mounting it with an empty target leaks a
   confusing 502 surface.
2. **Strip the `/director` prefix exactly once** and only on the way in.
   Forgetting to strip it sends `/director/foo` to the Director, which
   responds with 404 because it doesn't know about the prefix.
3. **Set the request Host header to the target's host.** Without this,
   Director responses with absolute URLs (links, redirects) break in
   subtle ways.
4. **Log the proxy mount at startup** so operators can confirm it is or
   isn't active without reading config files.
5. **Use a path prefix, not a subdomain.** Subdomain-based proxying
   would require TLS cert work outside the operator's control.

## Implementation-incidental

- The handler is built inline inside `startServer`. A new layout might
  extract it into a `proxy.go` or middleware module — fine, as long as
  the four invariants above hold.
- The use of `httputil.NewSingleHostReverseProxy` is just convenience;
  a hand-rolled `http.Handler` that does the same thing is equivalent.
- The default `DIRECTOR_URL` value (`http://127.0.0.1:11717`) — change
  it freely for non-default Director ports.

## Related

- [ADR-001](ADR-001-direct-connect-mode.md) — the mode gate.
- The repo-level deployment notes for any future story about
  exposing the Dune admin frontend behind real TLS / auth.
