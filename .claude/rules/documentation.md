---
paths: "**/*.md"
---

# Documentation Standards

## CRITICAL: Markdown Linting

After creating or modifying any markdown file, run:

```bash
make lint-md
```

This uses `markdownlint-cli2` with auto-fix. All markdown must pass before committing.

## File Locations

- **Project docs**: `docs/` — architecture, guides, design decisions
- **Root level**: `README.md`, `CLAUDE.md`, and the `SETUP_*.md` control-plane guides
  (`SETUP_AMP.md`, `SETUP_DOCKER.md`, `SETUP_KUBECTL.md`, `SETUP_LOCAL.md`)
- **Rule files**: `.claude/rules/`

## Naming Conventions

- UPPERCASE for important docs: `README.md`, `SETUP_DOCKER.md`
- Descriptive names: `SETUP_KUBECTL.md` not `k8s.md`
- Underscores for multi-word names

## Markdown Style

### Headers

- ATX-style (`#` syntax)
- One H1 per document
- Blank line before and after headers
- No trailing punctuation in headers

### Code Blocks

- Always specify language for syntax highlighting
- Fenced code blocks only (no indented blocks)

### Lists

- `-` for unordered lists (not `*` or `+`)
- `1.` for ordered lists
- Blank line before and after lists

## Documentation Maintenance

Update docs in the same PR as code changes when:

- Adding new API endpoints → update `CLAUDE.md` (and `docs/swagger.json`/`.yaml` if regenerated)
- Changing handler patterns → update `.claude/rules/api-design.md`
- Changing frontend patterns → update `.claude/rules/frontend.md`
- Introducing new global state → update `.claude/rules/architecture.md`
- Adding config options → update the Configuration section of `CLAUDE.md` (and the relevant `SETUP_*.md` guide)

## Documentation Checklist

- [ ] `make lint-md` run successfully
- [ ] Code examples tested and working
- [ ] File in correct directory
- [ ] Follows naming conventions
- [ ] Links are valid
