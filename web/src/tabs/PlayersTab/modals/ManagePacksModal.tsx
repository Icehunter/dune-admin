import * as React from 'react'
import { Button, Chip, Header, Input, ListBox, Modal, SearchField, Select, Separator, Spinner, toast } from '@heroui/react'
import type { Selection } from '@heroui/react'
import type { DataGridColumn } from '@heroui-pro/react'
import { DataGrid } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { Icon, NumberInput, ActionBar } from '../../../dune-ui'
import { api } from '../../../api/client'
import type { GivePack, GivePackItem } from '../../../api/client'
import type { ManagePacksModalProps, PackDiff, KeyedItem, KeyedPack } from './types'

const stripKey = ({ template, qty, quality }: KeyedItem): GivePackItem => {
  return { template, qty, quality }
}

const stripPackKeys = (pack: KeyedPack): GivePack => {
  return { ...pack, items: pack.items.map(stripKey) }
}

const DiffStatus: React.FC<{ diff: PackDiff }> = ({ diff }) => {
  const parts: { key: string, text: string, cls: string }[] = []
  if (diff.added > 0) parts.push({ key: 'added', text: `${diff.added} added`, cls: 'text-success' })
  if (diff.updated > 0) parts.push({ key: 'updated', text: `${diff.updated} updated`, cls: 'text-warning' })
  if (diff.removed > 0) parts.push({ key: 'removed', text: `${diff.removed} removed`, cls: 'text-danger' })
  if (parts.length === 0) return null
  return (
    <span className="text-xs flex items-center gap-1">
      {parts.map((p, i) => (
        <span key={p.key} className="flex items-center gap-1">
          {i > 0 && <span className="text-muted">·</span>}
          <span className={p.cls}>{p.text}</span>
        </span>
      ))}
    </span>
  )
}

export const ManagePacksModal: React.FC<ManagePacksModalProps> = ({
  isOpen,
  onClose,
  onSaved,
  templates,
}) => {
  const { t } = useTranslation()
  const [packs, setPacks] = React.useState<KeyedPack[]>([])
  const [savedPacks, setSavedPacks] = React.useState<GivePack[]>([])
  const [selectedID, setSelectedID] = React.useState('')
  const [loading, setLoading] = React.useState(false)
  const [saving, setSaving] = React.useState(false)
  const [selectedKeys, setSelectedKeys] = React.useState<Selection>(new Set())

  const [formID, setFormID] = React.useState('')
  const [formName, setFormName] = React.useState('')
  const [formCategory, setFormCategory] = React.useState('')
  const [formTier, setFormTier] = React.useState(1)

  const [addQuery, setAddQuery] = React.useState('')
  const [addSelected, setAddSelected] = React.useState('')
  const [addQty, setAddQty] = React.useState(1)
  const [addQuality, setAddQuality] = React.useState(0)

  const keyCounter = React.useRef(0)
  const nextKey = () => String(keyCounter.current++)

  const loadPacks = React.useCallback(() => {
    setLoading(true)
    api.givePacks.config()
      .then((cfg) => {
        const loaded = cfg.packs ?? []
        const keyedPacks: KeyedPack[] = loaded.map((pack) => ({
          ...pack,
          items: pack.items.map((item) => ({ ...item, _key: nextKey() })),
        }))
        setPacks(keyedPacks)
        setSavedPacks(loaded)
        setSelectedID(keyedPacks[0]?.id ?? '')
      })
      .catch((e) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [])

  React.useEffect(() => {
    if (!isOpen) return
    void Promise.resolve().then(loadPacks)
  }, [isOpen, loadPacks])

  const packsRef = React.useRef(packs)
  React.useEffect(() => {
    packsRef.current = packs
  }, [packs])

  React.useEffect(() => {
    const pack = packsRef.current.find((p) => p.id === selectedID)
    if (pack) {
      setFormID(pack.id)
      setFormName(pack.name)
      setFormCategory(pack.category)
      setFormTier(pack.tier)
      setSelectedKeys(new Set())
    }
    else {
      setFormID('')
      setFormName('')
      setFormCategory('')
      setFormTier(1)
      setSelectedKeys(new Set())
    }
  }, [selectedID])

  const nameMap = React.useMemo(() => new Map(templates.map((tpl) => [tpl.id, tpl.name])), [templates])

  const sortedPacks = React.useMemo(
    () => [...packs].sort((a, b) => a.category.localeCompare(b.category) || a.tier - b.tier),
    [packs],
  )

  const groupedPacks = React.useMemo(() => {
    const groups: Record<string, KeyedPack[]> = {}
    for (const p of sortedPacks) {
      if (!groups[p.category]) groups[p.category] = []
      groups[p.category].push(p)
    }
    return Object.entries(groups)
  }, [sortedPacks])

  const packDiff = React.useMemo((): PackDiff => {
    const savedIds = new Set(savedPacks.map((p) => p.id))
    const currentIds = new Set(packs.map((p) => p.id))
    const savedMap = new Map(savedPacks.map((p) => [p.id, p]))
    const added = packs.filter((p) => !savedIds.has(p.id)).length
    const removed = savedPacks.filter((p) => !currentIds.has(p.id)).length
    const updated = packs.filter((p) => {
      if (!savedIds.has(p.id)) return false
      return JSON.stringify(stripPackKeys(p)) !== JSON.stringify(savedMap.get(p.id))
    }).length
    return { added, updated, removed, isDirty: added + updated + removed > 0 }
  }, [packs, savedPacks])

  const selectedPack = packs.find((p) => p.id === selectedID)
  const items: KeyedItem[] = selectedPack?.items ?? []

  const setItems = (next: KeyedItem[]) => {
    setPacks(packs.map((p) => (p.id === selectedID ? { ...p, items: next } : p)))
  }

  const addFiltered = React.useMemo(() => {
    if (!addQuery) return []
    const q = addQuery.toLowerCase()
    return templates
      .filter((tpl) => tpl.id.toLowerCase().includes(q) || tpl.name.toLowerCase().includes(q))
      .slice(0, 100)
  }, [templates, addQuery])

  const pickTemplate = (tpl: { id: string, name: string }) => {
    setAddSelected(tpl.id)
    setAddQuery(tpl.name ? `${tpl.id}  —  ${tpl.name}` : tpl.id)
  }

  const addItem = () => {
    if (!addSelected) return
    setItems([...items, { template: addSelected, qty: addQty, quality: addQuality, _key: nextKey() }])
    setAddQuery('')
    setAddSelected('')
    setAddQty(1)
    setAddQuality(0)
  }

  const removeItem = (key: string) => {
    setItems(items.filter((it) => it._key !== key))
    setSelectedKeys((prev) => {
      if (prev === 'all') return new Set(items.filter((it) => it._key !== key).map((it) => it._key))
      const next = new Set(prev as Set<string>)
      next.delete(key)
      return next
    })
  }

  const setItem = (key: string, patch: Partial<GivePackItem>) =>
    setItems(items.map((it) => (it._key === key ? { ...it, ...patch } : it)))

  const selectionCount = selectedKeys === 'all' ? items.length : (selectedKeys as Set<string>).size

  const handleBulkDelete = () => {
    if (selectedKeys === 'all') {
      setItems([])
    }
    else {
      const keys = selectedKeys as Set<string>
      setItems(items.filter((it) => !keys.has(it._key)))
    }
    setSelectedKeys(new Set())
  }

  const isUpdating = selectedID !== '' && formID.trim() === selectedID

  const applyPack = () => {
    const id = formID.trim()
    const name = formName.trim()
    const category = formCategory.trim()
    if (!id || !name || !category) return
    if (isUpdating) {
      setPacks((prev) => prev.map((p) =>
        p.id === selectedID ? { ...p, id, name, category, tier: formTier } : p,
      ))
      setSelectedID(id)
    }
    else {
      if (packs.some((p) => p.id === id)) {
        toast.warning(t('players.givePacks.duplicateId'))
        return
      }
      setPacks((prev) => [...prev, { id, name, category, tier: formTier, items: [] }])
      setSelectedID(id)
    }
  }

  const clearPackForm = () => {
    setFormID('')
    setFormName('')
    setFormCategory('')
    setFormTier(1)
  }

  const deletePack = (id: string) => {
    const next = packs.filter((p) => p.id !== id)
    setPacks(next)
    if (selectedID === id) setSelectedID(next[0]?.id ?? '')
  }

  const save = async () => {
    setSaving(true)
    try {
      const cfg = await api.givePacks.saveConfig({ packs: packs.map(stripPackKeys) })
      setSavedPacks(cfg.packs)
      toast.success(t('players.givePacks.saved'))
      onSaved(cfg.packs)
    }
    catch (e) {
      toast.danger(t('players.givePacks.saveFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setSaving(false)
    }
  }

  const columns: DataGridColumn<KeyedItem>[] = [
    {
      id: 'template',
      isRowHeader: true,
      header: t('players.inventory.columns.template'),
      minWidth: 200,
      allowsResizing: true,
      cell: (item) => (
        <div className="leading-tight py-0.5">
          <div className="truncate text-sm">{nameMap.get(item.template) || item.template}</div>
          {nameMap.get(item.template) && (
            <div className="font-mono text-[10px] text-muted truncate">{item.template}</div>
          )}
        </div>
      ),
    },
    {
      id: 'qty',
      header: t('players.give.qty'),
      minWidth: 130,
      maxWidth: 250,
      allowsResizing: true,
      cell: (item) => (
        <NumberInput
          ariaLabel={t('players.give.qty')}
          min={1}
          value={item.qty}
          onChange={(v) => setItem(item._key, { qty: v })}
          className="w-full"
        />
      ),
    },
    {
      id: 'quality',
      header: t('players.give.quality'),
      minWidth: 130,
      maxWidth: 250,
      allowsResizing: true,
      cell: (item) => (
        <NumberInput
          ariaLabel={t('players.give.quality')}
          min={0}
          value={item.quality}
          onChange={(v) => setItem(item._key, { quality: v })}
          className="w-full"
        />
      ),
    },
    {
      id: 'actions',
      header: '',
      width: 52,
      cell: (item) => (
        <Button
          size="sm"
          variant="danger-soft"
          isIconOnly
          onPress={() => removeItem(item._key)}
          aria-label={t('players.givePacks.removeItem')}
        >
          <Icon name="trash" />
        </Button>
      ),
    },
  ]

  if (!isOpen) return null

  return (
    <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen onOpenChange={(v) => { if (!v) onClose() }}>
      <Modal.Container size="cover" scroll="outside">
        <Modal.Dialog className="p-10 dialog-surface-alt">
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading className="text-accent">{t('players.givePacks.title')}</Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-4 h-[80vh] min-h-0">
            {loading
              ? <Spinner size="sm" color="current" />
              : (
                  <div className="flex flex-col h-full min-h-0 gap-3">

                    {packDiff.isDirty && (
                      <div className="shrink-0 rounded-[var(--radius)] px-4 py-2 text-xs font-medium bg-warning/10 border border-warning/40 text-warning flex items-center gap-2">
                        <Icon name="triangle-alert" />
                        <span>You have unsaved changes — click Save Config to persist them.</span>
                      </div>
                    )}

                    {/* Pack picker + metadata */}
                    <div className="flex flex-wrap items-end gap-2 shrink-0 pb-1 border-b border-border">
                      <Select
                        aria-label={t('players.givePacks.editingPack')}
                        selectedKey={selectedID || null}
                        onSelectionChange={(k) => setSelectedID(k ? String(k) : '')}
                        className="w-56"
                      >
                        <Select.Trigger>
                          <Select.Value>
                            {!selectedID
                              ? '— select —'
                              : selectedPack
                                ? `${selectedPack.category} — ${selectedPack.name}`
                                : selectedID}
                          </Select.Value>
                          <Select.Indicator />
                        </Select.Trigger>
                        <Select.Popover>
                          <ListBox>
                            <ListBox.Item key="_none" id="" textValue="— select —">
                              — select —
                              <ListBox.ItemIndicator />
                            </ListBox.Item>
                            {groupedPacks.map(([cat, catPacks], i) => (
                              <ListBox.Section key={cat}>
                                <Header>{cat}</Header>
                                {catPacks.map((p) => (
                                  <ListBox.Item key={p.id} id={p.id} textValue={`${cat} — ${p.name}`}>
                                    {p.name}
                                    <ListBox.ItemIndicator />
                                  </ListBox.Item>
                                ))}
                                {i < groupedPacks.length - 1 && <Separator />}
                              </ListBox.Section>
                            ))}
                          </ListBox>
                        </Select.Popover>
                      </Select>
                      {selectedID && (
                        <Button size="sm" variant="ghost" onPress={() => deletePack(selectedID)} aria-label={t('players.givePacks.deletePack')}>
                          <Icon name="trash-2" />
                        </Button>
                      )}
                      <Button size="sm" variant="ghost" onPress={clearPackForm} aria-label={t('players.givePacks.newPack')}>
                        <Icon name="file-plus" />
                        {' '}
                        {t('players.givePacks.newPack')}
                      </Button>
                      <div className="flex flex-col gap-1">
                        <span className="text-xs text-muted">{t('players.givePacks.packId')}</span>
                        <Input
                          className="w-28"
                          aria-label={t('players.givePacks.packId')}
                          placeholder={t('players.givePacks.packId')}
                          value={formID}
                          onChange={(e) => setFormID(e.target.value)}
                          onKeyDown={(e) => { if (e.key === 'Enter') applyPack() }}
                        />
                      </div>
                      <div className="flex flex-col gap-1">
                        <span className="text-xs text-muted">{t('players.givePacks.packName')}</span>
                        <Input
                          className="w-24"
                          aria-label={t('players.givePacks.packName')}
                          placeholder={t('players.givePacks.packName')}
                          value={formName}
                          onChange={(e) => setFormName(e.target.value)}
                          onKeyDown={(e) => { if (e.key === 'Enter') applyPack() }}
                        />
                      </div>
                      <div className="flex flex-col gap-1">
                        <span className="text-xs text-muted">{t('players.givePacks.category')}</span>
                        <Input
                          className="w-28"
                          aria-label={t('players.givePacks.category')}
                          placeholder={t('players.givePacks.category')}
                          value={formCategory}
                          onChange={(e) => setFormCategory(e.target.value)}
                          onKeyDown={(e) => { if (e.key === 'Enter') applyPack() }}
                        />
                      </div>
                      <NumberInput label={t('players.givePacks.tier')} ariaLabel={t('players.givePacks.tier')} min={1} value={formTier} onChange={setFormTier} className="w-24" />
                      <Button
                        size="sm"
                        onPress={applyPack}
                        isDisabled={!formID.trim() || !formName.trim() || !formCategory.trim()}
                      >
                        <Icon name={isUpdating ? 'check' : 'plus'} />
                        {' '}
                        {isUpdating ? t('players.givePacks.updatePack') : t('players.givePacks.addPack')}
                      </Button>
                    </div>

                    {/* Item add row */}
                    {selectedID && (
                      <div className="flex items-center gap-2 shrink-0">
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
                              <SearchField.Input placeholder={t('players.givePacks.searchTemplates')} />
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
                                  {tpl.name && (
                                    <span className="text-muted">
                                      {' — '}
                                      {tpl.name}
                                    </span>
                                  )}
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                        <NumberInput prefix={t('players.give.qty')} ariaLabel={t('players.give.qty')} min={1} value={addQty} onChange={setAddQty} className="w-48 shrink-0" />
                        <NumberInput prefix={t('players.give.quality')} ariaLabel={t('players.give.quality')} min={0} value={addQuality} onChange={setAddQuality} className="w-48 shrink-0" />
                        <Button size="sm" onPress={addItem} isDisabled={!addSelected} className="shrink-0">
                          <Icon name="plus" />
                          {' '}
                          {t('players.givePacks.addItem')}
                        </Button>
                      </div>
                    )}

                    {/* Item DataGrid */}
                    {packs.length === 0
                      ? <p className="text-xs text-muted shrink-0">{t('players.givePacks.noPacks')}</p>
                      : !selectedID
                          ? <p className="text-xs text-muted shrink-0">{t('players.givePacks.noPackSelected')}</p>
                          : items.length === 0
                            ? <p className="text-xs text-muted shrink-0">{t('players.givePacks.noItemsYet')}</p>
                            : (
                                <DataGrid
                                  aria-label={t('players.givePacks.title')}
                                  columns={columns}
                                  data={items}
                                  getRowId={(item) => item._key}
                                  selectedKeys={selectedKeys}
                                  selectionMode="multiple"
                                  showSelectionCheckboxes
                                  onSelectionChange={setSelectedKeys}
                                  className="flex-1 min-h-0"
                                  scrollContainerClassName="h-full overflow-y-auto"
                                  allowsColumnResize
                                />
                              )}

                    {/* Save button + diff status */}
                    <div className="pt-3 shrink-0 border-t border-border flex items-center gap-3">
                      <Button size="sm" onPress={save} isDisabled={saving}>
                        {saving
                          ? <Spinner size="sm" color="current" />
                          : <Icon name="save" />}
                        {' '}
                        {t('players.givePacks.save')}
                      </Button>
                      <DiffStatus diff={packDiff} />
                    </div>

                  </div>
                )}
          </Modal.Body>
        </Modal.Dialog>
      </Modal.Container>

      <ActionBar aria-label={t('players.givePacks.title')} isOpen={selectionCount > 0}>
        <ActionBar.Prefix>
          <Chip size="sm" className="shrink-0 tabular-nums">{selectionCount}</Chip>
        </ActionBar.Prefix>
        <Separator />
        <ActionBar.Content>
          <Button
            size="sm"
            variant="ghost"
            className="text-danger"
            onPress={handleBulkDelete}
            aria-label={t('common.deleteSelected')}
          >
            <Icon name="trash-2" />
            <span className="action-bar__label">{t('common.deleteSelected')}</span>
          </Button>
        </ActionBar.Content>
        <Separator />
        <ActionBar.Suffix>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            onPress={() => setSelectedKeys(new Set())}
            aria-label={t('common.clearSelection')}
          >
            <Icon name="x" />
          </Button>
        </ActionBar.Suffix>
      </ActionBar>
    </Modal.Backdrop>
  )
}
