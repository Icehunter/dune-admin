# NumberField + Welcome Kits UI Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `dune-ui/NumberInput` wrapper around HeroUI NumberField, overhaul Welcome Kits UI (HeroUI selects + search-then-add item flow identical to Give Items), and sweep all `type="number"` inputs across the codebase to use `NumberInput`.

**Architecture:** A single themed `NumberInput` component wraps HeroUI `NumberField` with a `showButtons` flag — `true` for item qty/quality (steppers useful), `false` for config fields (port numbers, intervals, percentages). Welcome Kits adopts the same search-then-add pattern as `GiveItemsView`. All other files are mechanical replacements of `<input type="number">` / `<Input type="number">` / `InputGroup` wrappers.

**Tech Stack:** React + TypeScript, HeroUI v3 (`NumberField`, `Select`, `SearchField`, `ListBox`), dune-ui component library, `pnpm lint` for verification.

---

## File Map

| Action | File | Change |
|---|---|---|
| **Create** | `web/src/dune-ui/NumberInput.tsx` | New component |
| **Modify** | `web/src/dune-ui/index.ts` | Export `NumberInput` |
| **Modify** | `web/src/tabs/WelcomePackageTab.tsx` | Selects + scan interval + search-then-add items |
| **Modify** | `web/src/tabs/PlayersTab/views/GiveItemsView.tsx` | Replace `InputGroup` qty/quality + staged `Input` qty/quality |
| **Modify** | `web/src/tabs/PlayersTab/modals/GiveItemsModal.tsx` | Same pattern as GiveItemsView |
| **Modify** | `web/src/tabs/StorageTab.tsx` | Same pattern as GiveItemsView |
| **Modify** | `web/src/tabs/PlayersTab/views/ActionsView.tsx` | Replace `numInput` helper |
| **Modify** | `web/src/tabs/PlayersTab/modals/PlayerActionsModal.tsx` | Replace `numInput` helper |
| **Modify** | `web/src/tabs/MarketTab/bot/BotConfigEditor.tsx` | 6 config number fields |
| **Modify** | `web/src/tabs/BattlegroupTab/index.tsx` | 2 duration fields |
| **Modify** | `web/src/tabs/DatabaseTab.tsx` | 1 limit field |
| **Modify** | `web/src/tabs/ServerSettingsTab.tsx` | 1 numeric setting field |
| **Modify** | `web/src/components/SettingsConfigForm.tsx` | db_port + scrip_currency |

---

## Task 1: Create `dune-ui/NumberInput` component

**Files:**

- Create: `web/src/dune-ui/NumberInput.tsx`
- Modify: `web/src/dune-ui/index.ts`

- [ ] **Step 1: Create the component**

```tsx
// web/src/dune-ui/NumberInput.tsx
import { Label, NumberField } from '@heroui/react'

interface NumberInputProps {
  value: number
  onChange: (value: number) => void
  min?: number
  max?: number
  step?: number
  label?: string
  'aria-label'?: string
  isDisabled?: boolean
  className?: string
  showButtons?: boolean
}

export function NumberInput({
  value,
  onChange,
  min,
  max,
  step = 1,
  label,
  'aria-label': ariaLabel,
  isDisabled,
  className,
  showButtons = true,
}: NumberInputProps) {
  return (
    <NumberField
      value={value}
      onChange={(v) => onChange(v ?? min ?? 0)}
      minValue={min}
      maxValue={max}
      step={step}
      isDisabled={isDisabled}
      aria-label={ariaLabel ?? label}
      className={className}
    >
      {label && <Label className="text-xs text-muted">{label}</Label>}
      <NumberField.Group>
        {showButtons && <NumberField.DecrementButton />}
        <NumberField.Input />
        {showButtons && <NumberField.IncrementButton />}
      </NumberField.Group>
    </NumberField>
  )
}
```

- [ ] **Step 2: Export from dune-ui**

In `web/src/dune-ui/index.ts`, add after the existing `ConfirmDialog` export:

```ts
export { NumberInput } from './NumberInput'
```

- [ ] **Step 3: Lint**

```bash
cd web && pnpm lint
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/dune-ui/NumberInput.tsx web/src/dune-ui/index.ts
git commit -m "feat(dune-ui): add NumberInput wrapper around HeroUI NumberField"
```

---

## Task 2: Welcome Kits — HeroUI Select for dropdowns + NumberInput for scan interval

**Files:**

- Modify: `web/src/tabs/WelcomePackageTab.tsx`

The file currently imports `{ Button, Spinner, toast }` from `@heroui/react`. The `Field` helper and `INPUT_CLS` constant are local.

- [ ] **Step 1: Update imports**

Replace the existing `@heroui/react` import line:

```tsx
// before
import { Button, Spinner, toast } from '@heroui/react'

// after
import { Button, ListBox, Select, Spinner, toast } from '@heroui/react'
```

Add `NumberInput` to the dune-ui import:

```tsx
// before
import { DataTable, Icon, PageHeader, Panel, SectionLabel, type Column } from '../dune-ui'

// after
import { DataTable, Icon, NumberInput, PageHeader, Panel, SectionLabel, type Column } from '../dune-ui'
```

Delete the `INPUT_CLS` constant (top of file, line 7):

```tsx
// remove this line entirely:
const INPUT_CLS = 'bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground'
```

- [ ] **Step 2: Replace "Active version" select**

Find and replace in the Configuration panel (around line 184–193):

```tsx
// before
<div className="flex flex-col gap-1">
  <label className="text-xs text-muted">Active version</label>
  <select
    className={`${INPUT_CLS} w-48`}
    value={activeVersion}
    onChange={(e) => setActiveVersion(e.target.value)}
  >
    <option value="">— none —</option>
    {packages.map((p) => (
      <option key={p.version} value={p.version}>{p.version}</option>
    ))}
  </select>
</div>

// after
<div className="flex flex-col gap-1">
  <label className="text-xs text-muted">Active version</label>
  <Select
    aria-label="Active version"
    selectedKey={activeVersion || null}
    onSelectionChange={(k) => setActiveVersion(k ? String(k) : '')}
    className="w-48"
  >
    <Select.Trigger>
      <Select.Value>{!activeVersion ? '— none —' : activeVersion}</Select.Value>
      <Select.Indicator />
    </Select.Trigger>
    <Select.Popover>
      <ListBox>
        <ListBox.Item key="_none" id="" textValue="— none —">
          — none —
          <ListBox.ItemIndicator />
        </ListBox.Item>
        {packages.map((p) => (
          <ListBox.Item key={p.version} id={p.version} textValue={p.version}>
            {p.version}
            <ListBox.ItemIndicator />
          </ListBox.Item>
        ))}
      </ListBox>
    </Select.Popover>
  </Select>
</div>
```

- [ ] **Step 3: Replace "Scan interval" number input**

Find and replace (around line 195–205):

```tsx
// before
<div className="flex flex-col gap-1">
  <label className="text-xs text-muted">Scan interval (sec)</label>
  <input
    className={`${INPUT_CLS} w-28`}
    type="number"
    min={5}
    value={scanSecs}
    onChange={(e) => setScanSecs(Number(e.target.value))}
  />
</div>

// after
<NumberInput
  label="Scan interval (sec)"
  min={5}
  showButtons={false}
  value={scanSecs}
  onChange={setScanSecs}
  className="w-28"
/>
```

- [ ] **Step 4: Replace "Editing version" select**

Find and replace in the Packages panel (around line 214–227):

```tsx
// before
<Field label="Editing version">
  <select
    className={`${INPUT_CLS} w-44`}
    value={selected}
    onChange={(e) => setSelected(e.target.value)}
  >
    <option value="">— select —</option>
    {packages.map((p) => (
      <option key={p.version} value={p.version}>
        {p.version}
        {p.version === activeVersion ? ' (active)' : ''}
      </option>
    ))}
  </select>
</Field>

// after
<Field label="Editing version">
  <Select
    aria-label="Editing version"
    selectedKey={selected || null}
    onSelectionChange={(k) => setSelected(k ? String(k) : '')}
    className="w-44"
  >
    <Select.Trigger>
      <Select.Value>{!selected ? '— select —' : selected + (selected === activeVersion ? ' (active)' : '')}</Select.Value>
      <Select.Indicator />
    </Select.Trigger>
    <Select.Popover>
      <ListBox>
        <ListBox.Item key="_none" id="" textValue="— select —">
          — select —
          <ListBox.ItemIndicator />
        </ListBox.Item>
        {packages.map((p) => (
          <ListBox.Item key={p.version} id={p.version} textValue={p.version}>
            {p.version}
            {p.version === activeVersion ? ' (active)' : ''}
            <ListBox.ItemIndicator />
          </ListBox.Item>
        ))}
      </ListBox>
    </Select.Popover>
  </Select>
</Field>
```

- [ ] **Step 5: Lint**

```bash
cd web && pnpm lint
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/tabs/WelcomePackageTab.tsx
git commit -m "feat(welcome-kits): replace raw selects with HeroUI Select + NumberInput for scan interval"
```

---

## Task 3: Welcome Kits — search-then-add item flow

**Files:**

- Modify: `web/src/tabs/WelcomePackageTab.tsx`

- [ ] **Step 1: Add template state and imports**

Add `SearchField` to the `@heroui/react` import:

```tsx
import { Button, ListBox, SearchField, Select, Spinner, toast } from '@heroui/react'
```

Add `useMemo` to the React import:

```tsx
import { useState, useEffect, useCallback, useMemo } from 'react'
```

- [ ] **Step 2: Add add-row state variables**

After the existing state declarations (around line 38, after `const [newName, setNewName] = useState('')`), add:

```tsx
const [templates, setTemplates] = useState<{ id: string, name: string }[]>([])
const [addQuery, setAddQuery] = useState('')
const [addSelected, setAddSelected] = useState('')
const [addQty, setAddQty] = useState(1)
const [addQuality, setAddQuality] = useState(0)
```

- [ ] **Step 3: Load templates on mount**

Add a separate effect after the existing `useEffect` that calls `load()`:

```tsx
useEffect(() => {
  api.players.templates().then(setTemplates).catch(() => {})
}, [])
```

- [ ] **Step 4: Add filtered templates memo and pick/add helpers**

Replace the existing `addItem` function with a new `addItem` that uses `addSelected`. Add `addFiltered`, `pickTemplate`, and the reset inside `addItem`:

```tsx
// replace:
const addItem = () => setItems([...items, { template: '', qty: 1, quality: 0 }])

// with:
const addFiltered = useMemo(() => {
  if (!addQuery) return []
  const q = addQuery.toLowerCase()
  return templates
    .filter((t) => t.id.toLowerCase().includes(q) || t.name.toLowerCase().includes(q))
    .slice(0, 100)
}, [templates, addQuery])

const pickTemplate = (tpl: { id: string, name: string }) => {
  setAddSelected(tpl.id)
  setAddQuery(tpl.name ? `${tpl.id}  —  ${tpl.name}` : tpl.id)
}

const addItem = () => {
  if (!addSelected) return
  setItems([...items, { template: addSelected, qty: addQty, quality: addQuality }])
  setAddQuery('')
  setAddSelected('')
  setAddQty(1)
  setAddQuality(0)
}
```

- [ ] **Step 5: Replace the item list JSX**

Find and replace the entire block starting from `<div className="mt-3">` inside the `{!selected ? ... : ...}` ternary (around lines 259–312). Replace it with:

```tsx
<div className="mt-3 max-w-2xl">
  <div className="text-xs text-muted mb-2">
    Items in
    {' '}
    <span className="text-foreground">{selected}</span>
    {' '}
    (
    {items.length}
    )
  </div>

  {/* Add row */}
  <div className="flex items-center gap-2 mb-3">
    <div className="relative flex-1">
      <SearchField
        value={addQuery}
        onChange={(v) => {
          setAddQuery(v)
          setAddSelected('')
        }}
        className="w-full"
      >
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input placeholder="Search item templates…" />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>
      {addFiltered.length > 0 && (
        <div className="absolute z-50 w-full mt-1 rounded-[var(--radius)] border border-border bg-surface overflow-y-auto max-h-52">
          {addFiltered.map((tpl) => (
            <div
              key={tpl.id}
              className="px-3 py-1.5 text-xs cursor-pointer hover:bg-surface-hover"
              onClick={() => pickTemplate(tpl)}
            >
              <span className="font-mono">{tpl.id}</span>
              {tpl.name
                ? (
                    <span className="text-muted">
                      {' — '}
                      {tpl.name}
                    </span>
                  )
                : null}
            </div>
          ))}
        </div>
      )}
    </div>
    <NumberInput
      aria-label="Qty"
      min={1}
      value={addQty}
      onChange={setAddQty}
      className="w-24 shrink-0"
    />
    <NumberInput
      aria-label="Quality"
      min={0}
      value={addQuality}
      onChange={setAddQuality}
      className="w-24 shrink-0"
    />
    <Button size="sm" onPress={addItem} isDisabled={!addSelected} className="shrink-0">
      <Icon name="plus" />
      {' '}
      Add
    </Button>
  </div>

  {/* Item list */}
  <div className="flex flex-col gap-2">
    {items.length === 0 && (
      <p className="text-xs text-muted">No items in this version yet.</p>
    )}
    {items.map((it, i) => (
      <div
        key={i}
        className="flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius)] text-xs bg-surface border border-border"
      >
        <span className="flex-1 min-w-0 truncate font-mono text-foreground">{it.template}</span>
        <NumberInput
          aria-label="Qty"
          min={1}
          value={it.qty}
          onChange={(v) => setItem(i, { qty: v })}
          className="w-24 shrink-0"
        />
        <NumberInput
          aria-label="Quality"
          min={0}
          value={it.quality}
          onChange={(v) => setItem(i, { quality: v })}
          className="w-24 shrink-0"
        />
        <Button size="sm" variant="ghost" onPress={() => removeItem(i)} aria-label="Remove item">
          <Icon name="trash-2" />
        </Button>
      </div>
    ))}
  </div>
</div>
```

- [ ] **Step 6: Lint**

```bash
cd web && pnpm lint
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add web/src/tabs/WelcomePackageTab.tsx
git commit -m "feat(welcome-kits): search-then-add item flow matching Give Items UI"
```

---

## Task 4: GiveItemsView — NumberInput sweep

**Files:**

- Modify: `web/src/tabs/PlayersTab/views/GiveItemsView.tsx`

The file currently imports `InputGroup` from `@heroui/react` and uses it for qty/quality in the add row. The staged list uses `Input` with `type="number"`.

- [ ] **Step 1: Update imports**

```tsx
// before
import {
  Button, Header, Input, InputGroup, ListBox, SearchField, Select, Separator, Spinner, TextField, toast,
} from '@heroui/react'

// after
import {
  Button, Header, Input, ListBox, SearchField, Select, Separator, Spinner, toast,
} from '@heroui/react'
```

Add `NumberInput` to the dune-ui import:

```tsx
// before
import { Icon } from '../../../dune-ui'

// after
import { Icon, NumberInput } from '../../../dune-ui'
```

- [ ] **Step 2: Replace qty/quality add-row (lines ~204–227)**

```tsx
// before
<TextField className="w-32 shrink-0" aria-label={t('players.give.qty')}>
  <InputGroup>
    <InputGroup.Prefix>{t('players.give.qty')}</InputGroup.Prefix>
    <InputGroup.Input
      className="pl-2"
      type="number"
      min={1}
      value={qty}
      onChange={(e) => setQty(Math.max(1, parseInt(e.target.value) || 1))}
    />
  </InputGroup>
</TextField>
<TextField className="w-40 shrink-0" aria-label={t('players.give.quality')}>
  <InputGroup>
    <InputGroup.Prefix>{t('players.give.quality')}</InputGroup.Prefix>
    <InputGroup.Input
      className="pl-2"
      type="number"
      min={0}
      value={quality}
      onChange={(e) => setQuality(Math.max(0, parseInt(e.target.value) || 0))}
    />
  </InputGroup>
</TextField>

// after
<NumberInput
  aria-label={t('players.give.qty')}
  min={1}
  value={qty}
  onChange={setQty}
  className="w-32 shrink-0"
/>
<NumberInput
  aria-label={t('players.give.quality')}
  min={0}
  value={quality}
  onChange={setQuality}
  className="w-40 shrink-0"
/>
```

- [ ] **Step 3: Replace staged list qty/quality (lines ~250–278)**

```tsx
// before
<Input
  type="number"
  min={1}
  value={item.qty}
  onChange={(e) => updateStaged(idx, 'qty', Math.max(1, parseInt(e.target.value) || 1))}
  aria-label={`${t('players.give.qty')} for ${item.template}`}
  className="w-20 text-center"
/>
<Input
  type="number"
  min={0}
  value={item.quality}
  onChange={(e) => updateStaged(idx, 'quality', Math.max(0, parseInt(e.target.value) || 0))}
  aria-label={`${t('players.give.quality')} for ${item.template}`}
  className="w-20 text-center"
/>

// after
<NumberInput
  aria-label={`${t('players.give.qty')} for ${item.template}`}
  min={1}
  value={item.qty}
  onChange={(v) => updateStaged(idx, 'qty', v)}
  className="w-20"
/>
<NumberInput
  aria-label={`${t('players.give.quality')} for ${item.template}`}
  min={0}
  value={item.quality}
  onChange={(v) => updateStaged(idx, 'quality', v)}
  className="w-20"
/>
```

- [ ] **Step 4: Remove unused `TextField` import if no longer used**

Check if `TextField` is still referenced elsewhere in the file:

```bash
grep -n 'TextField' web/src/tabs/PlayersTab/views/GiveItemsView.tsx
```

If only in the old lines (now removed), also remove `TextField` from the `@heroui/react` import. If still used (it wraps the SearchField), keep it.

- [ ] **Step 5: Lint**

```bash
cd web && pnpm lint
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/tabs/PlayersTab/views/GiveItemsView.tsx
git commit -m "feat(players): NumberInput for Give Items qty/quality fields"
```

---

## Task 5: GiveItemsModal — NumberInput sweep

**Files:**

- Modify: `web/src/tabs/PlayersTab/modals/GiveItemsModal.tsx`

This file has the same qty/quality pattern as GiveItemsView (lines 218–240 add row, 264–279 staged list).

- [ ] **Step 1: Update imports — remove `InputGroup`, add `NumberInput` from dune-ui**

Find the `@heroui/react` import and remove `InputGroup` and `TextField`. Find the dune-ui import and add `NumberInput`.

```bash
# Verify current imports
grep -n 'import' web/src/tabs/PlayersTab/modals/GiveItemsModal.tsx | head -10
```

Add `NumberInput` to whatever dune-ui imports exist. Remove `InputGroup`, `TextField` from `@heroui/react` (check they're not used elsewhere first with `grep`).

- [ ] **Step 2: Replace add-row qty/quality**

Same replacement as GiveItemsView Task 4 Step 2, applied to lines ~218–240 of GiveItemsModal:

```tsx
// before
<TextField className="w-32 shrink-0" aria-label={t('players.give.qty')}>
  <InputGroup>
    <InputGroup.Prefix>{t('players.give.qty')}</InputGroup.Prefix>
    <InputGroup.Input
      className="pl-2"
      type="number"
      min={1}
      value={qty}
      onChange={(e) => setQty(Math.max(1, parseInt(e.target.value) || 1))}
    />
  </InputGroup>
</TextField>
<TextField className="w-40 shrink-0" aria-label={t('players.give.quality')}>
  <InputGroup>
    <InputGroup.Prefix>{t('players.give.quality')}</InputGroup.Prefix>
    <InputGroup.Input
      className="pl-2"
      type="number"
      min={0}
      value={quality}
      onChange={(e) => setQuality(Math.max(0, parseInt(e.target.value) || 0))}
    />
  </InputGroup>
</TextField>

// after
<NumberInput
  aria-label={t('players.give.qty')}
  min={1}
  value={qty}
  onChange={setQty}
  className="w-32 shrink-0"
/>
<NumberInput
  aria-label={t('players.give.quality')}
  min={0}
  value={quality}
  onChange={setQuality}
  className="w-40 shrink-0"
/>
```

- [ ] **Step 3: Replace staged list qty/quality (lines ~264–279)**

```tsx
// before
<Input
  type="number"
  min={1}
  value={item.qty}
  onChange={(e) => updateStaged(idx, 'qty', Math.max(1, parseInt(e.target.value) || 1))}
  aria-label={`${t('players.give.qty')} for ${item.template}`}
  className="w-20 text-center"
/>
<Input
  type="number"
  min={0}
  value={item.quality}
  onChange={(e) => updateStaged(idx, 'quality', Math.max(0, parseInt(e.target.value) || 0))}
  aria-label={`${t('players.give.quality')} for ${item.template}`}
  className="w-20 text-center"
/>

// after
<NumberInput
  aria-label={`${t('players.give.qty')} for ${item.template}`}
  min={1}
  value={item.qty}
  onChange={(v) => updateStaged(idx, 'qty', v)}
  className="w-20"
/>
<NumberInput
  aria-label={`${t('players.give.quality')} for ${item.template}`}
  min={0}
  value={item.quality}
  onChange={(v) => updateStaged(idx, 'quality', v)}
  className="w-20"
/>
```

- [ ] **Step 4: Lint**

```bash
cd web && pnpm lint
```

- [ ] **Step 5: Commit**

```bash
git add web/src/tabs/PlayersTab/modals/GiveItemsModal.tsx
git commit -m "feat(players): NumberInput for Give Items Modal qty/quality fields"
```

---

## Task 6: StorageTab — NumberInput sweep

**Files:**

- Modify: `web/src/tabs/StorageTab.tsx`

Same pattern as GiveItemsView: `InputGroup` on add row (lines ~415–438), `Input type="number"` on staged list (lines ~461–476).

- [ ] **Step 1: Update imports**

Add `NumberInput` to whatever dune-ui import exists. Remove `InputGroup` and `TextField` from `@heroui/react` after verifying they are not used elsewhere in the file.

```bash
grep -n 'InputGroup\|TextField' web/src/tabs/StorageTab.tsx
```

- [ ] **Step 2: Replace add-row qty/quality (lines ~415–438)**

```tsx
// before
<TextField className="w-32 shrink-0" aria-label="Quantity">
  <InputGroup>
    <InputGroup.Prefix>{t('storage.addModal.qtyLabel')}</InputGroup.Prefix>
    <InputGroup.Input
      className="pl-2"
      type="number"
      min={1}
      value={qty}
      onChange={(e) => setQty(Math.max(1, parseInt(e.target.value) || 1))}
    />
  </InputGroup>
</TextField>
<TextField className="w-40 shrink-0" aria-label="Quality">
  <InputGroup>
    <InputGroup.Prefix>{t('storage.addModal.qualityLabel')}</InputGroup.Prefix>
    <InputGroup.Input
      className="pl-2"
      type="number"
      min={0}
      value={quality}
      onChange={(e) => setQuality(Math.max(0, parseInt(e.target.value) || 0))}
    />
  </InputGroup>
</TextField>

// after
<NumberInput
  aria-label={t('storage.addModal.qtyLabel')}
  min={1}
  value={qty}
  onChange={setQty}
  className="w-32 shrink-0"
/>
<NumberInput
  aria-label={t('storage.addModal.qualityLabel')}
  min={0}
  value={quality}
  onChange={setQuality}
  className="w-40 shrink-0"
/>
```

- [ ] **Step 3: Replace staged list qty/quality (lines ~461–476)**

```tsx
// before
<Input
  type="number"
  min={1}
  value={item.qty}
  onChange={(e) => updateStaged(idx, 'qty', Math.max(1, parseInt(e.target.value) || 1))}
  aria-label={`Qty for ${item.template}`}
  className="w-20 text-center"
/>
<Input
  type="number"
  min={0}
  value={item.quality}
  onChange={(e) => updateStaged(idx, 'quality', Math.max(0, parseInt(e.target.value) || 0))}
  aria-label={`Quality for ${item.template}`}
  className="w-20 text-center"
/>

// after
<NumberInput
  aria-label={`Qty for ${item.template}`}
  min={1}
  value={item.qty}
  onChange={(v) => updateStaged(idx, 'qty', v)}
  className="w-20"
/>
<NumberInput
  aria-label={`Quality for ${item.template}`}
  min={0}
  value={item.quality}
  onChange={(v) => updateStaged(idx, 'quality', v)}
  className="w-20"
/>
```

- [ ] **Step 4: Lint**

```bash
cd web && pnpm lint
```

- [ ] **Step 5: Commit**

```bash
git add web/src/tabs/StorageTab.tsx
git commit -m "feat(storage): NumberInput for Storage qty/quality fields"
```

---

## Task 7: ActionsView + PlayerActionsModal — NumberInput in `numInput` helper

**Files:**

- Modify: `web/src/tabs/PlayersTab/views/ActionsView.tsx`
- Modify: `web/src/tabs/PlayersTab/modals/PlayerActionsModal.tsx`

Both files define an identical `numInput` helper (lines ~400 and ~378 respectively) that returns an `<Input type="number">`.

- [ ] **Step 1: Update ActionsView imports**

Add `NumberInput` to the dune-ui import. Verify `Input` is not used elsewhere before removing it from `@heroui/react`.

```bash
grep -n 'Input[^G]' web/src/tabs/PlayersTab/views/ActionsView.tsx | grep -v InputGroup | grep -v import
```

- [ ] **Step 2: Replace `numInput` helper in ActionsView**

```tsx
// before (lines ~400–410)
const numInput = (val: number, set: (v: number) => void, min = 1, max = 9999999) => (
  <Input
    type="number"
    min={min}
    max={max}
    value={val}
    onChange={(e) => set(Math.max(min, Math.min(max, parseInt(e.target.value) || min)))}
    aria-label="number"
    className="w-28"
  />
)

// after
const numInput = (val: number, set: (v: number) => void, min = 1, max = 9999999) => (
  <NumberInput
    aria-label="number"
    min={min}
    max={max}
    value={val}
    onChange={(v) => set(Math.max(min, Math.min(max, v)))}
    className="w-28"
  />
)
```

- [ ] **Step 3: Repeat for PlayerActionsModal**

Apply the identical change to `PlayerActionsModal.tsx` (lines ~378–388). Update imports the same way (add `NumberInput` from dune-ui, remove bare `Input` from `@heroui/react` if unused).

- [ ] **Step 4: Lint**

```bash
cd web && pnpm lint
```

- [ ] **Step 5: Commit**

```bash
git add web/src/tabs/PlayersTab/views/ActionsView.tsx web/src/tabs/PlayersTab/modals/PlayerActionsModal.tsx
git commit -m "feat(players): NumberInput in Actions numInput helper"
```

---

## Task 8: BotConfigEditor — NumberInput sweep

**Files:**

- Modify: `web/src/tabs/MarketTab/bot/BotConfigEditor.tsx`

Six `type="number"` inputs: `max_buys` (line 108), `listings_per_grade` (line 116), `buyPct` (line 125), `grade_multipliers` (line 158), `rarity_multipliers` (line 177), `vendor_multipliers` (line 197). Note: `list_interval` and `buy_interval` do NOT have `type="number"` — they are plain text (cron strings). Do NOT change those.

All use `className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"` (raw styling). Switch to `NumberInput showButtons={false}`.

- [ ] **Step 1: Update imports**

Add `NumberInput` to the dune-ui import (or create one if it only imports from `@heroui/react`):

```bash
grep -n 'from.*dune-ui' web/src/tabs/MarketTab/bot/BotConfigEditor.tsx
```

- [ ] **Step 2: Replace `max_buys` field (around line 106–111)**

```tsx
// before
<input
  className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
  type="number"
  value={draft.max_buys}
  onChange={(e) => set('max_buys', Number(e.target.value))}
/>

// after
<NumberInput
  aria-label={t('market.bot.configEditor.maxBuysPerTick')}
  value={draft.max_buys}
  onChange={(v) => set('max_buys', v)}
  showButtons={false}
  className="w-full"
/>
```

- [ ] **Step 3: Replace `listings_per_grade` field (around line 114–119)**

```tsx
// before
<input
  className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
  type="number"
  value={draft.listings_per_grade}
  onChange={(e) => set('listings_per_grade', Number(e.target.value))}
/>

// after
<NumberInput
  aria-label={t('market.bot.configEditor.listingsPerGrade')}
  value={draft.listings_per_grade}
  onChange={(v) => set('listings_per_grade', v)}
  showButtons={false}
  className="w-full"
/>
```

- [ ] **Step 4: Replace `buyPct` field (around line 123–131)**

```tsx
// before
<div className="flex items-center gap-2">
  <input
    className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-20"
    type="number"
    min={1}
    max={200}
    step={1}
    value={buyPct}
    onChange={(e) => setBuyPct(Number(e.target.value))}
  />
  <span className="text-sm text-muted">%</span>
</div>

// after
<div className="flex items-center gap-2">
  <NumberInput
    aria-label={t('market.bot.configEditor.buyThreshold')}
    min={1}
    max={200}
    value={buyPct}
    onChange={setBuyPct}
    showButtons={false}
    className="w-20"
  />
  <span className="text-sm text-muted">%</span>
</div>
```

- [ ] **Step 5: Replace `grade_multipliers` fields (around line 154–165)**

```tsx
// before
<input
  className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-24"
  type="number"
  step="0.05"
  min="0.01"
  value={mult}
  onChange={(e) => setGrade(i, Number(e.target.value))}
/>

// after
<NumberInput
  aria-label={GRADE_LABELS[i] ?? `Grade ${i}`}
  step={0.05}
  min={0.01}
  value={mult}
  onChange={(v) => setGrade(i, v)}
  showButtons={false}
  className="w-24"
/>
```

- [ ] **Step 6: Replace `rarity_multipliers` fields (around line 173–182)**

```tsx
// before
<input
  className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-24"
  type="number"
  step="0.1"
  min="0.01"
  value={mult}
  onChange={(e) => setRarity(rarity, Number(e.target.value))}
/>

// after
<NumberInput
  aria-label={capitalize(rarity)}
  step={0.1}
  min={0.01}
  value={mult as number}
  onChange={(v) => setRarity(rarity, v)}
  showButtons={false}
  className="w-24"
/>
```

- [ ] **Step 7: Replace `vendor_multipliers` fields (around line 193–202)**

```tsx
// before
<input
  className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-24"
  type="number"
  step="0.1"
  min="0.01"
  value={mult}
  onChange={(e) => setVendor(rarity, Number(e.target.value))}
/>

// after
<NumberInput
  aria-label={capitalize(rarity)}
  step={0.1}
  min={0.01}
  value={mult as number}
  onChange={(v) => setVendor(rarity, v)}
  showButtons={false}
  className="w-24"
/>
```

- [ ] **Step 8: Lint**

```bash
cd web && pnpm lint
```

- [ ] **Step 9: Commit**

```bash
git add web/src/tabs/MarketTab/bot/BotConfigEditor.tsx
git commit -m "feat(market-bot): NumberInput for bot config number fields"
```

---

## Task 9: BattlegroupTab — NumberInput sweep

**Files:**

- Modify: `web/src/tabs/BattlegroupTab/index.tsx`

Two `<Input type="number">` fields: `broadcastDuration` (line ~209–217) and `shutdownDelay` (line ~273–281).

- [ ] **Step 1: Update imports**

Add `NumberInput` to the dune-ui import. Verify `Input` is still used for text fields before removing it from `@heroui/react`.

```bash
grep -n '<Input' web/src/tabs/BattlegroupTab/index.tsx | grep -v 'type="number"'
```

- [ ] **Step 2: Replace `broadcastDuration` field (around line 209–217)**

```tsx
// before
<Input
  type="number"
  min={5}
  max={300}
  value={broadcastDuration}
  onChange={(e) => setBroadcastDuration(Math.max(5, parseInt(e.target.value) || 30))}
  className="w-20"
  aria-label="Duration"
/>

// after
<NumberInput
  aria-label={t('battlegroup.durationLabel')}
  min={5}
  max={300}
  value={broadcastDuration}
  onChange={setBroadcastDuration}
  showButtons={false}
  className="w-20"
/>
```

- [ ] **Step 3: Replace `shutdownDelay` field (around line 273–281)**

```tsx
// before
<Input
  type="number"
  min={1}
  max={120}
  value={shutdownDelay}
  onChange={(e) => setShutdownDelay(Math.max(1, parseInt(e.target.value) || 10))}
  className="w-20"
  aria-label={t('battlegroup.shutdownDelayLabel')}
/>

// after
<NumberInput
  aria-label={t('battlegroup.shutdownDelayLabel')}
  min={1}
  max={120}
  value={shutdownDelay}
  onChange={setShutdownDelay}
  showButtons={false}
  className="w-20"
/>
```

- [ ] **Step 4: Lint**

```bash
cd web && pnpm lint
```

- [ ] **Step 5: Commit**

```bash
git add web/src/tabs/BattlegroupTab/index.tsx
git commit -m "feat(battlegroup): NumberInput for duration fields"
```

---

## Task 10: DatabaseTab — NumberInput sweep

**Files:**

- Modify: `web/src/tabs/DatabaseTab.tsx`

One field: `limitInput` (line ~338–346), currently `string` state `useState('20')`. Change state to `number`.

- [ ] **Step 1: Change `limitInput` state type**

```tsx
// before (line ~186)
const [limitInput, setLimitInput] = useState('20')

// after
const [limitInput, setLimitInput] = useState(20)
```

The downstream usage `Number(limitInput) || 20` (line ~262) becomes just `limitInput`:

```tsx
// before
const r = await api.database.sample(tableInput.trim(), Number(limitInput) || 20)

// after
const r = await api.database.sample(tableInput.trim(), limitInput)
```

- [ ] **Step 2: Update imports**

Remove `InputGroup` and `TextField` from `@heroui/react` (verify they're not used elsewhere):

```bash
grep -n 'InputGroup\|TextField' web/src/tabs/DatabaseTab.tsx
```

Add `NumberInput` to the dune-ui import.

- [ ] **Step 3: Replace the limit input (lines ~335–347)**

```tsx
// before
<TextField className="w-28" aria-label="Limit">
  <InputGroup>
    <InputGroup.Prefix>{t('database.limitLabel')}</InputGroup.Prefix>
    <InputGroup.Input
      className="pl-2"
      type="number"
      min={1}
      max={1000}
      value={limitInput}
      onChange={(e) => setLimitInput(e.target.value)}
    />
  </InputGroup>
</TextField>

// after
<NumberInput
  aria-label={t('database.limitLabel')}
  min={1}
  max={1000}
  value={limitInput}
  onChange={setLimitInput}
  showButtons={false}
  className="w-28"
/>
```

- [ ] **Step 4: Lint**

```bash
cd web && pnpm lint
```

- [ ] **Step 5: Commit**

```bash
git add web/src/tabs/DatabaseTab.tsx
git commit -m "feat(database): NumberInput for query limit field"
```

---

## Task 11: ServerSettingsTab — NumberInput sweep

**Files:**

- Modify: `web/src/tabs/ServerSettingsTab.tsx`

One conditional `<input type="number">` inside a ternary that renders bool/string/number inputs. The `onChange` is `onChange(e.target.value)` — a string callback. The `value` is `display` (a string). Adapt to `Number(display) || 0` and `onChange(String(v))`.

- [ ] **Step 1: Update imports**

Add `NumberInput` to the dune-ui import (check what's already imported):

```bash
grep -n 'from.*dune-ui' web/src/tabs/ServerSettingsTab.tsx
```

- [ ] **Step 2: Replace the number branch in the conditional render (lines ~201–209)**

```tsx
// before
: (
    <input
      type="number"
      step={item.type === 'float' ? '0.01' : '1'}
      value={display}
      onChange={(e) => onChange(e.target.value)}
      className="w-28 bg-surface border border-border rounded px-2 py-1 text-xs font-mono text-foreground focus:outline-none focus:border-accent/60 text-right"
    />
  )}

// after
: (
    <NumberInput
      aria-label={item.key ?? 'value'}
      step={item.type === 'float' ? 0.01 : 1}
      value={Number(display) || 0}
      onChange={(v) => onChange(String(v))}
      showButtons={false}
      className="w-28"
    />
  )}
```

- [ ] **Step 3: Lint**

```bash
cd web && pnpm lint
```

- [ ] **Step 4: Commit**

```bash
git add web/src/tabs/ServerSettingsTab.tsx
git commit -m "feat(server-settings): NumberInput for numeric setting fields"
```

---

## Task 12: SettingsConfigForm — NumberInput sweep

**Files:**

- Modify: `web/src/components/SettingsConfigForm.tsx`

Two `<TI type="number">` calls: `db_port` (line ~240) and `scrip_currency` (line ~403). `TI` uses `onChange: (v: string) => void`. Adapt: `onChange={(v) => set('field')(String(v))}`.

- [ ] **Step 1: Update imports**

Add `NumberInput` to the dune-ui import (check what's already imported):

```bash
grep -n 'from.*dune-ui' web/src/components/SettingsConfigForm.tsx
```

If there's no dune-ui import yet, add:

```tsx
import { NumberInput } from '../dune-ui'
```

- [ ] **Step 2: Replace `db_port` (line ~240)**

```tsx
// before
<TI value={cfg.db_port || ''} onChange={set('db_port')} type="number" placeholder="15432" />

// after
<NumberInput
  aria-label={t('settings.db.port')}
  value={Number(cfg.db_port) || 0}
  onChange={(v) => set('db_port')(String(v))}
  showButtons={false}
  className="w-full"
/>
```

- [ ] **Step 3: Replace `scrip_currency` (line ~403)**

```tsx
// before
<TI value={cfg.scrip_currency || ''} onChange={set('scrip_currency')} type="number" placeholder="1" />

// after
<NumberInput
  aria-label={t('settings.adv.scripCurrency')}
  value={Number(cfg.scrip_currency) || 0}
  onChange={(v) => set('scrip_currency')(String(v))}
  showButtons={false}
  className="w-full"
/>
```

- [ ] **Step 4: Lint**

```bash
cd web && pnpm lint
```

- [ ] **Step 5: Commit**

```bash
git add web/src/components/SettingsConfigForm.tsx
git commit -m "feat(settings): NumberInput for db_port and scrip_currency fields"
```

---

## Task 13: Final verification

- [ ] **Step 1: Full lint**

```bash
cd web && pnpm lint
```

Expected: 0 errors, 0 warnings.

- [ ] **Step 2: Type check**

```bash
cd web && pnpm build
```

Expected: TypeScript compilation succeeds, dist output produced.

- [ ] **Step 3: Confirm no remaining raw `type="number"` inputs**

```bash
grep -rn 'type="number"' web/src --include="*.tsx"
```

Expected: zero results.

- [ ] **Step 4: Run backend verify (no backend changes but confirms CI won't break)**

```bash
make verify
```

Expected: all checks pass.
