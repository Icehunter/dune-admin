---
paths: "web/**"
---

# Frontend Standards

## Stack

- **Framework**: React + TypeScript (strict)
- **UI library**: HeroUI v3 (via `dune-ui/` wrappers)
- **Build**: Vite
- **Package manager**: `pnpm` — **never use npm or yarn in `web/`**
- **Auth**: Clerk (optional; keyed off `VITE_CLERK_PUBLISHABLE_KEY`)

## Canonical Reference Pattern

**`BasesTab.tsx` is the reference for new simple tabs.** Read it before creating a new tab.

### Minimal Tab Structure

```tsx
export default function FooTab() {
  const [data, setData] = useState<FooRow[]>([])
  const [loading, setLoading] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      setData(await api.foo.list())
    } catch (e) {
      toast.danger(`Failed: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  return (
    <Panel>
      <PageHeader title="Foo" onRefresh={load} loading={loading} />
      <DataTable columns={columns} rows={data} />
    </Panel>
  )
}
```

### Complex Tab Structure

For complex tabs, use a directory:

```
tabs/FooTab/
  index.tsx       — root component
  types.ts        — local types
  components/     — tab-local components
  modals/         — modal components
  views/          — sub-views (if needed)
```

## API Client

All backend calls go through `api/client.ts`. Import the `api` namespace for typed wrappers:

```ts
import { api, ApiError } from '../api/client'

const rows = await api.foo.list()
const detail = await api.foo.get(id)
```

- Do not use `fetch` directly in tab/component code
- The backend URL is runtime-configurable via `localStorage('dune_admin_backend')`
- Vite dev server proxies `/api` and WebSocket `/api/v1/logs/stream` → `:8080`

## Component Library (`dune-ui/`)

Import shared components from `../dune-ui` when a wrapper exists:

```ts
import {
  DataTable, Icon, PageHeader, Panel, SectionDivider, SectionLabel,
  InfoCard, Dropzone, SideNav,
} from '../dune-ui'
import type { Column } from '../dune-ui'
```

Use `@heroui/react` directly only for primitives not wrapped in `dune-ui`
(Button, Card, Chip, Spinner, toast, etc.).

`StatusChip` was removed — use inline `<Chip size="sm" variant="soft" color={...}>` instead.

## HeroUI v3 limitations

- `Select.Value` has no `placeholder` prop — use a sentinel item or keep native `<select>`
- HeroUI `Select` has no `<optgroup>` — keep native `<select>` for grouped option lists
- No equivalent for `<input list="...">` + `<datalist>` — keep native `<input>` with `bg-surface text-foreground border-border`

## Migration backlog

BattlegroupTab, StorageTab, DatabaseTab, LogsTab, BlueprintsTab still use raw HTML + inline styles. When refactoring any of these, follow the BasesTab pattern. Do not remove state/code — use `display: none` to hide features temporarily.

## Theming

All colours are CSS custom properties defined in `web/src/index.css`.
**Never use raw Tailwind colour utilities** (`bg-amber-900`, `text-zinc-400`, etc.).

Use semantic utilities:

```
bg-background       bg-surface        bg-surface-secondary
text-foreground     text-muted        text-accent
border-border
```

Inline `style={{ color: '#...' }}` overrides for colours are a sign the semantic token
approach wasn't used — fix them.

## Auth

`hasClerk = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY`

When absent, the app renders without auth (local dev). The `isSignedIn` prop gates
destructive features in certain tabs. Do not remove this gate.

**⚠️ This gate is cosmetic only.** `isSignedIn` hides/disables UI; it does **not** stop the
corresponding API call, and the backend currently performs **no** authentication (see
`.claude/rules/security.md` and `api-design.md`). Never treat a frontend gate as a security control.
dune-admin is a trusted-LAN operator tool that is deliberately not exposed to the internet, so this
is an accepted constraint — but it also means the UI gate gives no real protection.

## Accessibility (WCAG 2.2 AA — all surfaces)

Accessibility is a **required, non-negotiable** standard on every surface (admin and the planned
player view). Build it in as you go — do not defer it.

- **Semantic HTML + landmarks**: real `<nav>`/`<main>`/`<header>`/`<button>`; one `<main>` per page.
  Don't put click handlers on `<div>`s — use a `<button>`.
- **Keyboard operable**: every interactive element reachable and operable by keyboard; logical tab
  order; visible focus (`:focus-visible` ring); modals trap focus, close on `Esc`, and return focus
  to the trigger.
- **Accessible names**: every control has a label; icon-only buttons need `aria-label`. `DataTable`
  columns use proper headers/scope.
- **Never color-only**: status/meaning conveyed by text or icon in addition to colour.
- **Contrast**: meet AA (4.5:1 text, 3:1 UI/large text) — drive it through the semantic tokens in
  `index.css`, not ad-hoc colours (ties into Theming above).
- **Motion**: honour `prefers-reduced-motion`.
- Use the `frontend-accessibility` skill when building or reviewing a surface. Aim to enforce a11y
  at lint time (`eslint-plugin-jsx-a11y`) once it is wired up — see `.claude/rules/testing-web.md`.

## Responsive Design (mobile / tablet / desktop — all surfaces)

Every surface must work at mobile, tablet, and desktop widths. Design mobile-first.

- Use Tailwind breakpoints (`sm`/`md`/`lg`/`xl`); no fixed pixel widths that overflow small viewports.
- `DataTable`s must degrade gracefully on narrow screens (horizontal scroll or stacked rows), never
  truncate data off-screen.
- `SideNav` must collapse / become a drawer on small screens; modals must fit small viewports.
- Touch targets ≥ 44px.
- This applies to the whole migration backlog (BattlegroupTab, StorageTab, DatabaseTab, LogsTab,
  BlueprintsTab) as those tabs are refactored, and to all new tabs.
- The `ui-ux-pro-max` and `web-design-guidelines` skills can review responsive/UX quality.

## Testing & Lint (build it up as we go)

- `pnpm lint` (`--max-warnings 0`) and `pnpm build` (`tsc -b`) must be clean — the type-check is the
  gate.
- New/changed pure logic ships with a Vitest unit test; new/changed UI ships with a component test
  once Testing Library is wired up. Every bug fix lands a regression test that fails before the fix.
- Frontend testing conventions (Vitest + Testing Library + a11y assertions) live in
  `.claude/rules/testing-web.md`.

## Frontend Checklist

- [ ] Using `pnpm` (not npm/yarn)
- [ ] New tab follows `BasesTab.tsx` pattern
- [ ] All API calls go through `api/client.ts`
- [ ] `dune-ui/` wrappers used instead of direct `@heroui/react` where available
- [ ] Semantic colour tokens used (no raw Tailwind colours, no inline colour styles)
- [ ] TypeScript strict — no `any` unless absolutely necessary
- [ ] Keyboard-navigable + screen-reader-labelled (WCAG 2.2 AA)
- [ ] Layout verified at mobile, tablet, and desktop widths
- [ ] New/changed logic or UI has a test; bug fixes add a regression test
- [ ] `pnpm lint` passes (`cd web && pnpm lint`) and `pnpm build` type-checks clean
