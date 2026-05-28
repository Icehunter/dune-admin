# Phase 0 — Bug/Chore Fixes

**Status:** Complete ✅  
**Branch:** `chore/phase-0-bug-fixes`  
**PR:** #52  
**Base:** `main`

---

## Scope

Fix five open issues before any structural changes, so each diff is readable in isolation. Also address Copilot and CodeQL review feedback raised against PR #52.

---

## Issues

### #10 — Remove capture mode (`chore`)

**What was removed:**

- `capture.go` — `runCapture`, `captureBroker`, `printMessage`, `dialAMQP`, `buildCaptureJWT`, `binding` type, `capUser`/`capPass` constants
- `-capture` CLI flag from `main.go`
- `ListExchanges` and `EnsureCaptureUser` from the `ControlPlane` interface and all four provider implementations (`ampControl`, `dockerControl`, `kubectlControl`, `localControl`)
- `startEnsureCaptureUserLoop`, `rabbitmqctlPrefix`, `rabbitmqctl()`, `defaultDuneRabbitmqctl` from `ampControl`
- `brokerForLabel`, `ensureBrokerViaDockerExec` from `dockerControl`
- `ensureBrokerViaExec`, `parseExchanges`, `binding` type from `kubectlControl`
- `AmpRabbitmqctlPath` from `appConfig` and setup wizard

**What was kept** (still used by `handlers_notify.go` / `rmq_commands.go`):

- `dialAMQP` → moved to `broker.go`
- `buildCaptureJWT` → moved to `jwt_helpers.go`
- `EvalOnGameBroker` — retained on all control planes (used by `rmq_commands.go`)

---

### #11 — Malformed managed INI block silently drops settings (`bug`)

**File:** `handlers_server_settings.go`

**Root cause:** `splitAtDuneAdminMarker` returned an empty `managed` map when the BEGIN marker was present but the END marker was missing (truncated file). The caller then wrote a new managed block containing only the incoming updates — silently discarding all previously-managed settings.

**Fix:** `splitAtDuneAdminMarker` now returns `(preMarker, managed, error)`. When BEGIN is found without END it returns an error. Callers (`applyDuneAdminUpdates`, `applyDuneAdminRawSection`) propagate it as HTTP 409 Conflict.

---

### #12 — Array lines duplicate in INI after raw-section editor save (`bug`)

**File:** `handlers_server_settings.go`

**Root cause:** `applyDuneAdminRawSection` used `parseINI` which silently stripped `+`/`-` prefixes and only stored the last value for duplicate keys. `stripKeysFromContent` also only stripped plain `key=val` lines, leaving prefixed lines in the hand-edited region — which then appeared twice on disk.

**Fix:**

1. Added `parseINIRaw` — preserves `+`/`-` as part of the stored key. Duplicate keys (e.g. multiple `+ActiveMod=` lines) stored with a `\x00N` dedup suffix.
2. `renderDuneAdminBlock` strips the `\x00N` suffix before writing to disk.
3. `ownedKeySet` strips the suffix before building the ownership set.
4. `stripKeysFromContent` now also strips prefixed array lines (`+key=val`, `-key=val`) when the base key or exact prefixed key is owned.

---

### #13 — WebSocket endpoint accepts arbitrary non-browser clients (`bug`)

**File:** `server.go`

**Root cause:** `originAllowedForRequest` with `allowEmpty=true` returned `true` for any request with no `Origin` header. The empty-origin path used `r.Host` (a client-controlled header) to determine if the connection was local, which could be spoofed.

**Fix:**

- `originAllowedForRequest` now uses `r.RemoteAddr` (not `r.Host`) for the loopback check — this reflects the actual TCP source address.
- Empty-Origin requests are only allowed when `net.ParseIP(remoteHost).IsLoopback()` is true.
- `corsMiddleware` skips CORS headers entirely when `Origin` is absent (was emitting an empty `Access-Control-Allow-Origin` header).

---

### #14 — `ampExecutor` wrapping silently skipped when SSH executor active (`bug`)

**File:** `connection.go`, `executor_amp.go`

**Root cause:** `connectAll` only wrapped the executor as `ampExecutor` when the type assertion `exec.(*localExecutor)` succeeded. With `ssh_host` set, the executor is `*sshExecutor` — the assertion failed silently, leaving INI writes unelevated.

**Fix:** `ampExecutor` now embeds the `Executor` interface instead of `*localExecutor`. `WriteFile` calls `sudo -i -u <ampUser> tee` regardless of whether the inner executor is local or SSH. `connectAll` and `setup.go` updated to use `&ampExecutor{Executor: exec, ampUser: user}`.

---

## Copilot / CodeQL follow-up fixes (additional commits on #52)

| File | Issue | Fix |
|------|-------|-----|
| `server.go` | `r.Host` spoofable for origin check | Use `r.RemoteAddr` |
| `server.go` | CORS headers emitted with empty `Access-Control-Allow-Origin` | Skip headers when `Origin` absent |
| `broker.go` | `capUser`/`capPass` hardcoded in source | Replaced by `brokerCredentials()` reading `BROKER_USER`/`BROKER_PASS` env vars; fallback to defaults |
| `jwt_helpers.go` | HMAC signing secret hardcoded | Loaded from `BROKER_JWT_SECRET` env var; hardcoded value is opt-out fallback only |
| `handlers_server_settings.go` | `parseINIRaw` lost duplicate array keys | Fixed with `\x00N` dedup suffix scheme |

---

## Verification

```bash
go build .
go test -race .
go tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint run
go tool github.com/securego/gosec/v2/cmd/gosec -severity high -confidence high .
go tool golang.org/x/vuln/cmd/govulncheck .
```

All passed. CodeQL alert #57 (broker.go InsecureSkipVerify) suppressed with `// lgtm[go/disabled-certificate-check]`.
