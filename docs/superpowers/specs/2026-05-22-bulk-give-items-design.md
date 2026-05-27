# Bulk Give Items

**Date:** 2026-05-22  
**Status:** Approved

## Overview

Add a "Give Items" bulk flow alongside the existing "Give Item" single-item flow in both the Players tab and the Storage tab. Mirrors the tags staging pattern: queue multiple items with per-item qty/quality, then submit in one action.

## UI

Both `PlayersTab` and `StorageTab` replace the single "Give Item" button with two buttons:

```
[ Give Item ]  [ Give Items (N) ]
```

The count `N` reflects how many items are currently staged. "Give Items" is always visible; the badge shows 0 when empty and the submit button is disabled.

The "Give Items" modal contains:

- **Search input** — backed by the existing templates API (same source as Give Item modal), with an **+ Add** button
- **Staged list** — each row: template name | qty input | quality input | ✕ remove button
  - Duplicates are allowed (giving the same template twice is valid)
  - Qty and quality are editable inline per row
- **Give N Items** submit button — disabled when list is empty
- **Result summary** — shown inline after submit: `"3 given, 1 skipped (Stillsuit: inventory full)"`
  - Modal closes automatically on full success (all items given)
  - Modal stays open on partial failure so the user can see what was skipped
  - Staged list clears on submit; skipped items are not re-staged (user sees result and decides)

## API

Two new endpoints mirroring the existing single-item routes:

```
POST /api/v1/players/give-items
POST /api/v1/storage/{id}/give-items
```

### Request

```json
{
  "player_id": "...",
  "items": [
    { "template": "crysknife", "qty": 1, "quality": 0 },
    { "template": "spice_ration", "qty": 5, "quality": 0 }
  ]
}
```

### Response

```json
{
  "given": ["crysknife", "spice_ration"],
  "skipped": [
    { "template": "stillsuit", "reason": "inventory full" }
  ]
}
```

## Backend

**handlers_players.go** — new `handleGiveItems()` handler:

- Decodes `{ player_id, items: [{template, qty, quality}] }`
- Loops `cmdGiveItem(playerID, item.Template, item.Qty, item.Quality)` for each item
- Collects successes into `given []string` and errors into `skipped []{ Template, Reason }`
- Returns the combined result — never returns an HTTP error for per-item failures

**handlers_storage.go** — new `handleGiveItemsToStorage()` handler:

- Same pattern using `cmdGiveItemToContainer`

**server.go** — register two new routes:

```
POST /api/v1/players/give-items
POST /api/v1/storage/{id}/give-items
```

No new DB functions needed. Both handlers reuse existing `cmdGiveItem` and `cmdGiveItemToContainer`.

## Frontend State

Each modal is self-contained with local state:

```ts
const [stagedItems, setStagedItems] = useState<{ template: string; qty: number; quality: number }[]>([])
const [result, setResult] = useState<{ given: string[]; skipped: { template: string; reason: string }[] } | null>(null)
```

Operations:

- **Add:** append to `stagedItems`
- **Remove:** filter by index
- **Edit qty/quality:** replace by index
- **Submit:** call `api.players.giveItems(...)` or `api.storage.giveItems(...)`, set `result`, clear `stagedItems`

## API Client (`client.ts`)

Two new methods:

```ts
players: {
  giveItems: (player_id, items) => POST /players/give-items
}
storage: {
  giveItems: (id, items) => POST /storage/{id}/give-items
}
```

## Files to Change

| File | Change |
|------|--------|
| `handlers_players.go` | Add `handleGiveItems()` |
| `handlers_storage.go` | Add `handleGiveItemsToStorage()` |
| `server.go` | Register 2 new routes |
| `web/src/api/client.ts` | Add `giveItems` to `players` and `storage` |
| `web/src/tabs/PlayersTab.tsx` | Add "Give Items" button + modal |
| `web/src/tabs/StorageTab.tsx` | Add "Give Items" button + modal |
