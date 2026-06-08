---
paths: "**/*"
---

# Security Standards — Security First

**Security comes first, in all things.** Every change — backend, frontend, config, docs — is made
with security as the primary constraint, not an afterthought. When a security concern conflicts with
convenience, security wins. If you are unsure whether something is safe, stop and flag it.

## Current posture (read this first)

- **⚠️ The backend performs NO authentication today.** The SPA sends a Clerk `Bearer` token, but the
  Go backend never verifies it — there is no auth middleware and no per-endpoint authorization. Any
  client that can reach the listen address (default `:8080`) can call **every** endpoint, including
  destructive ones (give items/currency, delete account, teleport, broadcast, SQL, market bot,
  server settings). When `VITE_CLERK_PUBLISHABLE_KEY` is absent the SPA also runs fully open.
- `jwt_helpers.go` only re-signs the **game** server's broker `ServiceAuthToken` for capture — it is
  **not** admin authentication. Any doc claiming otherwise is wrong.
- **Frontend gates (`isSignedIn`) are cosmetic** — they hide UI but do not stop the API call.
- **Top priority**: add real backend auth (verify the Clerk session JWT in middleware against JWKS,
  put the verified identity + role in request context) and a server-enforced role model
  (admin vs player) before exposing anything to non-operators. This is the hard prerequisite for the
  player view (project goals 3 & 4). Until then, treat every endpoint as unauthenticated and do not
  add new endpoints that assume the caller is trusted.

## Backend (Go)

- **Authorization is server-side.** Never rely on the React UI to gate an action. Anything that must
  be restricted must be enforced in the handler/middleware against a server-verified identity/role.
- **Parameterised SQL only.** All queries live in `db.go`, use the `dune.` schema prefix, and use
  pgx named parameters. Never build SQL by string concatenation of user input. The admin SQL endpoint
  is read-only — keep `isReadOnlySQL` (SELECT/EXPLAIN/SHOW/WITH only).
- **Validate before shell/exec.** Validate every value interpolated into a shell command, container
  name, log filename, kubectl/podman/docker arg, or file path (`isValidK8sName`, allowlists, regex).
  Unvalidated interpolation into `exec.Command` is a command-injection bug *and* a gosec failure.
- **gosec is a gate.** `make verify` does **not** run gosec — run `make gosec` separately before any
  push that touches `exec.Command`, SQL, or file paths (the pre-push hook gates on it). Suppress only
  true false positives with `// #nosec G204,G702 -- <reason>` (both IDs required). Never
  `git push --no-verify`.
- **No secrets in output.** Never log or return secrets (`ServiceAuthToken`, `amp_api_pass`, DB
  credentials, session tokens) in logs, error messages, or API responses. Wrap errors with context,
  not secrets.
- **CORS stays strict.** Keep the `originAllowed` allowlist (`ALLOWED_ORIGINS`); don't widen to `*`.
- **Market bot — player orders are inviolable.** Never delete/expire/modify non-NPC exchange orders.
  Every `DELETE`/`UPDATE` on exchange tables must include `WHERE … AND is_npc_order = TRUE AND
  owner_id = <botID>`.
- **Player-scoped data.** Player-facing reads must derive the account from the authenticated identity,
  never trust an id from the path/body. No "give me player {id}" endpoint may be exposed to players.
- **Verify-code / link flows** (character linking, goal 3): codes must be single-use, short-lived
  (≈5–10 min), rate-limited per user and per account, and compared in constant time. Deliver them
  over the existing single-player whisper rail (`POST /api/v1/chat/whisper`).

## Frontend (web)

- A frontend gate is never a security control — see Backend authorization above.
- Don't expand the attack surface: the backend URL is runtime-configurable via
  `localStorage('dune_admin_backend')` — keep secrets out of the SPA and localStorage.
- Sanitise/escape any user-controlled content rendered as HTML; avoid `dangerouslySetInnerHTML`.

## Config & secrets

- Config files (`~/.dune-admin/config.yaml`) are mode 600 and never committed. Keep example/`.env`
  files free of real credentials.
- Sudoers grants stay narrow (specific binaries/paths) — see the AMP section in `CLAUDE.md`.

## Security Checklist

- [ ] No action relies on a frontend-only gate; restricted actions enforced server-side
- [ ] SQL parameterised, in `db.go`, `dune.` prefix; SQL endpoint stays read-only
- [ ] Every value interpolated into exec/shell/path/container/k8s name is validated
- [ ] `make gosec` clean (and `make verify`) before push; no `--no-verify`
- [ ] No secrets in logs, errors, responses, or committed files
- [ ] New player-facing reads are account-scoped to the authenticated identity
