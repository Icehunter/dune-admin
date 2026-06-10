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
  ActionBar, ConfirmDialog, DataTable, Dropzone, FieldInput, FieldSelect,
  Icon, InfoCard, LoadingState, NumberInput, PageHeader, Panel,
  SectionDivider, SectionLabel, SideNav, TimeInput,
} from '../dune-ui'
import type { Column } from '../dune-ui'
```

Use `@heroui/react` directly only for primitives not wrapped in `dune-ui`
(Button, Card, Chip, Spinner, toast, etc.).

`StatusChip` was removed — use inline `<Chip size="sm" variant="soft" color={...}>` instead.

### `FieldInput`

Wraps HeroUI `Input` with `size="sm"`. Use for all text, number, password, email, and url inputs.

```tsx
<FieldInput value={val} onChange={setVal} placeholder="…" aria-label="…" />
<FieldInput type="number" value={num} onChange={setNum} className="w-32" />
<FieldInput value={path} onChange={setPath} classNames={{ input: 'font-mono' }} />
```

### `FieldSelect`

Wraps HeroUI `Select` + `ListBox` for small, fixed option sets (booleans, enums up to ~20 items).

```tsx
<FieldSelect value={val} onChange={setVal} options={['true', 'false']} />
<FieldSelect value={mode} onChange={setMode} options={['A', 'B', 'C']} className="w-40" />
```

For large option sets, `FieldSelect` (and HeroUI `Select` directly) still work — use them
for visual consistency. `TimezoneSelect` in `components/` wraps HeroUI `Select` for the
~400-entry IANA list with a host-local sentinel.

### `TimeInput`

Wraps HeroUI `TimeField` with 24-hour segmented input. Accepts and emits `"HH:MM"` strings.

```tsx
<TimeInput value={rule.time} onChange={(v) => setRuleTime(i, v)} ariaLabel="time" />
```

### Checkboxes and Toggles

Use HeroUI's `Checkbox` and `Switch` from `@heroui/react` — never native `<input type="checkbox">`:

```tsx
import { Checkbox, Switch } from '@heroui/react'

// Toggle (on/off) — use Switch
<Switch isSelected={enabled} onChange={setEnabled} size="sm">{t('enable')}</Switch>

// Checkbox (filter/option) — use Checkbox (no size prop)
<Checkbox isSelected={isOn} onChange={setOn}>{t('label')}</Checkbox>

// Checkbox with indeterminate state
<Checkbox isSelected={allOn} isIndeterminate={!allOn && anyOn} onChange={handleChange} />
```

## HeroUI v3 limitations

- HeroUI `Select` has no `<optgroup>` — keep native `<select>` for grouped option lists
- No equivalent for `<input list="...">` + `<datalist>` — keep native `<input>` with `bg-surface text-foreground border border-border rounded`

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

## Frontend Checklist

- [ ] Using `pnpm` (not npm/yarn)
- [ ] New tab follows `BasesTab.tsx` pattern
- [ ] All API calls go through `api/client.ts`
- [ ] `dune-ui/` wrappers used instead of direct `@heroui/react` where available
- [ ] Semantic colour tokens used (no raw Tailwind colours, no inline colour styles)
- [ ] TypeScript strict — no `any` unless absolutely necessary
- [ ] `pnpm lint` passes (`cd web && pnpm lint`)
