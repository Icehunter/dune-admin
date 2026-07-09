import * as React from 'react'
import {
  Button, Chip, Header, ListBox, SearchField, Select, Separator, Spinner, TextField, toast,
} from '@heroui/react'
import type { Selection } from '@heroui/react'
import type { DataGridColumn } from '@heroui-pro/react'
import { DataGrid } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { useAtomValue } from 'jotai'
import { api } from '../../../api/client'
import type { GivePack } from '../../../api/client'
import { ActionBar, Icon, LoadingState, NumberInput } from '../../../dune-ui'
import { ItemDetailDrawer } from '../../../components/ItemDetailDrawer'
import { ItemOptionRow } from '../../../components/ItemOptionRow'
import { StagedItemCell } from '../../../components/StagedItemCell'
import { itemDataSyncAtom } from '../../../data/store'
import { retainSkippedStaged, filterTemplates } from './giveItemsHelpers'
import { ManagePacksModal } from '../modals/ManagePacksModal'
import type { GiveItemsViewProps } from './interfaces'
import type { GiveResult, StagedItem } from './types'

export const GiveItemsView: React.FC<GiveItemsViewProps> = ({ player }) => {
  const { t } = useTranslation()
  const itemData = useAtomValue(itemDataSyncAtom)
  const [templates, setTemplates] = React.useState<{ id: string, name: string }[]>([])
  const [packs, setPacks] = React.useState<GivePack[]>([])
  const [loading, setLoading] = React.useState(false)
  const [query, setQuery] = React.useState('')
  const [selected, setSelected] = React.useState('')
  const [qty, setQty] = React.useState(1)
  const [quality, setQuality] = React.useState(0)
  const [staged, setStaged] = React.useState<StagedItem[]>([])
  const [submitting, setSubmitting] = React.useState(false)
  const [result, setResult] = React.useState<GiveResult>(null)
  const [manageOpen, setManageOpen] = React.useState(false)
  const [selectedKeys, setSelectedKeys] = React.useState<Selection>(new Set())
  const [detailId, setDetailId] = React.useState<string | null>(null)

  const keyCounter = React.useRef(0)
  const nextKey = () => String(keyCounter.current++)

  const loadData = (): void => {
    setLoading(true)
    setQuery('')
    setSelected('')
    setQty(1)
    setQuality(0)
    setStaged([])
    setResult(null)
    Promise.all([
      api.players.templates(),
      api.givePacks.config(),
    ])
      .then(([tmpls, cfg]) => {
        setTemplates(tmpls)
        setPacks(cfg.packs ?? [])
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  React.useEffect(() => {
    void Promise.resolve().then(() => loadData())
  }, [player.id])

  const nameMap = new Map(templates.map((tpl) => [tpl.id, tpl.name]))

  const filtered = filterTemplates(templates, query)

  const groupedPacks: Record<string, { id: string, name: string, tier: number }[]> = {}
  for (const pack of packs) {
    if (!groupedPacks[pack.category]) groupedPacks[pack.category] = []
    groupedPacks[pack.category].push({ id: pack.id, name: pack.name, tier: pack.tier })
  }
  for (const cat of Object.keys(groupedPacks)) {
    groupedPacks[cat].sort((a, b) => a.tier - b.tier)
  }

  const pick = (tpl: { id: string, name: string }) => {
    setSelected(tpl.id)
    setQuery(tpl.name ? `${tpl.id}  —  ${tpl.name}` : tpl.id)
  }

  const addToStaged = () => {
    if (!selected) {
      toast.warning(t('players.give.selectTemplate'))
      return
    }
    setStaged((prev) => [...prev, { template: selected, qty, quality, _key: nextKey() }])
    setQuery('')
    setSelected('')
    setQty(1)
    setQuality(0)
  }

  const removeFromStaged = (key: string) => {
    setStaged((prev) => prev.filter((it) => it._key !== key))
    setSelectedKeys((prev) => {
      if (prev === 'all') return new Set(staged.filter((it) => it._key !== key).map((it) => it._key))
      const next = new Set(prev as Set<string>)
      next.delete(key)
      return next
    })
  }

  const updateStaged = (key: string, field: 'qty' | 'quality', value: number) => {
    setStaged((prev) => prev.map((item) => item._key === key ? { ...item, [field]: value } : item))
  }

  const selectionCount = selectedKeys === 'all' ? staged.length : (selectedKeys as Set<string>).size

  const handleBulkDelete = () => {
    if (selectedKeys === 'all') {
      setStaged([])
    }
    else {
      const keys = selectedKeys as Set<string>
      setStaged((prev) => prev.filter((it) => !keys.has(it._key)))
    }
    setSelectedKeys(new Set())
  }

  const handleSubmit = async () => {
    if (staged.length === 0) return
    setSubmitting(true)
    try {
      const items = staged.map(({ template, qty: q, quality: ql }) => ({ template, qty: q, quality: ql }))
      const res = await api.players.giveItems(player.id, items)
      setResult(res)
      setStaged((prev) => retainSkippedStaged(prev, res.given))
      setSelectedKeys(new Set())
      if (res.skipped.length === 0) {
        toast.success(t('players.give.gaveItems', { count: res.given.length, player: player.name }))
        setQuery('')
        setSelected('')
        setQty(1)
        setQuality(0)
        setResult(null)
      }
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
    finally {
      setSubmitting(false)
    }
  }

  const columns: DataGridColumn<StagedItem>[] = [
    {
      id: 'template',
      isRowHeader: true,
      header: t('players.inventory.columns.template'),
      minWidth: 200,
      allowsResizing: true,
      cell: (item) => (
        <StagedItemCell templateId={item.template} name={nameMap.get(item.template) || ''} entry={itemData.items[item.template] ?? null} />
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
          onChange={(v) => updateStaged(item._key, 'qty', v)}
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
          onChange={(v) => updateStaged(item._key, 'quality', v)}
          className="w-full"
        />
      ),
    },
    {
      id: 'actions',
      header: '',
      width: 88,
      cell: (item) => (
        <div className="flex items-center gap-1">
          <Button
            size="sm"
            variant="ghost"
            isIconOnly
            onPress={() => setDetailId(item.template)}
            aria-label={t('common.info')}
          >
            <Icon name="info" />
          </Button>
          <Button
            size="sm"
            variant="danger-soft"
            isIconOnly
            onPress={() => removeFromStaged(item._key)}
            aria-label={t('common.remove')}
          >
            <Icon name="trash" />
          </Button>
        </div>
      ),
    },
  ]

  if (loading) {
    return <LoadingState />
  }

  return (
    <React.Fragment>
      <div className="flex flex-col h-full min-h-0 gap-3">
        <div className="flex items-center gap-2 shrink-0">
          <Select
            aria-label={t('players.give.loadPack')}
            placeholder={t('players.give.loadPack')}
            selectedKey={null}
            onSelectionChange={(k) => {
              const id = k ? String(k) : ''
              const pack = packs.find((p) => p.id === id)
              if (pack) setStaged((prev) => [...prev, ...pack.items.map((item) => ({ ...item, _key: nextKey() }))])
            }}
            className="flex-1"
          >
            <Select.Trigger>
              <Select.Value />
              <Select.Indicator />
            </Select.Trigger>
            <Select.Popover>
              <ListBox>
                {Object.entries(groupedPacks)
                  .sort(([a], [b]) => a.localeCompare(b))
                  .map(([cat, catPacks], i, arr) => (
                    <ListBox.Section key={cat}>
                      <Header>{cat.replace(/-/g, ' ')}</Header>
                      {catPacks.map((p) => (
                        <ListBox.Item key={p.id} id={p.id} textValue={p.name}>
                          {p.name}
                          <ListBox.ItemIndicator />
                        </ListBox.Item>
                      ))}
                      {i < arr.length - 1 && <Separator />}
                    </ListBox.Section>
                  ))}
              </ListBox>
            </Select.Popover>
          </Select>
          <Button
            size="sm"
            variant="ghost"
            onPress={() => setManageOpen(true)}
            aria-label={t('players.give.managePacks')}
          >
            <Icon name="settings-2" />
            {' '}
            {t('players.give.managePacks')}
          </Button>
        </div>

        <div className="flex items-end gap-3 shrink-0">
          <TextField className="flex-1 min-w-0" aria-label={t('players.inventory.columns.template')}>
            <div className="relative w-full">
              <SearchField
                className="w-full"
                aria-label={t('players.inventory.columns.template')}
                value={query}
                onChange={(v) => {
                  setQuery(v)
                  setSelected('')
                }}
              >
                <SearchField.Group>
                  <SearchField.SearchIcon />
                  <SearchField.Input placeholder={t('players.give.searchTemplates')} />
                  <SearchField.ClearButton />
                </SearchField.Group>
              </SearchField>
              {filtered.length > 0 && (
                <div className="absolute z-50 w-full mt-1 rounded-[var(--radius)] border border-border bg-surface overflow-y-auto max-h-52">
                  {filtered.map((tpl) => (
                    <ItemOptionRow
                      key={tpl.id}
                      id={tpl.id}
                      name={tpl.name}
                      entry={itemData.items[tpl.id] ?? null}
                      onPick={() => pick(tpl)}
                      onDetail={() => setDetailId(tpl.id)}
                    />
                  ))}
                </div>
              )}
            </div>
          </TextField>
          <NumberInput
            prefix={t('players.give.qty')}
            ariaLabel={t('players.give.qty')}
            min={1}
            value={qty}
            onChange={setQty}
            className="w-44 shrink-0"
          />
          <NumberInput
            prefix={t('players.give.quality')}
            ariaLabel={t('players.give.quality')}
            min={0}
            value={quality}
            onChange={setQuality}
            className="w-44 shrink-0"
          />
          <Button size="sm" onPress={addToStaged} isDisabled={!selected} className="shrink-0">
            <Icon name="plus" />
            {' '}
            {t('players.give.add')}
          </Button>
        </div>

        {/* Quality>0 is a live-state limitation of the game: the item
            is written to the DB but only materializes after the player
            relogs, and may land outside their free inventory slots (#207). */}
        {(quality > 0 || staged.some((s) => s.quality > 0)) && (
          <div className="shrink-0 flex items-start gap-2 rounded-[var(--radius)] px-3 py-2 bg-surface border border-warning/40 text-xs text-muted">
            <Icon name="triangle-alert" className="text-warning shrink-0 mt-0.5" />
            <span>{t('players.give.qualityWarning')}</span>
          </div>
        )}

        {staged.length > 0 && (
          <DataGrid
            aria-label={t('players.give.loadPack')}
            columns={columns}
            data={staged}
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

        {result && (
          <div className="text-xs shrink-0 rounded-[var(--radius)] px-3 py-2 bg-surface border border-border">
            {result.given.length > 0 && (
              <div className="text-success">
                {t('players.give.gave')}
                {' '}
                {result.given.join(', ')}
              </div>
            )}
            {result.skipped.map((s, i) => (
              <div key={i} className="text-danger">
                {t('players.give.skipped', { template: s.template, reason: s.reason })}
              </div>
            ))}
          </div>
        )}

        {staged.length > 0 && (
          <div className="shrink-0 pt-3 border-t border-border flex justify-end">
            <Button size="sm" onPress={handleSubmit} isDisabled={submitting || staged.length === 0}>
              {submitting ? <Spinner size="sm" color="current" /> : <Icon name="gift" />}
              {' '}
              {t('players.give.giveCount', { count: staged.length })}
            </Button>
          </div>
        )}
      </div>

      <ActionBar aria-label={t('players.give.loadPack')} isOpen={selectionCount > 0}>
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

      <ManagePacksModal
        isOpen={manageOpen}
        onClose={() => setManageOpen(false)}
        onSaved={(savedPacks) => {
          setPacks(savedPacks)
          setManageOpen(false)
        }}
        templates={templates}
      />

      <ItemDetailDrawer
        templateId={detailId}
        name={detailId !== null ? nameMap.get(detailId) : undefined}
        onClose={() => setDetailId(null)}
      />
    </React.Fragment>
  )
}
