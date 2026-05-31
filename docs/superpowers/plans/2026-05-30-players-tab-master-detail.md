# Players Tab Master-Detail Layout — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure PlayersTab into a 320px left sidebar (filter + scrollable full player list) beside a flex-1 detail panel; auto-select first player on load.

**Architecture:** Single-file frontend change. `PlayersTab/index.tsx` gets a horizontal split layout (`flex-row`) replacing the current stacked layout. Backend unchanged — all players are loaded in one shot and filtered client-side. `PlayerCard` and `PlayerDetailPanel` are used as-is.

**Tech Stack:** React 18, TypeScript strict, HeroUI v3, Tailwind semantic tokens, pnpm

---

## Tasks

### Task 1: Restructure `PlayersTab/index.tsx`

**Files:**

- Modify: `web/src/tabs/PlayersTab/index.tsx`

No backend changes. No new files. The only file that changes is the root tab component.

- [ ] **Step 1: Remove `MAX_VISIBLE` and the slice**

Open `web/src/tabs/PlayersTab/index.tsx`.

Delete line 13:

```ts
const MAX_VISIBLE = 5
```

Change the `filtered` memo — remove the `.slice(0, MAX_VISIBLE)` call:

```ts
const filtered = useMemo(() => {
  const q = search.toLowerCase()
  return q
    ? players.filter((p) =>
        p.name.toLowerCase().includes(q)
        || p.class.toLowerCase().includes(q)
        || p.map.toLowerCase().includes(q),
      )
    : players
}, [players, search])
```

- [ ] **Step 2: Replace the full JSX return with the two-column layout**

Replace the entire `return (...)` block with:

```tsx
return (
  <div className="flex flex-col h-full min-h-0">
    <PageHeader title="Players" onRefresh={loadPlayers} loading={loading}>
      <div className="flex items-center gap-2 text-sm text-muted">
        <StatusDot status={onlineCount > 0 ? 'Online' : 'Offline'} />
        {onlineCount}
        {' online'}
      </div>
    </PageHeader>

    <div className="flex flex-1 min-h-0">
      {/* ── Left sidebar ── */}
      <div className="w-80 shrink-0 flex flex-col border-r border-border">
        <div className="p-2 border-b border-border">
          <SearchField
            aria-label="Search players"
            className="w-full"
            value={search}
            onChange={setSearch}
          >
            <SearchField.Group>
              <SearchField.SearchIcon />
              <SearchField.Input placeholder="Filter players..." />
              <SearchField.ClearButton />
            </SearchField.Group>
          </SearchField>
        </div>

        <div className="flex-1 overflow-y-auto p-2 flex flex-col gap-1">
          {loading && players.length === 0
            ? <div className="flex justify-center py-4"><Spinner size="sm" /></div>
            : filtered.length === 0
              ? <p className="text-muted text-sm text-center py-4">No players found</p>
              : filtered.map((p) => (
                  <PlayerCard
                    key={p.id}
                    player={p}
                    selected={selected?.id === p.id}
                    onSelect={setSelected}
                    onAction={openModal}
                  />
                ))}
        </div>
      </div>

      {/* ── Right detail ── */}
      <div className="flex-1 min-w-0 overflow-y-auto p-4">
        {selected
          ? (
              <div className="flex flex-col gap-1">
                <div className="flex items-center gap-2 mb-2">
                  <span className="font-semibold text-accent">{selected.name}</span>
                  <StatusDot status={selected.online_status} />
                  <span className="text-muted text-xs">{selected.online_status}</span>
                </div>
                <PlayerDetailPanel player={selected} />
              </div>
            )
          : <p className="text-muted text-sm text-center py-8">Select a player</p>}
      </div>
    </div>

    {modalPlayer && (
      <>
        <InventoryModal player={modalPlayer} open={showInventory} onClose={() => setShowInventory(false)} />
        <GiveItemsModal player={modalPlayer} open={showGive} onClose={() => setShowGive(false)} />
        <PlayerActionsModal player={modalPlayer} open={showActions} onClose={() => setShowActions(false)} />
      </>
    )}
  </div>
)
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd /Volumes/Engineering/Icehunter/dune-admin && make tsc
```

Expected: exits 0, no errors.

- [ ] **Step 4: Lint**

```bash
cd /Volumes/Engineering/Icehunter/dune-admin/web && pnpm lint
```

Expected: exits 0, no warnings or errors.

- [ ] **Step 5: Start dev server and visually verify**

```bash
cd /Volumes/Engineering/Icehunter/dune-admin && make dev
```

Open `http://localhost:5173` → Players tab.

Check:

- 320px sidebar on the left with search box at top, scrollable player list below
- First player is selected on load (accent border on card, detail panel visible)
- Filtering the search narrows the list instantly
- Clicking a different player updates the detail panel
- Scrolling the sidebar works when there are many players
- Detail panel scrolls independently

- [ ] **Step 6: Run full verify**

```bash
cd /Volumes/Engineering/Icehunter/dune-admin && make verify
```

Expected: all checks pass (Go tests + lint + fmt).

---

## Self-Review Checklist

- **Spec coverage:**
  - [x] 320px left panel with filter — Task 1 Step 2 (sidebar `w-80`, `SearchField` in `p-2` block)
  - [x] Infinite scroll (full list, no cap) — Task 1 Step 1 (remove `MAX_VISIBLE`)
  - [x] First player auto-selected — preserved via existing `prev ?? list[0]` in `loadPlayers`
  - [x] Detail on right — Task 1 Step 2 (right `flex-1` div)
  - [x] Backend unchanged — no backend tasks in plan
- **Placeholders:** none
- **Type consistency:** no new types introduced; all props match existing `Player`, `PlayerCard`, `PlayerDetailPanel` signatures
