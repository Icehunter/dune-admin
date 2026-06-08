#!/bin/bash
# Session start hook — remind Claude to follow project rules

cat <<'EOF'
IMPORTANT: dune-admin has strict development requirements in CLAUDE.md and .claude/rules/.

Before writing any code:
- Read CLAUDE.md — especially the "Mandatory Workflow" and "Critical Gotchas" sections
- The HTTP backend is one flat package main in cmd/dune-admin/ — keep the server flat. Reusable standalone libraries (e.g. internal/marketbot) live under internal/; default to cmd/dune-admin/.
- Write tests FIRST. No implementation without a test file.
- Run `make verify` before considering any task complete.
- Never commit without explicit user approval.
EOF
