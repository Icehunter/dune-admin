# Players Tab: Master-Detail Layout

## Summary

Redesign the Players tab from a stacked list+detail layout into a two-column master-detail layout with a fixed 320px player list on the left and an always-visible detail panel on the right. Remove the 5-player visibility cap; show all players in a scrollable sidebar.

## Layout

```
┌──────────────────────────────────────────────────────┐
│  Players                           ● 5 online  ↺     │
├─────────────────┬────────────────────────────────────┤
│ 320px sidebar   │  flex-1 detail panel               │
│ ┌────────────┐  │                                    │
│ │ 🔍 Filter  │  │  [selected player details]         │
│ └────────────┘  │                                    │
│  ● Alice        │                                    │
│  ○ Bob          │                                    │
│  ○ Carol        │                                    │
│   (scrollable)  │                                    │
└─────────────────┴────────────────────────────────────┘
```

## Components

**`PlayersTab/index.tsx`** — restructured root:

- `flex-col h-full min-h-0` outer wrapper
- `PageHeader` spans full width
- `flex flex-1 min-h-0` row below header:
  - Left: `w-80 flex-shrink-0 flex-col border-r border-border` — search + scrollable player list (`overflow-y-auto`)
  - Right: `flex-1 min-w-0 overflow-y-auto p-4` — `PlayerDetailPanel` or empty state
- Remove `MAX_VISIBLE = 5`; render all filtered players
- First player auto-selected on load (current `prev ?? list[0]` logic preserved)
- Action buttons (inventory / give / actions) stay on each `PlayerCard`

**`PlayerCard.tsx`** — no structural changes; works in 320px sidebar as-is.

## Backend

No changes. `GET /api/v1/players` returns all players in one shot. At ≤200 players the payload is ~20KB — server-side pagination is not needed at this scale.

## Data Flow

1. `useEffect` → `api.players.list()` → sets `players` state
2. `setSelected(prev => prev ?? list[0] ?? null)` auto-selects first
3. Client-side filter (`useMemo`) on `search` — no debounce needed at ≤200 items
4. Selected player passed to `PlayerDetailPanel` (unchanged)

## Out of Scope

- Action button relocation to detail panel
- Server-side pagination
- Virtualization (not needed at ≤200 rows)
