#!/bin/bash
# Session start hook — remind Claude to follow project rules

cat <<'EOF'
IMPORTANT: dune-admin has strict development requirements in CLAUDE.md and .claude/rules/.

Before writing any code:
- Read CLAUDE.md — especially the "Mandatory Workflow" and "Critical Gotchas" sections
- The entire Go backend is package main (cmd/dune-admin/). Never create sub-packages.
- Write tests FIRST. No implementation without a test file.
- Run `make verify` before considering any task complete.
- Never commit without explicit user approval.
EOF
