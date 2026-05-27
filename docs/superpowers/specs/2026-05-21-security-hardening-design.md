# Security Hardening + CI Design

**Date:** 2026-05-21  
**Status:** Approved

## Context

dune-admin is a Go backend + React frontend admin tool for Dune. The frontend is hosted at `https://dune-admin.layout.tools/`. Each user runs the Go backend locally; the frontend connects to `localhost:8080`. The backend connects to a remote game VM over SSH.

The current backend has no auth, wildcard CORS, no HTTP timeouts, no SQL guardrails, and no CI.

## Threat Model

The primary risk is a malicious website making cross-origin requests to the user's local `localhost:8080` backend (CSRF-style). Network interception is out of scope — the backend is local-only.

## Approach

CORS lockdown as the primary protection layer. No token auth — it adds complexity without meaningfully improving security given the localhost-only architecture.

---

## Section 1 — Backend Security

### 1.1 CORS Lockdown (`server.go`)

Replace `Access-Control-Allow-Origin: *` with a dynamic allowlist check.

- Default allowed origins: `https://dune-admin.layout.tools`, `http://localhost:5173`
- Configurable via `ALLOWED_ORIGINS` env var (comma-separated) for extensibility
- `Vary: Origin` header set on all responses
- Preflight (`OPTIONS`) returns `204 No Content`
- Requests from unlisted origins: headers not set (browser blocks them)

### 1.2 HTTP Timeouts (`server.go`)

Replace bare `http.ListenAndServe` with `http.Server`:

```
ReadHeaderTimeout: 5s
ReadTimeout:       15s
WriteTimeout:      60s
IdleTimeout:       60s
```

### 1.3 SQL Read-Only Guard (`handlers_database.go`)

Applied to `handleDBSQL` only — the raw SQL console in the DB tab. All other API handlers (give-item, award-xp, etc.) are unaffected.

Logic:

1. Strip SQL line comments (`-- …`) and block comments (`/* … */`) from the query before validation
2. Check the stripped statement begins with an allowed prefix: `SELECT`, `EXPLAIN`, `SHOW`
3. Reject everything else with `400 Bad Request`

This prevents accidental or intentional destructive queries (`DROP`, `DELETE`, `UPDATE`, `INSERT`) via the console.

### 1.4 Request Body Limit (`handlers_database.go`)

Apply `http.MaxBytesReader(w, r.Body, 1<<20)` (1MB) to `handleDBSQL` before reading the request body.

### 1.5 Log Stream Input Validation (`handlers_logs.go`)

Validate `namespace` and `pod` query params in `handleLogStream` against a Kubernetes name regex (`^[a-z0-9][a-z0-9\-\.]{0,251}[a-z0-9]$`) before passing to `sshExec`. Reject invalid values with `400 Bad Request`.

---

## Section 2 — CI Actions

Three new workflow files in `.github/workflows/`.

All use `pull_request` trigger (not `pull_request_target`) — for fork PRs, workflows run from `main` and secrets are not injected, limiting blast radius of malicious test code.

### 2.1 `test.yml` — Go Tests

Triggers: push and PR to `main`

- `go vet ./...`
- `go test -race ./...`

Fails build on any test failure or vet error.

### 2.2 `sast.yml` — Static Analysis (gosec)

Triggers: push and PR to `main`

- Installs and runs `gosec ./...`
- Flags: hardcoded credentials, unsafe file ops, SQL injection patterns, etc.

Fails build on HIGH severity findings.

### 2.3 `sca.yml` — Dependency Vulnerability Scan

Triggers: push and PR to `main`

- Backend: `govulncheck ./...` (Go vuln DB)
- Frontend: `npm audit --audit-level=high` in `web/`

Fails build on HIGH or CRITICAL findings.

---

## Out of Scope

- Token auth (not needed for localhost-only architecture)
- `handleBGExec`, `handleGiveItem` body limits (follow-up PR)
- Clerk integration changes
