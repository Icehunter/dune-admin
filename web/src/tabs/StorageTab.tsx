import { useState, useEffect, useMemo } from 'react'
import { Button, Modal, Spinner, toast } from '@heroui/react'
import { api } from '../api/client'
import type { InventoryItem } from '../api/client'
import { useTableSort } from '../hooks/useTableSort'
import { SortIndicator } from '../components/SortIndicator'

type ItemSortKey = 'id' | 'template' | 'stack_size' | 'quality' | 'durability'

const ITEM_COLUMNS: { key: ItemSortKey; label: string }[] = [
  { key: 'id', label: 'ID' },
  { key: 'template', label: 'Template' },
  { key: 'stack_size', label: 'Stack' },
  { key: 'quality', label: 'Quality' },
  { key: 'durability', label: 'Durability' },
]

type Container = { id: number; name: string; class: string; map: string; item_count: number }

const TYPE_LABELS: Record<string, string> = {
  SpiceSilo_Placeable: 'Small Storage Container',
  GenericContainer_Placeable: 'Chest',
  StorageContainer_Placeable: 'Storage Container',
  MediumStorageContainer_Placeable: 'Medium Storage Container',
}

function shortClass(cls: string): string {
  return TYPE_LABELS[cls] ?? cls.replace(/_Placeable$/, '')
}

export default function StorageTab() {
  const [containers, setContainers] = useState<Container[]>([])
  const [loading, setLoading] = useState(false)
  const [selected, setSelected] = useState<Container | null>(null)
  const [items, setItems] = useState<InventoryItem[]>([])
  const [itemsLoading, setItemsLoading] = useState(false)
  const [showGiveItems, setShowGiveItems] = useState(false)
  const [search, setSearch] = useState('')

  const load = async () => {
    setLoading(true)
    try {
      setContainers(await api.storage.list())
    } catch (e: unknown) {
      toast.danger((e instanceof Error ? e.message : String(e)))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const selectContainer = async (c: Container) => {
    setSelected(c)
    setItemsLoading(true)
    try {
      setItems(await api.storage.items(c.id))
    } catch (e: unknown) {
      toast.danger((e instanceof Error ? e.message : String(e)))
    } finally {
      setItemsLoading(false)
    }
  }

  const handleDeleteItem = async (itemId: number) => {
    try {
      await api.players.deleteItem(itemId)
      setItems(prev => prev.filter(i => i.id !== itemId))
      if (selected) setContainers(prev => prev.map(c => c.id === selected.id ? { ...c, item_count: c.item_count - 1 } : c))
      toast.success('Item removed')
    } catch (e: unknown) {
      toast.danger((e instanceof Error ? e.message : String(e)))
    }
  }

  const filtered = useMemo(() => {
    const q = search.toLowerCase()
    return containers.filter(c => String(c.id).includes(q) || c.map.toLowerCase().includes(q) || shortClass(c.class).toLowerCase().includes(q))
  }, [containers, search])

  const { sorted: sortedItems, sortKey: itemSortKey, sortDir: itemSortDir, toggle: toggleItemSort } =
    useTableSort<InventoryItem, ItemSortKey>(items, 'id', (r, k) => {
      if (k === 'template') return r.name || r.template_id
      return r[k] as string | number
    })

  return (
    <div className="flex flex-col gap-3 h-full overflow-hidden">
      <div className="shrink-0 rounded-lg px-4 py-2 text-xs font-medium" style={{ background: '#1a0808', border: '1px solid #7a1818', color: '#e88' }}>
        ⚠ Items added to or removed from storage containers require a <strong>server zone restart</strong> to become visible to other players.
      </div>
    <div className="flex gap-4 flex-1 overflow-hidden min-h-0">
      {/* Container list */}
      <div
        className="w-64 shrink-0 flex flex-col overflow-hidden"
        style={{ background: 'var(--color-surface)', border: '1px solid #2a2418', borderRadius: 8 }}
      >
        <div className="flex items-center justify-between px-3 py-2 shrink-0" style={{ borderBottom: '1px solid #2a2418' }}>
          <span className="text-xs font-semibold uppercase" style={{ color: 'var(--color-primary)' }}>
            Containers ({containers.length})
          </span>
          <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
            {loading ? <Spinner size="sm" color="current" /> : '↻'}
          </Button>
        </div>
        <div className="px-2 py-1.5 shrink-0">
          <input
            className="w-full rounded px-2 py-1 text-xs border"
            style={{ background: '#0d0b07', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }}
            placeholder="Search..."
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
        </div>
        <div className="overflow-y-auto flex-1">
          {filtered.map(c => (
            <button
              key={c.id}
              onClick={() => selectContainer(c)}
              className="w-full text-left px-3 py-2 text-xs transition-colors"
              style={{
                background: selected?.id === c.id ? '#241e12' : 'transparent',
                borderBottom: '1px solid #1a1610',
                borderLeft: selected?.id === c.id ? '2px solid var(--color-primary)' : '2px solid transparent',
                color: 'var(--color-text)',
              }}
            >
              <div className="flex items-center justify-between gap-1">
                <span className="font-semibold truncate" style={{ color: selected?.id === c.id ? 'var(--color-primary)' : 'var(--color-text)' }}>
                  {c.name || `#${c.id}`}
                </span>
                <span className="text-xs px-1.5 py-0.5 rounded shrink-0" style={{ background: '#2a2418', color: 'var(--color-text-dim)' }}>
                  {c.item_count} items
                </span>
              </div>
              <div className="text-xs truncate mt-0.5" style={{ color: 'var(--color-text-dim)' }}>
                {c.name ? `#${c.id} · ` : ''}{shortClass(c.class)} · {c.map}
              </div>
            </button>
          ))}
        </div>
      </div>

      {/* Items panel */}
      <div className="flex-1 flex flex-col overflow-hidden min-h-0">
        {!selected ? (
          <div className="flex items-center justify-center h-full" style={{ color: 'var(--color-text-dim)' }}>
            <p className="text-sm">Select a container to view its contents</p>
          </div>
        ) : (
          <>
            <div className="flex items-center justify-between mb-3 shrink-0">
              <div>
                <h2 className="text-base font-semibold" style={{ color: 'var(--color-primary)' }}>
                  {selected.name || `Container #${selected.id}`}
                </h2>
                <p className="text-xs" style={{ color: 'var(--color-text-dim)' }}>
                  {selected.name ? `#${selected.id} · ` : ''}{shortClass(selected.class)} · {selected.map}
                </p>
              </div>
              <div className="flex gap-2">
                <Button size="sm" variant="ghost" onPress={() => selectContainer(selected)} isDisabled={itemsLoading}>
                  {itemsLoading ? <Spinner size="sm" color="current" /> : '↻ Refresh'}
                </Button>
                <Button size="sm" onPress={() => setShowGiveItems(true)}>
                  + Add Items
                </Button>
              </div>
            </div>

            {itemsLoading ? (
              <div className="flex justify-center py-12"><Spinner size="lg" /></div>
            ) : items.length === 0 ? (
              <div className="flex items-center justify-center flex-1" style={{ color: 'var(--color-text-dim)' }}>
                <p className="text-sm">Container is empty</p>
              </div>
            ) : (
              <div className="overflow-auto flex-1 rounded-lg" style={{ border: '1px solid #2a2418' }}>
                <table className="w-full text-xs">
                  <thead>
                    <tr style={{ background: '#1a1610', borderBottom: '1px solid #2a2418' }}>
                      {ITEM_COLUMNS.map(c => (
                        <th
                          key={c.key}
                          onClick={() => toggleItemSort(c.key)}
                          className="text-left px-3 py-2 font-semibold uppercase tracking-wide select-none"
                          style={{ color: 'var(--color-primary)', cursor: 'pointer' }}
                        >
                          {c.label}
                          <SortIndicator active={itemSortKey === c.key} dir={itemSortDir} />
                        </th>
                      ))}
                      <th className="text-left px-3 py-2 font-semibold uppercase tracking-wide" style={{ color: 'var(--color-primary)' }}></th>
                    </tr>
                  </thead>
                  <tbody>
                    {sortedItems.map((item, i) => (
                      <tr key={item.id} style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#0f0d09' }}>
                        <td className="px-3 py-1.5 font-mono" style={{ color: 'var(--color-text-dim)' }}>{item.id}</td>
                        <td className="px-3 py-1.5">
                          <div style={{ color: 'var(--color-text)' }}>{item.name || item.template_id}</div>
                          {item.name && <div className="text-xs font-mono" style={{ color: 'var(--color-text-dim)' }}>{item.template_id}</div>}
                        </td>
                        <td className="px-3 py-1.5" style={{ color: 'var(--color-text)' }}>{item.stack_size}</td>
                        <td className="px-3 py-1.5" style={{ color: 'var(--color-text)' }}>{item.quality}</td>
                        <td className="px-3 py-1.5" style={{ color: 'var(--color-text-dim)' }}>{item.durability}</td>
                        <td className="px-3 py-1.5">
                          <Button size="sm" variant="danger-soft" onPress={() => handleDeleteItem(item.id)}>Remove</Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </div>

      {/* Add Items Modal */}
      {selected && (
        <AddItemsModal
          container={selected}
          open={showGiveItems}
          onClose={() => setShowGiveItems(false)}
          onSuccess={() => { setShowGiveItems(false); selectContainer(selected) }}
          onRefresh={() => selectContainer(selected)}
        />
      )}
    </div>
    </div>
  )
}

function AddItemsModal({ container, open, onClose, onSuccess, onRefresh }: {
  container: Container;
  open: boolean;
  onClose: () => void;
  onSuccess: () => void;
  onRefresh: () => void;
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
      if (res.skipped.length === 0) {
        onSuccess()
      } else if (res.given.length > 0) {
        onRefresh()
      }
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
            <Modal.Header><Modal.Heading>Add Items — {container.name || `Container #${container.id}`}</Modal.Heading></Modal.Header>
            <Modal.Body style={{ display: 'flex', flexDirection: 'column', overflow: 'hidden', padding: '12px 16px' }}>
              {loading ? (
                <div className="flex justify-center py-6"><Spinner size="lg" /></div>
              ) : (
                <div className="flex flex-col gap-3 h-full overflow-hidden">
                  <div className="flex items-end gap-2 shrink-0">
                    <div className="flex flex-col gap-0.5 flex-1">
                      <span className="text-xs" style={{ color: 'var(--color-text-dim)' }}>Template</span>
                      <div className="relative">
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
                    </div>
                    <div className="flex flex-col items-center gap-0.5">
                      <span className="text-xs" style={{ color: 'var(--color-text-dim)' }}>Qty</span>
                      <input type="number" min={1} value={qty} onChange={e => setQty(Math.max(1, parseInt(e.target.value) || 1))}
                        className="rounded px-2 py-1.5 text-sm border w-16 text-center"
                        style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }} />
                    </div>
                    <div className="flex flex-col items-center gap-0.5">
                      <span className="text-xs" style={{ color: 'var(--color-text-dim)' }}>Quality</span>
                      <input type="number" min={0} value={quality} onChange={e => setQuality(Math.max(0, parseInt(e.target.value) || 0))}
                        className="rounded px-2 py-1.5 text-sm border w-16 text-center"
                        style={{ background: 'var(--color-surface)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }} />
                    </div>
                    <Button size="sm" onPress={addToStaged} isDisabled={!selected}>+ Add</Button>
                  </div>
                  {staged.length > 0 && (
                    <>
                      <div className="flex items-center gap-2 px-3 shrink-0">
                        <span className="flex-1" />
                        <span className="text-xs w-14 text-center" style={{ color: 'var(--color-text-dim)' }}>Qty</span>
                        <span className="text-xs w-14 text-center" style={{ color: 'var(--color-text-dim)' }}>Qual</span>
                        <span className="w-6" />
                      </div>
                      <div className="flex flex-col gap-1 overflow-y-auto flex-1">
                        {staged.map((item, idx) => (
                          <div key={idx} className="flex items-center gap-2 px-3 py-1.5 rounded text-xs" style={{ background: 'var(--color-surface)', border: '1px solid #2a2418' }}>
                            <span className="flex-1 font-mono">{item.template}</span>
                            <input type="number" min={1} value={item.qty} onChange={e => updateStaged(idx, 'qty', Math.max(1, parseInt(e.target.value) || 1))}
                              className="rounded px-2 py-1 border w-14 text-center"
                              style={{ background: 'var(--color-bg)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }} />
                            <input type="number" min={0} value={item.quality} onChange={e => updateStaged(idx, 'quality', Math.max(0, parseInt(e.target.value) || 0))}
                              className="rounded px-2 py-1 border w-14 text-center"
                              style={{ background: 'var(--color-bg)', color: 'var(--color-text)', borderColor: '#2a2418', outline: 'none' }} />
                            <button onClick={() => removeFromStaged(idx)} className="text-red-400 hover:text-red-300 px-1" style={{ cursor: 'pointer' }}>✕</button>
                          </div>
                        ))}
                      </div>
                    </>
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
