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
- **Root level**: Only `README.md`, `CLAUDE.md`, `AGENTS.md`, `CONTRIBUTING.md`, `CHANGELOG.md`
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

- Adding new API endpoints → update `AGENTS.md` and/or `docs/`
- Changing handler patterns → update `.claude/rules/api-design.md`
- Changing frontend patterns → update `.claude/rules/frontend.md`
- Introducing new global state → update `.claude/rules/architecture.md`
- Adding config options → update `AGENTS.md` configuration section

## Documentation Checklist

- [ ] `make lint-md` run successfully
- [ ] Code examples tested and working
- [ ] File in correct directory
- [ ] Follows naming conventions
- [ ] Links are valid
