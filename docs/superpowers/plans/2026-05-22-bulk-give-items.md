# Bulk Give Items Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Give Items" bulk flow alongside the existing "Give Item" button in both the Players and Storage tabs, using a staging list UI where each item has per-item qty and quality.

**Architecture:** Two new backend handlers loop the existing `cmdGiveItem`/`cmdGiveItemToContainer` per item, collect per-item results, and return `{ given, skipped }`. The frontend adds a `GiveItemsModal` (Players) and a bulk `AddItemsModal` (Storage) — each self-contained within the existing tab files, following the same component pattern as the current single-item modals.

**Tech Stack:** Go (net/http), TypeScript, React, HeroUI v3

---

## File Map

| File | Change |
|------|--------|
| `handlers_players.go` | Add `handleGiveItems()` handler |
| `handlers_storage.go` | Add `handleGiveItemsToStorage()` handler |
| `server.go` | Register 2 new routes |
| `web/src/api/client.ts` | Add `BulkGiveResult` type + `giveItems` to `players` and `storage` |
| `web/src/tabs/PlayersTab.tsx` | Add `GiveItemsModal` component + `showGiveItems` state + button |
| `web/src/tabs/StorageTab.tsx` | Add bulk `AddItemsModal` component + `showGiveItems` state + button |

---

## Task 1: Backend — `handleGiveItems` in `handlers_players.go`

**Files:**

- Modify: `handlers_players.go` (append after `handleGiveItem`, around line 208)

- [ ] **Step 1: Add the handler**

  Append this function directly after the closing `}` of `handleGiveItem` (line 208):

  ```go
  func handleGiveItems(w http.ResponseWriter, r *http.Request) {
   var req struct {
    PlayerID int64 `json:"player_id"`
    Items    []struct {
     Template string `json:"template"`
     Qty      int64  `json:"qty"`
     Quality  int64  `json:"quality"`
    } `json:"items"`
   }
   if err := decode(r, &req); err != nil {
    jsonErr(w, err, 400)
    return
   }
   type skippedItem struct {
    Template string `json:"template"`
    Reason   string `json:"reason"`
   }
   given := []string{}
   skipped := []skippedItem{}
   for _, item := range req.Items {
    msg, ok := cmdGiveItem(req.PlayerID, item.Template, item.Qty, item.Quality)().(msgMutate)
    if !ok || msg.err != nil {
     reason := "internal error"
     if ok && msg.err != nil {
      reason = msg.err.Error()
     }
     skipped = append(skipped, skippedItem{Template: item.Template, Reason: reason})
     continue
    }
    given = append(given, item.Template)
   }
   jsonOK(w, map[string]interface{}{"given": given, "skipped": skipped})
  }
  ```

- [ ] **Step 2: Verify it compiles**

  Run from the project root:

  ```bash
  go build ./...
  ```

  Expected: no output, exit code 0.

- [ ] **Step 3: Run existing tests**

  ```bash
  go test ./...
  ```

  Expected: `ok   dune-admin` (all pass, no new failures).

- [ ] **Step 4: Commit**

  ```bash
  git add handlers_players.go
  git commit -m "feat: add handleGiveItems handler"
  ```

---

## Task 2: Backend — `handleGiveItemsToStorage` in `handlers_storage.go`

**Files:**

- Modify: `handlers_storage.go` (append after `handleGiveItemToStorage`, around line 73)

- [ ] **Step 1: Add the handler**

  Append this function directly after the closing `}` of `handleGiveItemToStorage` (line 73):

  ```go
  func handleGiveItemsToStorage(w http.ResponseWriter, r *http.Request) {
   idStr := r.PathValue("id")
   id, err := strconv.ParseInt(idStr, 10, 64)
   if err != nil {
    jsonErr(w, fmt.Errorf("invalid id"), 400)
    return
   }
   var req struct {
    Items []struct {
     Template string `json:"template"`
     Qty      int64  `json:"qty"`
     Quality  int64  `json:"quality"`
    } `json:"items"`
   }
   if err := decode(r, &req); err != nil {
    jsonErr(w, err, 400)
    return
   }
   type skippedItem struct {
    Template string `json:"template"`
    Reason   string `json:"reason"`
   }
   given := []string{}
   skipped := []skippedItem{}
   for _, item := range req.Items {
    msg, ok := cmdGiveItemToContainer(id, item.Template, item.Qty, item.Quality)().(msgMutate)
    if !ok || msg.err != nil {
     reason := "internal error"
     if ok && msg.err != nil {
      reason = msg.err.Error()
     }
     skipped = append(skipped, skippedItem{Template: item.Template, Reason: reason})
     continue
    }
    given = append(given, item.Template)
   }
   jsonOK(w, map[string]interface{}{"given": given, "skipped": skipped})
  }
  ```

- [ ] **Step 2: Verify it compiles**

  ```bash
  go build ./...
  ```

  Expected: no output, exit code 0.

- [ ] **Step 3: Commit**

  ```bash
  git add handlers_storage.go
  git commit -m "feat: add handleGiveItemsToStorage handler"
  ```

---

## Task 3: Backend — Register routes in `server.go`

**Files:**

- Modify: `server.go`

- [ ] **Step 1: Add the two new routes**

  In `server.go`, find the players block (around line 74):

  ```go
  mux.HandleFunc("POST /api/v1/players/give-item", handleGiveItem)
  ```

  Add directly after it:

  ```go
  mux.HandleFunc("POST /api/v1/players/give-items", handleGiveItems)
  ```

  Find the storage block (around line 128):

  ```go
  mux.HandleFunc("POST /api/v1/storage/{id}/give-item", handleGiveItemToStorage)
  ```

  Add directly after it:

  ```go
  mux.HandleFunc("POST /api/v1/storage/{id}/give-items", handleGiveItemsToStorage)
  ```

- [ ] **Step 2: Verify it compiles and tests pass**

  ```bash
  go build ./... && go test ./...
  ```

  Expected: no output from build, `ok   dune-admin` from tests.

- [ ] **Step 3: Commit**

  ```bash
  git add server.go
  git commit -m "feat: register bulk give-items routes"
  ```

---

## Task 4: API Client — `giveItems` methods in `client.ts`

**Files:**

- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Add `BulkGiveResult` type**

  In `web/src/api/client.ts`, find line 56:

  ```typescript
  export type MutateResult = { ok: string }
  ```

  Add directly after it:

  ```typescript
  export type BulkGiveResult = { given: string[]; skipped: { template: string; reason: string }[] }
  ```

- [ ] **Step 2: Add `giveItems` to the `players` object**

  Find (line 98):

  ```typescript
      giveItem: (player_id: number, template: string, qty: number, quality: number) =>
        req<MutateResult>('POST', '/players/give-item', { player_id, template, qty, quality }),
  ```

  Add directly after it:

  ```typescript
      giveItems: (player_id: number, items: { template: string; qty: number; quality: number }[]) =>
        req<BulkGiveResult>('POST', '/players/give-items', { player_id, items }),
  ```

- [ ] **Step 3: Add `giveItems` to the `storage` object**

  Find (line 170):

  ```typescript
      giveItem: (id: number, template: string, qty: number, quality: number) =>
        req<MutateResult>('POST', `/storage/${id}/give-item`, { template, qty, quality }),
  ```

  Add directly after it:

  ```typescript
      giveItems: (id: number, items: { template: string; qty: number; quality: number }[]) =>
        req<BulkGiveResult>('POST', `/storage/${id}/give-items`, { items }),
  ```

- [ ] **Step 4: Type-check**

  ```bash
  cd web && npx tsc --noEmit
  ```

  Expected: no errors.

- [ ] **Step 5: Commit**

  ```bash
  git add web/src/api/client.ts
  git commit -m "feat: add giveItems to API client"
  ```

---

## Task 5: Frontend — `GiveItemsModal` in `PlayersTab.tsx`

**Files:**

- Modify: `web/src/tabs/PlayersTab.tsx`

- [ ] **Step 1: Add `showGiveItems` state**

  Find (line 69):

  ```typescript
    const [showGiveItem, setShowGiveItem] = useState(false)
  ```

  Add directly after it:

  ```typescript
    const [showGiveItems, setShowGiveItems] = useState(false)
  ```

- [ ] **Step 2: Add the "Give Items" button**

  Find (line 244):

  ```tsx
                            <Button size="sm" variant="ghost" onPress={() => { setSelectedPlayer(player); setShowGiveItem(true) }}>Give Item</Button>
  ```

  Add directly after it:

  ```tsx
                            <Button size="sm" variant="ghost" onPress={() => { setSelectedPlayer(player); setShowGiveItems(true) }}>Give Items</Button>
  ```

- [ ] **Step 3: Mount the modal**

  Find (line 421):

  ```tsx
          <GiveItemModal player={selectedPlayer} open={showGiveItem} onClose={() => setShowGiveItem(false)} />
  ```

  Add directly after it:

  ```tsx
          <GiveItemsModal player={selectedPlayer} open={showGiveItems} onClose={() => setShowGiveItems(false)} />
  ```

- [ ] **Step 4: Add the `GiveItemsModal` component**

  Find the end of the file (after the closing `}` of `GiveItemModal`). Append the new component:

  ```tsx
  function GiveItemsModal({ player, open, onClose }: { player: Player; open: boolean; onClose: () => void }) {
    const [templates, setTemplates] = useState<{id: string; name: string}[]>([])
    const [loading, setLoading] = useState(false)
    const [query, setQuery] = useState('')
    const [selected, setSelected] = useState('')
    const [qty, setQty] = useState(1)
    const [quality, setQuality] = useState(0)
    const [staged, setStaged] = useState<{ template: string; qty: number; quality: number }[]>([])
    const [submitting, setSubmitting] = useState(false)
    const [result, setResult] = useState<{ given: string[]; skipped: { template: string; reason: string }[] } | null>(null)

    useEffect(() => {
      if (!open) return
      setLoading(true)
      api.players.templates().then(setTemplates).catch(() => {}).finally(() => setLoading(false))
      setQuery(''); setSelected(''); setQty(1); setQuality(0); setStaged([]); setResult(null)
    }, [open])

    const filtered = useMemo(() => {
      if (!query) return []
      const q = query.toLowerCase()
      return templates.filter(t => t.id.toLowerCase().includes(q) || t.name.toLowerCase().includes(q)).slice(0, 100)
    }, [templates, query])

    const pick = (t: {id: string; name: string}) => {
      setSelected(t.id)
      setQuery(t.name ? `${t.id}  —  ${t.name}` : t.id)
    }

    const addToStaged = () => {
      if (!selected) { toast.warning('Select a template'); return }
      setStaged(prev => [...prev, { template: selected, qty, quality }])
      setQuery(''); setSelected(''); setQty(1); setQuality(0)
    }

    const removeFromStaged = (idx: number) => {
      setStaged(prev => prev.filter((_, i) => i !== idx))
    }

    const updateStaged = (idx: number, field: 'qty' | 'quality', value: number) => {
      setStaged(prev => prev.map((item, i) => i === idx ? { ...item, [field]: value } : item))
    }

    const handleSubmit = async () => {
      if (staged.length === 0) return
      setSubmitting(true)
      try {
        const res = await api.players.giveItems(player.id, staged)
        setResult(res)
        setStaged([])
        if (res.skipped.length === 0) onClose()
      } catch (e: unknown) {
        toast.danger(e instanceof Error ? e.message : String(e))
      } finally {
        setSubmitting(false)
      }
    }

    return (
      <Modal>
        <Modal.Backdrop isOpen={open} onOpenChange={v => !v && onClose()}>
          <Modal.Container size="full">
            <Modal.Dialog style={{ maxHeight: '85vh', display: 'flex', flexDirection: 'column' }}>
              <Modal.CloseTrigger />
              <Modal.Header><Modal.Heading>Give Items — {player.name}</Modal.Heading></Modal.Header>
              <Modal.Body style={{ display: 'flex', flexDirection: 'column', overflow: 'hidden', padding: '12px 16px' }}>
                {loading ? (
                  <div className="flex justify-center py-6"><Spinner size="lg" /></div>
                ) : (
                  <div className="flex flex-col gap-3 h-full overflow-hidden">
                    <div className="flex items-center gap-2 shrink-0">
                      <div className="relative flex-1">
                        <input
                          className="w-full rounded px-3 py-1.5 text-sm border"
                          style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }}
                          placeholder="Search templates..."
                          value={query}
                          onChange={e => { setQuery(e.target.value); setSelected('') }}
                        />
                        {filtered.length > 0 && (
                          <div className="absolute z-50 w-full mt-1 rounded border overflow-y-auto" style={{ background: 'var(--color-surface)', borderColor: '#2a2418', maxHeight: '200px' }}>
                            {filtered.map(t => (
                              <div key={t.id} className="px-3 py-1.5 text-xs cursor-pointer hover:bg-[#2a2418]" onClick={() => pick(t)}>
                                <span className="font-mono">{t.id}</span>{t.name ? <span style={{ color: 'var(--color-text-dim)' }}>  —  {t.name}</span> : null}
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                      <input type="number" min={1} value={qty} onChange={e => setQty(Number(e.target.value))}
                        className="rounded px-2 py-1.5 text-sm border w-16 text-center"
                        style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }} />
                      <input type="number" min={0} value={quality} onChange={e => setQuality(Number(e.target.value))}
                        className="rounded px-2 py-1.5 text-sm border w-16 text-center"
                        style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }} />
                      <Button size="sm" onPress={addToStaged} isDisabled={!selected}>+ Add</Button>
                    </div>
                    {staged.length > 0 && (
                      <div className="flex flex-col gap-1 overflow-y-auto flex-1">
                        {staged.map((item, idx) => (
                          <div key={idx} className="flex items-center gap-2 px-3 py-1.5 rounded text-xs" style={{ background: 'var(--color-surface)', border: '1px solid #2a2418' }}>
                            <span className="flex-1 font-mono">{item.template}</span>
                            <input type="number" min={1} value={item.qty} onChange={e => updateStaged(idx, 'qty', Number(e.target.value))}
                              className="rounded px-2 py-1 border w-14 text-center"
                              style={{ background: 'var(--color-bg)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }} />
                            <input type="number" min={0} value={item.quality} onChange={e => updateStaged(idx, 'quality', Number(e.target.value))}
                              className="rounded px-2 py-1 border w-14 text-center"
                              style={{ background: 'var(--color-bg)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }} />
                            <button onClick={() => removeFromStaged(idx)} className="text-red-400 hover:text-red-300 px-1">✕</button>
                          </div>
                        ))}
                      </div>
                    )}
                    {result && (
                      <div className="text-xs shrink-0 rounded px-3 py-2" style={{ background: 'var(--color-surface)', border: '1px solid #2a2418' }}>
                        {result.given.length > 0 && <div style={{ color: 'var(--color-success)' }}>✓ Gave: {result.given.join(', ')}</div>}
                        {result.skipped.map((s, i) => (
                          <div key={i} style={{ color: 'var(--color-danger)' }}>✕ Skipped {s.template}: {s.reason}</div>
                        ))}
                      </div>
                    )}
                    <div className="flex items-center gap-3 shrink-0">
                      <Button variant="tertiary" size="sm" onPress={onClose}>Cancel</Button>
                      <Button size="sm" onPress={handleSubmit} isDisabled={submitting || staged.length === 0}>
                        {submitting ? <Spinner size="sm" color="current" /> : null}
                        Give {staged.length} Item{staged.length !== 1 ? 's' : ''}
                      </Button>
                    </div>
                  </div>
                )}
              </Modal.Body>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
    )
  }
  ```

- [ ] **Step 5: Type-check**

  ```bash
  cd web && npx tsc --noEmit
  ```

  Expected: no errors.

- [ ] **Step 6: Commit**

  ```bash
  git add web/src/tabs/PlayersTab.tsx
  git commit -m "feat: add GiveItemsModal to PlayersTab"
  ```

---

## Task 6: Frontend — Bulk `AddItemsModal` in `StorageTab.tsx`

**Files:**

- Modify: `web/src/tabs/StorageTab.tsx`

- [ ] **Step 1: Add `showGiveItems` state**

  Find (line 19):

  ```typescript
    const [showGive, setShowGive] = useState(false)
  ```

  Add directly after it:

  ```typescript
    const [showGiveItems, setShowGiveItems] = useState(false)
  ```

- [ ] **Step 2: Add the "Add Items" button**

  Find (line 141):

  ```tsx
                <Button size="sm" onPress={() => setShowGive(true)}>
                  + Add Item
                </Button>
  ```

  Add directly after it:

  ```tsx
                <Button size="sm" onPress={() => setShowGiveItems(true)}>
                  + Add Items
                </Button>
  ```

- [ ] **Step 3: Mount the bulk modal**

  Find (around line 188):

  ```tsx
        {selected && (
          <AddItemModal
            container={selected}
            open={showGive}
            onClose={() => setShowGive(false)}
            onSuccess={() => { setShowGive(false); selectContainer(selected) }}
          />
        )}
  ```

  Replace it with:

  ```tsx
        {selected && (
          <>
            <AddItemModal
              container={selected}
              open={showGive}
              onClose={() => setShowGive(false)}
              onSuccess={() => { setShowGive(false); selectContainer(selected) }}
            />
            <AddItemsModal
              container={selected}
              open={showGiveItems}
              onClose={() => setShowGiveItems(false)}
              onSuccess={() => { setShowGiveItems(false); selectContainer(selected) }}
            />
          </>
        )}
  ```

- [ ] **Step 4: Add the `AddItemsModal` component**

  Find the end of the file (after the closing `}` of `AddItemModal`). Append:

  ```tsx
  function AddItemsModal({ container, open, onClose, onSuccess }: {
    container: Container;
    open: boolean;
    onClose: () => void;
    onSuccess: () => void;
  }) {
    const [templates, setTemplates] = useState<{id: string; name: string}[]>([])
    const [loading, setLoading] = useState(false)
    const [query, setQuery] = useState('')
    const [selected, setSelected] = useState('')
    const [qty, setQty] = useState(1)
    const [quality, setQuality] = useState(0)
    const [staged, setStaged] = useState<{ template: string; qty: number; quality: number }[]>([])
    const [submitting, setSubmitting] = useState(false)
    const [result, setResult] = useState<{ given: string[]; skipped: { template: string; reason: string }[] } | null>(null)

    useEffect(() => {
      if (!open) return
      setLoading(true)
      api.players.templates().then(setTemplates).catch(() => {}).finally(() => setLoading(false))
      setQuery(''); setSelected(''); setQty(1); setQuality(0); setStaged([]); setResult(null)
    }, [open])

    const filtered = useMemo(() => {
      if (!query) return []
      const q = query.toLowerCase()
      return templates.filter(t => t.id.toLowerCase().includes(q) || t.name.toLowerCase().includes(q)).slice(0, 100)
    }, [templates, query])

    const pick = (t: {id: string; name: string}) => {
      setSelected(t.id)
      setQuery(t.name ? `${t.id}  —  ${t.name}` : t.id)
    }

    const addToStaged = () => {
      if (!selected) { toast.warning('Select a template'); return }
      setStaged(prev => [...prev, { template: selected, qty, quality }])
      setQuery(''); setSelected(''); setQty(1); setQuality(0)
    }

    const removeFromStaged = (idx: number) => {
      setStaged(prev => prev.filter((_, i) => i !== idx))
    }

    const updateStaged = (idx: number, field: 'qty' | 'quality', value: number) => {
      setStaged(prev => prev.map((item, i) => i === idx ? { ...item, [field]: value } : item))
    }

    const handleSubmit = async () => {
      if (staged.length === 0) return
      setSubmitting(true)
      try {
        const res = await api.storage.giveItems(container.id, staged)
        setResult(res)
        setStaged([])
        if (res.skipped.length === 0) onSuccess()
      } catch (e: unknown) {
        toast.danger(e instanceof Error ? e.message : String(e))
      } finally {
        setSubmitting(false)
      }
    }

    return (
      <Modal>
        <Modal.Backdrop isOpen={open} onOpenChange={v => !v && onClose()}>
          <Modal.Container size="full">
            <Modal.Dialog style={{ maxHeight: '85vh', display: 'flex', flexDirection: 'column' }}>
              <Modal.CloseTrigger />
              <Modal.Header><Modal.Heading>Add Items — Container #{container.id}</Modal.Heading></Modal.Header>
              <Modal.Body style={{ display: 'flex', flexDirection: 'column', overflow: 'hidden', padding: '12px 16px' }}>
                {loading ? (
                  <div className="flex justify-center py-6"><Spinner size="lg" /></div>
                ) : (
                  <div className="flex flex-col gap-3 h-full overflow-hidden">
                    <div className="flex items-center gap-2 shrink-0">
                      <div className="relative flex-1">
                        <input
                          className="w-full rounded px-3 py-1.5 text-sm border"
                          style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }}
                          placeholder="Search templates..."
                          value={query}
                          onChange={e => { setQuery(e.target.value); setSelected('') }}
                        />
                        {filtered.length > 0 && (
                          <div className="absolute z-50 w-full mt-1 rounded border overflow-y-auto" style={{ background: 'var(--color-surface)', borderColor: '#2a2418', maxHeight: '200px' }}>
                            {filtered.map(t => (
                              <div key={t.id} className="px-3 py-1.5 text-xs cursor-pointer hover:bg-[#2a2418]" onClick={() => pick(t)}>
                                <span className="font-mono">{t.id}</span>{t.name ? <span style={{ color: 'var(--color-text-dim)' }}>  —  {t.name}</span> : null}
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                      <input type="number" min={1} value={qty} onChange={e => setQty(Number(e.target.value))}
                        className="rounded px-2 py-1.5 text-sm border w-16 text-center"
                        style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }} />
                      <input type="number" min={0} value={quality} onChange={e => setQuality(Number(e.target.value))}
                        className="rounded px-2 py-1.5 text-sm border w-16 text-center"
                        style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }} />
                      <Button size="sm" onPress={addToStaged} isDisabled={!selected}>+ Add</Button>
                    </div>
                    {staged.length > 0 && (
                      <div className="flex flex-col gap-1 overflow-y-auto flex-1">
                        {staged.map((item, idx) => (
                          <div key={idx} className="flex items-center gap-2 px-3 py-1.5 rounded text-xs" style={{ background: 'var(--color-surface)', border: '1px solid #2a2418' }}>
                            <span className="flex-1 font-mono">{item.template}</span>
                            <input type="number" min={1} value={item.qty} onChange={e => updateStaged(idx, 'qty', Number(e.target.value))}
                              className="rounded px-2 py-1 border w-14 text-center"
                              style={{ background: 'var(--color-bg)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }} />
                            <input type="number" min={0} value={item.quality} onChange={e => updateStaged(idx, 'quality', Number(e.target.value))}
                              className="rounded px-2 py-1 border w-14 text-center"
                              style={{ background: 'var(--color-bg)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }} />
                            <button onClick={() => removeFromStaged(idx)} className="text-red-400 hover:text-red-300 px-1">✕</button>
                          </div>
                        ))}
                      </div>
                    )}
                    {result && (
                      <div className="text-xs shrink-0 rounded px-3 py-2" style={{ background: 'var(--color-surface)', border: '1px solid #2a2418' }}>
                        {result.given.length > 0 && <div style={{ color: 'var(--color-success)' }}>✓ Added: {result.given.join(', ')}</div>}
                        {result.skipped.map((s, i) => (
                          <div key={i} style={{ color: 'var(--color-danger)' }}>✕ Skipped {s.template}: {s.reason}</div>
                        ))}
                      </div>
                    )}
                    <div className="flex items-center gap-3 shrink-0">
                      <Button variant="tertiary" size="sm" onPress={onClose}>Cancel</Button>
                      <Button size="sm" onPress={handleSubmit} isDisabled={submitting || staged.length === 0}>
                        {submitting ? <Spinner size="sm" color="current" /> : null}
                        Add {staged.length} Item{staged.length !== 1 ? 's' : ''}
                      </Button>
                    </div>
                  </div>
                )}
              </Modal.Body>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
    )
  }
  ```

- [ ] **Step 5: Type-check**

  ```bash
  cd web && npx tsc --noEmit
  ```

  Expected: no errors.

- [ ] **Step 6: Commit**

  ```bash
  git add web/src/tabs/StorageTab.tsx
  git commit -m "feat: add bulk AddItemsModal to StorageTab"
  ```
