import { useState, useEffect, useMemo } from 'react'
import {
  Button, Chip, Input, InputGroup, Modal, Spinner, TextField, toast,
} from '@heroui/react'
import { api } from '../api/client'
import type { InventoryItem } from '../api/client'
import { DataTable, Icon, PageHeader, SideNav, type Column } from '../dune-ui'

type ItemKey = 'id' | 'template' | 'stack_size' | 'quality' | 'durability'

const ITEM_COLUMNS: Column<ItemKey>[] = [
  { key: 'id',         label: 'ID',         width: 100 },
  { key: 'template',   label: 'Template',   minWidth: 240 },
  { key: 'stack_size', label: 'Stack',      width: 100 },
  { key: 'quality',    label: 'Quality',    width: 100 },
  { key: 'durability', label: 'Durability', width: 130 },
]

type Container = {
  id: number; name: string; class: string; map: string; item_count: number
  item_templates: string[]; item_names: string[]; owner_name: string
}

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
  const [showAdd, setShowAdd] = useState(false)
  const [search, setSearch] = useState('')

  const load = async () => {
    setLoading(true)
    try {
      setContainers(await api.storage.list())
    } catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
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
      toast.danger(e instanceof Error ? e.message : String(e))
    } finally {
      setItemsLoading(false)
    }
  }

  const filtered = useMemo(() => {
    if (!search) return containers
    const q = search.toLowerCase()
    return containers.filter(c =>
      String(c.id).includes(q) ||
      c.map.toLowerCase().includes(q) ||
      shortClass(c.class).toLowerCase().includes(q) ||
      (c.name && c.name.toLowerCase().includes(q)) ||
      (c.owner_name && c.owner_name.toLowerCase().includes(q)) ||
      (c.item_templates ?? []).some(t => t.toLowerCase().includes(q)) ||
      (c.item_names ?? []).some(n => n.toLowerCase().includes(q)),
    )
  }, [containers, search])

  const navItems = filtered.map(c => ({
    key: String(c.id),
    label: c.name || `#${c.id}`,
    sublabel: [
      c.name ? `#${c.id}` : null,
      shortClass(c.class),
      c.map,
      c.owner_name || null,
    ].filter(Boolean).join(' · '),
    hint: <Chip size="sm" variant="soft">{c.item_count}</Chip>,
  }))

  return (
    <div className="flex flex-col gap-3 h-full min-h-0">
      <div className="flex gap-3 flex-1 min-h-0">
        <SideNav
          items={navItems}
          active={selected ? String(selected.id) : null}
          onSelect={id => {
            const c = containers.find(x => String(x.id) === id)
            if (c) selectContainer(c)
          }}
          title={`Containers (${containers.length})`}
          titleAction={
            <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
              {loading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
            </Button>
          }
          width="w-72"
        >
          <Input
            aria-label="Search containers"
            placeholder="Search..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="w-full"
          />
        </SideNav>

        <div className="flex-1 flex flex-col gap-3 min-h-0">
          {!selected ? (
            <div className="flex items-center justify-center h-full text-muted">
              <p className="text-sm">Select a container to view its contents</p>
            </div>
          ) : (
            <>
              <PageHeader
                title={selected.name || `Container #${selected.id}`}
                subtitle={[
                  selected.name ? `#${selected.id}` : null,
                  shortClass(selected.class),
                  selected.map,
                  selected.owner_name ? `Owner: ${selected.owner_name}` : null,
                ].filter(Boolean).join(' · ')}
              >
                <Button size="sm" variant="ghost" onPress={() => selectContainer(selected)} isDisabled={itemsLoading}>
                  {itemsLoading ? <Spinner size="sm" color="current" /> : <><Icon name="refresh-cw" /> Refresh</>}
                </Button>
              </PageHeader>

              {itemsLoading ? (
                <div className="flex justify-center py-12"><Spinner size="lg" /></div>
              ) : (
                <DataTable<InventoryItem, ItemKey>
                  aria-label="Container items"
                  className="min-h-0 max-h-full"
                  columns={ITEM_COLUMNS}
                  rows={items}
                  rowId={i => String(i.id)}
                  initialSort={{ column: 'id', direction: 'ascending' }}
                  sortValue={(i, k) => {
                    if (k === 'template') return i.name || i.template_id
                    return (i as unknown as Record<string, string | number>)[k]
                  }}
                  emptyState={<div className="py-8 text-center text-muted">Container is empty</div>}
                  renderCell={(i, key) => {
                    switch (key) {
                      case 'id':         return <span className="font-mono text-muted">{i.id}</span>
                      case 'template':
                        return (
                          <span className="inline-flex flex-col">
                            <span>{i.name || i.template_id}</span>
                            {i.name && <span className="text-xs font-mono text-muted">{i.template_id}</span>}
                          </span>
                        )
                      case 'stack_size': return <span>{i.stack_size}</span>
                      case 'quality':    return <span>{i.quality}</span>
                      case 'durability': return <span className="text-muted">{i.durability}</span>
                    }
                  }}
                />
              )}
            </>
          )}
        </div>
      </div>

      {selected && (
        <AddItemsModal
          container={selected}
          open={showAdd}
          onClose={() => setShowAdd(false)}
          onSuccess={() => { setShowAdd(false); selectContainer(selected) }}
          onRefresh={() => selectContainer(selected)}
        />
      )}
    </div>
  )
}

function AddItemsModal({ container, open, onClose, onSuccess, onRefresh }: {
  container: Container
  open: boolean
  onClose: () => void
  onSuccess: () => void
  onRefresh: () => void
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
      else if (res.given.length > 0) onRefresh()
    } catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal>
      <Modal.Backdrop isOpen={open} onOpenChange={v => !v && onClose()}>
        <Modal.Container size="cover">
          <Modal.Dialog className="flex flex-col">
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading className="text-accent">
                {container.name || `Container #${container.id}`} — Add Items
              </Modal.Heading>
            </Modal.Header>
            <Modal.Body className="flex flex-col gap-3 overflow-hidden">
              {loading ? (
                <div className="flex justify-center py-6"><Spinner size="lg" /></div>
              ) : (
                <>
                  <div className="flex items-center gap-3 shrink-0">
                    <TextField className="flex-1 min-w-0" aria-label="Template">
                      <div className="relative w-full">
                        <InputGroup className="w-full">
                          <InputGroup.Prefix>Template</InputGroup.Prefix>
                          <InputGroup.Input
                            className="flex-1 w-full"
                            placeholder="Search templates..."
                            value={query}
                            onChange={e => { setQuery(e.target.value); setSelected('') }}
                          />
                        </InputGroup>
                        {filtered.length > 0 && (
                          <div className="absolute z-50 w-full mt-1 rounded-[var(--radius)] border border-border bg-surface overflow-y-auto max-h-52">
                            {filtered.map(t => (
                              <div
                                key={t.id}
                                className="px-3 py-1.5 text-xs cursor-pointer hover:bg-surface-hover"
                                onClick={() => pick(t)}
                              >
                                <span className="font-mono">{t.id}</span>
                                {t.name ? <span className="text-muted">  —  {t.name}</span> : null}
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    </TextField>
                    <TextField className="w-32 shrink-0" aria-label="Quantity">
                      <InputGroup>
                        <InputGroup.Prefix>Qty</InputGroup.Prefix>
                        <InputGroup.Input
                          type="number" min={1} value={qty}
                          onChange={e => setQty(Math.max(1, parseInt(e.target.value) || 1))}
                        />
                      </InputGroup>
                    </TextField>
                    <TextField className="w-40 shrink-0" aria-label="Quality">
                      <InputGroup>
                        <InputGroup.Prefix>Quality</InputGroup.Prefix>
                        <InputGroup.Input
                          type="number" min={0} value={quality}
                          onChange={e => setQuality(Math.max(0, parseInt(e.target.value) || 0))}
                        />
                      </InputGroup>
                    </TextField>
                    <Button size="sm" onPress={addToStaged} isDisabled={!selected} className="shrink-0">
                      <Icon name="plus" /> Add
                    </Button>
                  </div>

                  {staged.length > 0 && (
                    <>
                      <div className="flex items-center gap-2 px-3 shrink-0">
                        <span className="flex-1" />
                        <span className="text-xs w-20 text-center text-muted">Qty</span>
                        <span className="text-xs w-20 text-center text-muted">Quality</span>
                        <span className="w-6" />
                      </div>
                      <div className="flex flex-col gap-1 overflow-y-auto flex-1 min-h-0">
                        {staged.map((item, idx) => (
                          <div
                            key={idx}
                            className="flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius)] text-xs bg-surface border border-border"
                          >
                            <span className="flex-1 font-mono">{item.template}</span>
                            <Input
                              type="number" min={1} value={item.qty}
                              onChange={e => updateStaged(idx, 'qty', Math.max(1, parseInt(e.target.value) || 1))}
                              aria-label={`Qty for ${item.template}`} className="w-20 text-center"
                            />
                            <Input
                              type="number" min={0} value={item.quality}
                              onChange={e => updateStaged(idx, 'quality', Math.max(0, parseInt(e.target.value) || 0))}
                              aria-label={`Quality for ${item.template}`} className="w-20 text-center"
                            />
                            <Button
                              size="sm" variant="danger-soft"
                              onPress={() => removeFromStaged(idx)}
                              aria-label="Remove"
                            >
                              <Icon name="x" />
                            </Button>
                          </div>
                        ))}
                      </div>
                    </>
                  )}

                  {result && (
                    <div className="text-xs shrink-0 rounded-[var(--radius)] px-3 py-2 bg-surface border border-border">
                      {result.given.length > 0 && (
                        <div className="text-success">✓ Added: {result.given.join(', ')}</div>
                      )}
                      {result.skipped.map((s, i) => (
                        <div key={i} className="text-danger">✕ Skipped {s.template}: {s.reason}</div>
                      ))}
                    </div>
                  )}
                </>
              )}
            </Modal.Body>
            <Modal.Footer>
              <Button variant="tertiary" size="sm" onPress={onClose}>Cancel</Button>
              <Button size="sm" onPress={handleSubmit} isDisabled={submitting || staged.length === 0}>
                {submitting ? <Spinner size="sm" color="current" /> : <Icon name="plus" />}
                Add {staged.length} Item{staged.length !== 1 ? 's' : ''}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
