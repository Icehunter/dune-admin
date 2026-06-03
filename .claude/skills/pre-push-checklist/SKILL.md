---
name: pre-push-checklist
description: Run before any git push or PR creation. Ensures gosec, vulncheck, tests, and lint all pass — and that the user has approved the commit.
---

# Pre-Push Checklist — dune-admin

The pre-push git hook runs a fuller suite than `make verify`. Run these manually before pushing.

## Steps

```bash
make verify        # go test -race, golangci-lint, markdownlint, gocognit
make gosec         # ⚠ NOT included in make verify — required before any push
make vulncheck     # dependency vulnerability scan
```

If `make gosec` reports findings:

- **False positive on re-exec / slice args?** Suppress with `// #nosec G204,G702 -- <reason>` — include **both** rule IDs (G204 alone does not suppress G702).
- **Real finding?** Fix it. Do not `git push --no-verify`.

## Gate: user approval

Before committing or pushing, **stop and show the diff to the user**.

Do NOT commit unless the user explicitly says "commit" or "go ahead and commit."

Omit commit steps from all implementation plans and subagent dispatches.

## PR description template

```
[TICKET-ID] Short descriptive title

## What
- bullet: what changed

## Why
- bullet: why it was needed

## Testing
- make verify passes
- make gosec clean
```

## Checklist

- [ ] `make verify` passes
- [ ] `make gosec` clean (or findings justified with #nosec)
- [ ] `make vulncheck` no critical findings
- [ ] Frontend: `cd web && pnpm build` clean (if web/ changed)
- [ ] User has reviewed and approved the diff
- [ ] No `git push --no-verify` used
