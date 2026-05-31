# NumberField + Welcome Kits UI Alignment

**Date:** 2026-05-31
**Scope:** `web/src/`

## Summary

Three related improvements:

1. Add a `dune-ui/NumberInput` wrapper around HeroUI `NumberField` and sweep all 27 `type="number"` inputs in the codebase to use it.
2. Overhaul Welcome Kits (`WelcomePackageTab.tsx`) — replace raw `<select>` elements with HeroUI `Select`, adopt a search-then-add item flow identical to Give Items.
3. All qty/quality fields across StorageTab, GiveItemsView, GiveItemsModal, etc. move to `NumberInput`.

---

## Section 1: `dune-ui/NumberInput`

**File:** `web/src/dune-ui/NumberInput.tsx`
**Export:** added to `web/src/dune-ui/index.ts`

### Props

```ts
interface NumberInputProps {
  value: number
  onChange: (value: number) => void
  min?: number
  max?: number
  step?: number
  label?: string          // visible label above the field
  'aria-label'?: string   // when no visible label
  isDisabled?: boolean
  className?: string
  showButtons?: boolean   // default true
}
```

### Behaviour

- Wraps `NumberField` → `NumberField.Group` → `NumberField.Input`
- When `showButtons` is `true` (default): renders `NumberField.IncrementButton` and `NumberField.DecrementButton` flanking the input.
- When `showButtons` is `false`: renders input only (config-style fields where steppers add noise).
- `onChange` normalises `undefined` from HeroUI to `min ?? 0`.
- Uses semantic tokens: `bg-surface`, `border-border`, `text-foreground`, `--radius`.

### `showButtons` convention

| Context | `showButtons` |
|---|---|
| Item qty / quality (small discrete values, +/- useful) | `true` |
| Config fields (db port, scan interval, price limits, etc.) | `false` |

---

## Section 2: Welcome Kits overhaul

**File:** `web/src/tabs/WelcomePackageTab.tsx`

### 2a — Dropdowns → HeroUI `Select`

Replace both raw `<select>` elements:

- **Active version** select → `Select` with `Select.Trigger` / `Select.Value` / `Select.Indicator` / `Select.Popover` / `ListBox` / `ListBox.Item`. Includes "— none —" option as an empty-string item.
- **Editing version** select → same pattern.

Reference: the pack selector in `GiveItemsView.tsx`.

The "Enabled" native checkbox and "New version name" text input are unchanged.

### 2b — Scan interval → `NumberInput`

```tsx
// before
<input className={INPUT_CLS} type="number" min={5} value={scanSecs} onChange={...} />

// after
<NumberInput min={5} showButtons={false} label="Scan interval (sec)" value={scanSecs} onChange={setScanSecs} />
```

Remove `INPUT_CLS` constant (no longer needed once all raw inputs are replaced).

### 2c — Item list → search-then-add

#### State additions

```ts
const [templates, setTemplates] = useState<{ id: string, name: string }[]>([])
const [addQuery, setAddQuery] = useState('')
const [addSelected, setAddSelected] = useState('')
const [addQty, setAddQty] = useState(1)
const [addQuality, setAddQuality] = useState(0)
```

Load templates on mount alongside the existing config load:

```ts
api.players.templates().then(setTemplates).catch(() => {})
```

#### Add row (top of item list)

```
[ SearchField + dropdown ]  [ NumberInput qty ]  [ NumberInput quality ]  [ + Add button ]
```

- `SearchField` filters `templates` by id/name (max 100, same as Give Items).
- Dropdown appears below the search field when `filtered.length > 0`, styled identically to Give Items (`absolute z-50`, `bg-surface`, `border-border`).
- Picking a template sets `addSelected` + updates `addQuery` to `"${id}  —  ${name}"`.
- "+ Add" appends `{ template: addSelected, qty: addQty, quality: addQuality }` to items, resets the add-row state.
- "+ Add" is disabled when `addSelected` is empty.

#### Item list (below add row)

Each row:

```
[ font-mono template name (read-only) ]  [ NumberInput qty ]  [ NumberInput quality ]  [ trash Button ]
```

- Qty/quality `NumberInput` changes call the existing `setItem(i, patch)` immediately.
- Trash button calls `removeItem(i)`.
- "Items in V1 (1)" header + the old "+ Add item" button at the top-right are removed; the add row replaces both.

---

## Section 3: Full `NumberInput` sweep

Replace every remaining `type="number"` input. No functional changes — only the component used.

| File | Fields | `showButtons` |
|---|---|---|
| `tabs/StorageTab.tsx` | qty + quality (add row + staged list) | `true` |
| `tabs/PlayersTab/views/GiveItemsView.tsx` | qty + quality (add row + staged list) — replaces `InputGroup` | `true` |
| `tabs/PlayersTab/modals/GiveItemsModal.tsx` | qty + quality (add row + staged list) | `true` |
| `tabs/PlayersTab/views/ActionsView.tsx` | single quantity field | `true` |
| `tabs/PlayersTab/modals/PlayerActionsModal.tsx` | single quantity field | `true` |
| `tabs/MarketTab/bot/BotConfigEditor.tsx` | 6 config fields (prices, intervals, limits) | `false` |
| `tabs/BattlegroupTab/index.tsx` | 2 config fields | `false` |
| `tabs/DatabaseTab.tsx` | 1 field (query limit) | `false` |
| `tabs/ServerSettingsTab.tsx` | 1 config field | `false` |
| `components/SettingsConfigForm.tsx` | db_port + scrip_currency | `false` |

For files where `type="number"` inputs are already inside HeroUI `InputGroup` (e.g. `GiveItemsView` qty/quality in the add row), the `InputGroup` + `InputGroup.Prefix` wrapper is removed and replaced with `NumberInput`. In compact row layouts (add row, staged list rows), use `aria-label` instead of `label` to avoid adding a visible label above each small field.

---

## Implementation Order

1. Create `dune-ui/NumberInput.tsx` and export it.
2. Welcome Kits overhaul (2a selects → 2b scan interval → 2c item search-then-add).
3. Full sweep of remaining files (can be done in parallel per file).
4. Run `pnpm lint` + `make verify`.

---

## Out of Scope

- i18n of NumberInput label strings in Welcome Kits (Welcome Kits tab is not yet translated).
- Backend changes (no API changes required).
- Changing the Give Items search dropdown to a formal HeroUI `ComboBox` (out of scope; existing pattern works).
