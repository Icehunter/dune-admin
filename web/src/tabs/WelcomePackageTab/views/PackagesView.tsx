import * as React from 'react'
import { Button, Chip, Input, ListBox, SearchField, Select, Separator, Spinner } from '@heroui/react'
import type { Selection } from '@heroui/react'
import type { DataGridColumn } from '@heroui-pro/react'
import { DataGrid } from '@heroui-pro/react'
import { useAtomValue } from 'jotai'
import { useTranslation } from 'react-i18next'
import { usePermissions } from '../../../hooks/usePermissions'
import { itemDataSyncAtom } from '../../../data/store'
import { ActionBar, Icon, NumberInput, PageHeader } from '../../../dune-ui'
import type { WelcomePackage } from '../../../api/client'
import { DiffStatus } from '../components/DiffStatus'
import type { PackagesViewProps, KeyedItem } from './types'

export const PackagesView: React.FC<PackagesViewProps> = ({
  packages,
  setPackages,
  activeVersions,
  templates,
  save,
  saving,
  load,
  loading,
  configDiff,
}) => {
  const { t } = useTranslation()
  const { can } = usePermissions()

  const [selected, setSelected] = React.useState(() => packages[0]?.version ?? '')
  const [newName, setNewName] = React.useState('')
  const [addQuery, setAddQuery] = React.useState('')
  const [addSelected, setAddSelected] = React.useState('')
  const [addQty, setAddQty] = React.useState(1)
  const [addQuality, setAddQuality] = React.useState(0)
  const [selectedKeys, setSelectedKeys] = React.useState<Selection>(new Set())

  const keyCounter = React.useRef(0)
  const nextKey = () => String(keyCounter.current++)

  // Clear selection when selected package changes
  React.useEffect(() => {
    void Promise.resolve().then(() => setSelectedKeys(new Set()))
  }, [selected])

  const nameMap = React.useMemo(() => new Map(templates.map((tpl) => [tpl.id, tpl.name])), [templates])

  const itemData = useAtomValue(itemDataSyncAtom)
  const defaultVolume = itemData.default_volume ?? 0
  const defaultStackMax = itemData.default_stack_max ?? 0

  // Per-row volume (volume × qty) — the inventory-load metric labelled "weight/volume".
  const rowVolume = React.useCallback((template: string, qty: number): number => {
    const v = itemData.items[template]?.volume ?? defaultVolume
    return v > 0 ? v * qty : 0
  }, [itemData, defaultVolume])

  // Per-row slot need = ceil(qty / stack_max). Falls back to one slot per unit
  // when no stack size is known.
  const rowSlots = React.useCallback((template: string, qty: number): number => {
    const stackMax = itemData.items[template]?.stack_max ?? defaultStackMax
    if (stackMax > 0) return Math.ceil(qty / stackMax)
    return qty
  }, [itemData, defaultStackMax])

  // Derive keyed items from the selected package (index-based keys, cleared on any removal)
  const keyedItems = React.useMemo(() => {
    const pkg = packages.find((p) => p.version === selected)
    return (pkg?.items ?? []).map((it, i) => ({ ...it, _key: String(i) }))
  }, [packages, selected])

  const totals = React.useMemo(() => {
    let volume = 0
    let slots = 0
    for (const it of keyedItems) {
      volume += rowVolume(it.template, it.qty)
      slots += rowSlots(it.template, it.qty)
    }
    return { volume, slots }
  }, [keyedItems, rowVolume, rowSlots])

  const setItems = (next: KeyedItem[]) => {
    const stripped = next.map(({ template, qty, quality }) => ({ template, qty, quality }))
    setPackages(packages.map((p) => (p.version === selected ? { ...p, items: stripped } : p)))
  }

  const removeItem = (key: string) => {
    setItems(keyedItems.filter((it) => it._key !== key))
    setSelectedKeys(new Set())
  }

  const setItem = (key: string, patch: Partial<KeyedItem>) => {
    setItems(keyedItems.map((it) => (it._key === key ? { ...it, ...patch } : it)))
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
    setItems([...keyedItems, { template: addSelected, qty: addQty, quality: addQuality, _key: nextKey() }])
    setAddQuery('')
    setAddSelected('')
    setAddQty(1)
    setAddQuality(0)
  }

  const addVersion = () => {
    const name = newName.trim()
    if (!name || packages.some((p) => p.version === name)) return
    const next: WelcomePackage[] = [...packages, { version: name, items: [] }]
    setPackages(next)
    setSelected(name)
    setNewName('')
  }

  const deleteVersion = (v: string) => {
    const next = packages.filter((p) => p.version !== v)
    setPackages(next)
    if (selected === v) setSelected(next[0]?.version ?? '')
  }

  const selectionCount = selectedKeys === 'all' ? keyedItems.length : (selectedKeys as Set<string>).size

  const handleBulkDelete = () => {
    if (selectedKeys === 'all') {
      setItems([])
    }
    else {
      const keys = selectedKeys as Set<string>
      setItems(keyedItems.filter((it) => !keys.has(it._key)))
    }
    setSelectedKeys(new Set())
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
      id: 'weight',
      header: t('welcome.kit.weightVolume'),
      minWidth: 110,
      maxWidth: 160,
      allowsResizing: true,
      cell: (item) => {
        const v = rowVolume(item.template, item.qty)
        return v > 0
          ? <span className="text-muted text-xs tabular-nums">{Math.round(v * 100) / 100}</span>
          : <span className="text-muted">—</span>
      },
    },
    {
      id: 'slots',
      header: t('welcome.kit.slots'),
      minWidth: 90,
      maxWidth: 140,
      allowsResizing: true,
      cell: (item) => {
        const stackMax = itemData.items[item.template]?.stack_max ?? defaultStackMax
        const slots = rowSlots(item.template, item.qty)
        return (
          <span className="text-muted text-xs tabular-nums">
            {slots}
            {stackMax > 0 ? ` (${t('welcome.kit.stackOf', { n: stackMax })})` : ''}
          </span>
        )
      },
    },
    {
      id: 'actions',
      header: '',
      width: 52,
      cell: (item) => (
        can('welcome:manage')
          ? (
              <Button
                size="sm"
                variant="danger-soft"
                isIconOnly
                onPress={() => removeItem(item._key)}
                aria-label={t('welcome.removeItem')}
              >
                <Icon name="trash" />
              </Button>
            )
          : null
      ),
    },
  ]

  return (
    <div className="flex flex-col h-full min-h-0">
      <PageHeader title={t('welcome.sections.packages')} subtitle={t('welcome.packagesSubtitle')}>
        <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
          {loading
            ? <Spinner size="sm" color="current" />
            : (
                <>
                  <Icon name="refresh-cw" />
                  {' '}
                  {t('common.refresh')}
                </>
              )}
        </Button>
      </PageHeader>

      {configDiff.isDirty && (
        <div className="shrink-0 rounded-[var(--radius)] mb-3 px-4 py-2 text-xs font-medium bg-warning/10 border border-warning/40 text-warning flex items-center gap-2">
          <Icon name="triangle-alert" />
          <span>You have unsaved changes — click Save Config to persist them.</span>
        </div>
      )}

      {/* Version picker + new version input */}
      <div className="flex flex-wrap items-end gap-3 pb-3 shrink-0">
        <div className="flex items-end gap-2">
          <div className="flex flex-col gap-0.5">
            <label className="text-xs text-muted">{t('welcome.editingVersion')}</label>
            <Select
              aria-label={t('welcome.editingVersion')}
              selectedKey={selected || null}
              onSelectionChange={(k) => setSelected(k ? String(k) : '')}
              className="w-48"
            >
              <Select.Trigger>
                <Select.Value>
                  {!selected
                    ? '— select —'
                    : selected + (activeVersions.includes(selected) ? ' (active)' : '')}
                </Select.Value>
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
                      {activeVersions.includes(p.version) ? ' (active)' : ''}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
          </div>
          {can('welcome:manage') && selected && (
            <Button size="sm" variant="ghost" onPress={() => deleteVersion(selected)}>
              <Icon name="trash-2" />
            </Button>
          )}
        </div>

        {can('welcome:manage') && (
          <div className="flex items-end gap-2">
            <div className="flex flex-col gap-0.5">
              <label className="text-xs text-muted">{t('welcome.newVersionLabel')}</label>
              <Input
                aria-label={t('welcome.newVersionLabel')}
                className="w-36"
                placeholder={t('welcome.newVersionPlaceholder')}
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') addVersion() }}
              />
            </div>
            <Button size="sm" variant="outline" onPress={addVersion}>
              <Icon name="plus" />
              {' '}
              {t('welcome.addVersion')}
            </Button>
          </div>
        )}
      </div>

      {/* Add-item row */}
      {can('welcome:manage') && selected && (
        <div className="flex items-center gap-2 pb-3 shrink-0">
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
          <NumberInput prefix="Qty" ariaLabel="Qty" min={1} value={addQty} onChange={setAddQty} className="w-48 shrink-0" />
          <NumberInput prefix="Quality" ariaLabel="Quality" min={0} value={addQuality} onChange={setAddQuality} className="w-48 shrink-0" />
          <Button size="sm" onPress={addItem} isDisabled={!addSelected} className="shrink-0">
            <Icon name="plus" />
            {' '}
            {t('welcome.addItem')}
          </Button>
        </div>
      )}

      {/* Item DataGrid */}
      {!selected
        ? <p className="text-xs text-muted shrink-0">{t('welcome.noPackageSelected')}</p>
        : keyedItems.length === 0
          ? <p className="text-xs text-muted shrink-0">{t('welcome.noItemsYet')}</p>
          : (
              <DataGrid
                aria-label={t('welcome.sections.packages')}
                columns={columns}
                data={keyedItems}
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

      {/* Per-package totals */}
      {selected && keyedItems.length > 0 && (
        <div className="pt-3 shrink-0 flex flex-wrap items-center gap-4 text-xs text-muted">
          <span className="flex items-center gap-1.5">
            <Icon name="weight" />
            {t('welcome.kit.totalVolume')}
            <span className="text-foreground tabular-nums">{Math.round(totals.volume * 100) / 100}</span>
          </span>
          <span className="flex items-center gap-1.5">
            <Icon name="grid-3x3" />
            {t('welcome.kit.totalSlots')}
            <span className="text-foreground tabular-nums">{totals.slots}</span>
          </span>
        </div>
      )}

      {/* Save button + diff status */}
      {can('welcome:manage') && (
        <div className="pt-3 shrink-0 flex items-center gap-3">
          <Button size="sm" variant="secondary" onPress={save} isDisabled={saving}>
            {saving
              ? <Spinner size="sm" color="current" />
              : (
                  <>
                    <Icon name="save" />
                    {' '}
                    {t('welcome.saveConfig')}
                  </>
                )}
          </Button>
          <DiffStatus diff={configDiff} />
        </div>
      )}

      <ActionBar aria-label={t('welcome.sections.packages')} isOpen={can('welcome:manage') && selectionCount > 0}>
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
    </div>
  )
}
