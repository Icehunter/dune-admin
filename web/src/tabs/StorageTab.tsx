import { useState, useEffect, useMemo, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button, Chip, Modal, SearchField, Spinner, TextField, toast,
} from '@heroui/react'
import { api } from '../api/client'
import type { InventoryItem } from '../api/client'
import { DataTable, Icon, NumberInput, PageHeader, SideNav, type Column } from '../dune-ui'

type ItemKey = 'id' | 'template' | 'stack_size' | 'quality' | 'durability' | 'actions'

type Container = {
  id: number
  name: string
  class: string
  map: string
  item_count: number
  item_templates: string[]
  item_names: string[]
  owner_name: string
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
  const { t } = useTranslation()

  const ITEM_COLUMNS: Column<ItemKey>[] = [
    { key: 'id', label: t('storage.columns.id'), width: 100 },
    { key: 'template', label: t('storage.columns.template'), minWidth: 240 },
    { key: 'stack_size', label: t('storage.columns.stack'), width: 100 },
    { key: 'quality', label: t('storage.columns.quality'), width: 100 },
    { key: 'durability', label: t('storage.columns.durability'), width: 130 },
    { key: 'actions', label: '', width: 120, sortable: false },
  ]

  const [containers, setContainers] = useState<Container[]>([])
  const [loading, setLoading] = useState(false)
  const [selected, setSelected] = useState<Container | null>(null)
  const [items, setItems] = useState<InventoryItem[]>([])
  const [itemsLoading, setItemsLoading] = useState(false)
  const [showAdd, setShowAdd] = useState(false)
  const [search, setSearch] = useState('')

  const load = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.storage.list())
      .then(setContainers)
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const selectContainer = async (c: Container) => {
    setSelected(c)
    setItemsLoading(true)
    try {
      setItems(await api.storage.items(c.id))
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
    finally {
      setItemsLoading(false)
    }
  }

  const handleDeleteItem = async (itemId: number) => {
    try {
      await api.players.deleteItem(itemId)
      setItems((prev) => prev.filter((i) => i.id !== itemId))
      if (selected) {
        setContainers((prev) => prev.map((c) => c.id === selected.id ? { ...c, item_count: c.item_count - 1 } : c))
      }
      toast.success(t('storage.itemRemoved'))
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
  }

  const filtered = useMemo(() => {
    if (!search) return containers
    const q = search.toLowerCase()
    return containers.filter((c) =>
      String(c.id).includes(q)
      || c.map.toLowerCase().includes(q)
      || shortClass(c.class).toLowerCase().includes(q)
      || (c.name && c.name.toLowerCase().includes(q))
      || (c.owner_name && c.owner_name.toLowerCase().includes(q))
      || (c.item_templates ?? []).some((tmpl) => tmpl.toLowerCase().includes(q))
      || (c.item_names ?? []).some((n) => n.toLowerCase().includes(q)),
    )
  }, [containers, search])

  const navItems = useMemo(() => filtered.map((c) => ({
    key: String(c.id),
    label: c.name || `#${c.id}`,
    sublabel: [
      c.name ? `#${c.id}` : null,
      shortClass(c.class),
      c.map,
      c.owner_name || null,
    ].filter(Boolean).join(' · '),
    hint: <Chip size="sm" variant="soft">{c.item_count}</Chip>,
  })), [filtered])

  return (
    <div className="flex flex-col gap-3 h-full min-h-0">
      {/* Warning banner */}
      <div className="shrink-0 rounded-[var(--radius)] px-4 py-2 text-xs font-medium bg-danger/10 border border-danger/40 text-danger flex items-center gap-2">
        <Icon name="triangle-alert" />
        <span>{t('storage.warningText')}</span>
      </div>

      <div className="flex gap-3 flex-1 min-h-0">
        <SideNav
          items={navItems}
          active={selected ? String(selected.id) : null}
          onSelect={(id) => {
            const c = containers.find((x) => String(x.id) === id)
            if (c) selectContainer(c)
          }}
          title={t('storage.containersTitle', { count: containers.length })}
          titleAction={(
            <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
              {loading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
            </Button>
          )}
          width="w-60"
        >
          <SearchField
            aria-label={t('storage.searchLabel')}
            value={search}
            onChange={setSearch}
            className="w-full"
          >
            <SearchField.Group>
              <SearchField.SearchIcon />
              <SearchField.Input placeholder={t('storage.searchPlaceholder')} />
              <SearchField.ClearButton />
            </SearchField.Group>
          </SearchField>
        </SideNav>

        <div className="flex-1 flex flex-col gap-3 min-h-0">
          {!selected
            ? (
                <div className="flex items-center justify-center h-full text-muted">
                  <p className="text-sm">{t('storage.selectContainer')}</p>
                </div>
              )
            : (
                <>
                  <PageHeader
                    title={selected.name || t('storage.containerTitle', { id: selected.id })}
                    subtitle={[
                      selected.name ? `#${selected.id}` : null,
                      shortClass(selected.class),
                      selected.map,
                      selected.owner_name ? t('storage.ownerLabel', { name: selected.owner_name }) : null,
                    ].filter(Boolean).join(' · ')}
                  >
                    <Button size="sm" variant="ghost" onPress={() => selectContainer(selected)} isDisabled={itemsLoading}>
                      {itemsLoading
                        ? <Spinner size="sm" color="current" />
                        : (
                            <>
                              <Icon name="refresh-cw" />
                              {' '}
                              {t('common.refresh')}
                            </>
                          )}
                    </Button>
                    <Button size="sm" onPress={() => setShowAdd(true)}>
                      <Icon name="plus" />
                      {' '}
                      {t('storage.addItems')}
                    </Button>
                  </PageHeader>

                  {itemsLoading
                    ? (
                        <div className="flex justify-center py-12"><Spinner size="lg" /></div>
                      )
                    : (
                        <DataTable<InventoryItem, ItemKey>
                          aria-label={t('storage.ariaLabel')}
                          className="min-h-0 max-h-full"
                          columns={ITEM_COLUMNS}
                          rows={items}
                          rowId={(i) => String(i.id)}
                          initialSort={{ column: 'id', direction: 'ascending' }}
                          sortValue={(i, k) => {
                            if (k === 'template') return i.name || i.template_id
                            if (k === 'actions') return ''
                            return (i as unknown as Record<string, string | number>)[k]
                          }}
                          emptyState={<div className="py-8 text-center text-muted">{t('storage.containerEmpty')}</div>}
                          renderCell={(i, key) => {
                            switch (key) {
                              case 'id': return <span className="font-mono text-muted">{i.id}</span>
                              case 'template':
                                return (
                                  <span className="inline-flex flex-col">
                                    <span>{i.name || i.template_id}</span>
                                    {i.name && <span className="text-xs font-mono text-muted">{i.template_id}</span>}
                                  </span>
                                )
                              case 'stack_size': return <span>{i.stack_size}</span>
                              case 'quality': return <span>{i.quality}</span>
                              case 'durability': return <span className="text-muted">{i.durability}</span>
                              case 'actions':
                                return (
                                  <Button
                                    size="sm"
                                    variant="danger-soft"
                                    className="w-full"
                                    onPress={() => handleDeleteItem(i.id)}
                                  >
                                    <Icon name="x" />
                                    {' '}
                                    {t('storage.remove')}
                                  </Button>
                                )
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
          onSuccess={() => {
            setShowAdd(false)
            selectContainer(selected)
          }}
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
  const { t } = useTranslation()
  const [templates, setTemplates] = useState<{ id: string, name: string }[]>([])
  const [loading, setLoading] = useState(false)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState('')
  const [qty, setQty] = useState(1)
  const [quality, setQuality] = useState(0)
  const [staged, setStaged] = useState<{ template: string, qty: number, quality: number }[]>([])
  const [submitting, setSubmitting] = useState(false)
  type AddResult = { given: string[], skipped: { template: string, reason: string }[] } | null
  const [result, setResult] = useState<AddResult>(null)

  useEffect(() => {
    if (!open) return
    Promise.resolve()
      .then(() => {
        setLoading(true)
        setQuery('')
        setSelected('')
        setQty(1)
        setQuality(0)
        setStaged([])
        setResult(null)
      })
      .then(() => api.players.templates())
      .then(setTemplates)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [open])

  const filtered = useMemo(() => {
    if (!query) return []
    const q = query.toLowerCase()
    return templates
      .filter((tmpl) => tmpl.id.toLowerCase().includes(q) || tmpl.name.toLowerCase().includes(q))
      .slice(0, 100)
  }, [templates, query])

  const pick = (tmpl: { id: string, name: string }) => {
    setSelected(tmpl.id)
    setQuery(tmpl.name ? `${tmpl.id}  —  ${tmpl.name}` : tmpl.id)
  }

  const addToStaged = () => {
    if (!selected) {
      toast.warning(t('storage.addModal.selectTemplate'))
      return
    }
    setStaged((prev) => [...prev, { template: selected, qty, quality }])
    setQuery('')
    setSelected('')
    setQty(1)
    setQuality(0)
  }

  const removeFromStaged = (idx: number) => {
    setStaged((prev) => prev.filter((_, i) => i !== idx))
  }

  const updateStaged = (idx: number, field: 'qty' | 'quality', value: number) => {
    setStaged((prev) => prev.map((item, i) => i === idx ? { ...item, [field]: value } : item))
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
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
    finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal>
      <Modal.Backdrop isOpen={open} onOpenChange={(v) => !v && onClose()}>
        <Modal.Container size="cover" scroll="outside">
          <Modal.Dialog>
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading className="text-accent">
                {container.name || t('storage.containerTitle', { id: container.id })}
                {' '}
                —
                {' '}
                {t('storage.addItems')}
              </Modal.Heading>
            </Modal.Header>
            <Modal.Body className="flex flex-col gap-3">
              {loading
                ? (
                    <div className="flex justify-center py-6"><Spinner size="lg" /></div>
                  )
                : (
                    <>
                      <div className="flex items-end gap-3 shrink-0">
                        <TextField className="flex-1 min-w-0" aria-label={t('storage.addModal.templateLabel')}>
                          <div className="relative w-full">
                            <SearchField
                              className="w-full"
                              value={query}
                              onChange={(v) => {
                                setQuery(v)
                                setSelected('')
                              }}
                            >
                              <SearchField.Group>
                                <SearchField.SearchIcon />
                                <SearchField.Input placeholder={t('storage.addModal.searchPlaceholder')} />
                                <SearchField.ClearButton />
                              </SearchField.Group>
                            </SearchField>
                            {filtered.length > 0 && (
                              <div className="absolute z-50 w-full mt-1 rounded-[var(--radius)] border border-border bg-surface overflow-y-auto max-h-52">
                                {filtered.map((tmpl) => (
                                  <div
                                    key={tmpl.id}
                                    className="px-3 py-1.5 text-xs cursor-pointer hover:bg-surface-hover"
                                    onClick={() => pick(tmpl)}
                                  >
                                    <span className="font-mono">{tmpl.id}</span>
                                    {tmpl.name
                                      ? (
                                          <span className="text-muted">
                                            {' '}
                                            —
                                            {tmpl.name}
                                          </span>
                                        )
                                      : null}
                                  </div>
                                ))}
                              </div>
                            )}
                          </div>
                        </TextField>
                        <NumberInput
                          prefix={t('storage.addModal.qtyLabel')}
                          ariaLabel={t('storage.addModal.qtyLabel')}
                          min={1}
                          value={qty}
                          onChange={setQty}
                          className="w-56 shrink-0"
                        />
                        <NumberInput
                          prefix={t('storage.addModal.qualityLabel')}
                          ariaLabel={t('storage.addModal.qualityLabel')}
                          min={0}
                          value={quality}
                          onChange={setQuality}
                          className="w-56 shrink-0"
                        />
                        <Button size="sm" onPress={addToStaged} isDisabled={!selected} className="shrink-0">
                          <Icon name="plus" />
                          {' '}
                          {t('storage.addModal.add')}
                        </Button>
                      </div>

                      {staged.length > 0 && (
                        <>
                          <div className="flex flex-col gap-1 overflow-y-auto flex-1 min-h-0">
                            {staged.map((item, idx) => (
                              <div
                                key={idx}
                                className="flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius)] text-xs bg-surface border border-border"
                              >
                                <span className="flex-1 font-mono">{item.template}</span>
                                <NumberInput
                                  ariaLabel={`Qty for ${item.template}`}
                                  prefix={t('storage.addModal.qtyColLabel')}
                                  min={1}
                                  value={item.qty}
                                  onChange={(v) => updateStaged(idx, 'qty', v)}
                                  className="w-56"
                                />
                                <NumberInput
                                  ariaLabel={`Quality for ${item.template}`}
                                  prefix={t('storage.addModal.qualityColLabel')}
                                  min={0}
                                  value={item.quality}
                                  onChange={(v) => updateStaged(idx, 'quality', v)}
                                  className="w-56"
                                />
                                <Button
                                  size="sm"
                                  variant="danger-soft"
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
                            <div className="text-success">
                              ✓ Added:
                              {result.given.join(', ')}
                            </div>
                          )}
                          {result.skipped.map((s, i) => (
                            <div key={i} className="text-danger">
                              ✕ Skipped
                              {s.template}
                              :
                              {s.reason}
                            </div>
                          ))}
                        </div>
                      )}
                    </>
                  )}
            </Modal.Body>
            <Modal.Footer>
              <Button variant="tertiary" size="sm" slot="close">{t('common.cancel')}</Button>
              <Button size="sm" onPress={handleSubmit} isDisabled={submitting || staged.length === 0}>
                {submitting ? <Spinner size="sm" color="current" /> : <Icon name="plus" />}
                {t('storage.addModal.add')}
                {' '}
                {staged.length}
                {' '}
                Item
                {staged.length !== 1 ? 's' : ''}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
