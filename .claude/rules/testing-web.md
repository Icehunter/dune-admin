---
paths: "web/**"
---

# Frontend Testing Standards

The Go backend is well-tested (`testing.md`); the **frontend is the weak spot** and must be brought
up as we go. Build comprehensive tests + lints incrementally to prevent regression — the same
ratcheting rule as the Go side.

## Current state (gap to close)

- `vitest` is installed but **not wired up**: there is no `test` script in `web/package.json`, no
  Vitest config, and no DOM environment / Testing Library installed. Two test files exist
  (`src/i18n/i18n.test.ts`, `src/tabs/PlayersTab/views/giveItemsHelpers.test.ts`) but nothing runs
  them — they are orphaned.
- ESLint has **no** accessibility plugin (`eslint-plugin-jsx-a11y` is absent), so WCAG is unverified.
- **CI runs no frontend gate** beyond a dependency audit — no `pnpm lint`, no `tsc`, no Vitest on PRs.
  Local git hooks run lint/tsc on changed files but are bypassable with `--no-verify`.

## Target setup (wire this up early)

- Add scripts to `web/package.json`: `"test": "vitest run"`, `"test:watch": "vitest"`,
  `"test:coverage": "vitest run --coverage"`. Move `vitest` to `devDependencies`.
- Add a Vitest config with a `jsdom` (or `happy-dom`) environment.
- Install (as devDependencies) `@testing-library/react`, `@testing-library/jest-dom`,
  `@testing-library/user-event` for component/interaction tests, and `eslint-plugin-jsx-a11y` for
  a11y linting (recommended ruleset) in `eslint.config.js`.
- Add a11y assertions (`vitest-axe`/`jest-axe`) to component tests, or a Playwright +
  `@axe-core/playwright` smoke suite for WCAG regression.
- Add a frontend CI job that runs on PRs: `pnpm lint` (`--max-warnings 0`), `pnpm typecheck`
  (`tsc`), and `pnpm test`. CI — not hooks — is the durable enforcement point.

## Conventions

- **Pure logic** (helpers, formatters, reducers): plain Vitest unit tests. Prefer extracting logic
  out of components so it is unit-testable.
- **Components**: Testing Library — assert on accessible roles/labels and user-visible behaviour, not
  implementation details. Drive interactions with `user-event`.
- **TDD where practical**: for a bug fix, write the failing test first; for new pure logic, write the
  test alongside.
- **Mock the API**: stub `api/client.ts` calls; never hit a real backend in unit/component tests.
- **No flakiness**: no real timers/sleeps — use Vitest fake timers; await Testing Library `findBy*`.
- **a11y is part of the test**, not separate: a component test should assert it has no axe violations.

## Checklist

- [ ] New/changed pure logic has a Vitest unit test
- [ ] New/changed UI has a Testing Library component test (roles/labels, behaviour)
- [ ] Bug fix includes a regression test that failed before the fix
- [ ] a11y assertion (axe) on rendered components where applicable
- [ ] `pnpm lint` (incl. jsx-a11y once enabled) and `pnpm test` pass; `tsc` clean
