# Security guidance

Dune Admin controls a live game server, reaches a VM over SSH, and can mutate the game database. Treat the backend as a privileged admin service.

## Required runtime settings

Set these values in `.env` before starting the backend:

```env
ADMIN_TOKEN=<long random token>
LISTEN_ADDR=127.0.0.1:8080
ALLOWED_ORIGINS=http://localhost:5173,https://dune-admin.layout.tools
```

Generate a token with:

```bash
openssl rand -base64 32
```

The frontend stores the token in browser `localStorage` and sends it as a bearer token to the backend.

## Exposure rules

- Prefer `127.0.0.1:8080` for local use.
- Do not expose the backend directly to the internet.
- If remote access is required, put the backend behind TLS, a trusted reverse proxy, and a strong identity provider.
- Rotate any credentials that were previously committed or shared.
- Keep `.env`, SSH keys, and database snapshots out of git.

## Raw SQL

The database tab only allows read-only SQL prefixes from the backend. Destructive database changes should be implemented as specific, audited admin actions rather than arbitrary SQL.
