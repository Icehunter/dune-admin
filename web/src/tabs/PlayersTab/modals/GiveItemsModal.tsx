import * as React from 'react'
import {
  Button, Chip, Header, ListBox, Modal,
  SearchField, Select, Separator, Spinner, TextField, toast,
} from '@heroui/react'
import type { Selection } from '@heroui/react'
import type { DataGridColumn } from '@heroui-pro/react'
import { DataGrid } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { useAtom } from 'jotai'
import { api } from '../../../api/client'
import { ActionBar, Icon, LoadingState, NumberInput } from '../../../dune-ui'
import { packsSyncAtom } from '../../../data/store'
import type { GiveItemsModalProps, GiveResult, StagedItem } from './types'

export const GiveItemsModal: React.FC<GiveItemsModalProps> = ({ player, open, onClose }) => {
  const { t } = useTranslation()
  const [templates, setTemplates] = React.useState<{ id: string, name: string }[]>([])
  const [loading, setLoading] = React.useState(false)
  const [query, setQuery] = React.useState('')
  const [selected, setSelected] = React.useState('')
  const [qty, setQty] = React.useState(1)
  const [quality, setQuality] = React.useState(0)
  const [staged, setStaged] = React.useState<StagedItem[]>([])
  const [submitting, setSubmitting] = React.useState(false)
  const [result, setResult] = React.useState<GiveResult>(null)
  const [selectedKeys, setSelectedKeys] = React.useState<Selection>(new Set())
  const [packsData] = useAtom(packsSyncAtom)

  const keyCounter = React.useRef(0)
  const nextKey = () => String(keyCounter.current++)

  React.useEffect(() => {
    if (!open) return
    void Promise.resolve().then(() => {
      setLoading(true)
      setQuery('')
      setSelected('')
      setQty(1)
      setQuality(0)
      setStaged([])
      setResult(null)
      setSelectedKeys(new Set())
    })
      .then(() => api.players.templates())
      .then((tmpls) => { setTemplates(tmpls) })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [open])

  const nameMap = React.useMemo(() => new Map(templates.map((tpl) => [tpl.id, tpl.name])), [templates])

  const filtered = React.useMemo(() => {
    if (!query) return []
    const q = query.toLowerCase()
    return templates
      .filter((tpl) => tpl.id.toLowerCase().includes(q) || tpl.name.toLowerCase().includes(q))
      .slice(0, 100)
  }, [templates, query])

  const groupedPacks = React.useMemo(() => {
    const groups: Record<string, { id: string, name: string, tier: number }[]> = {}
    for (const [id, pack] of Object.entries(packsData.packs)) {
      if (!groups[pack.category]) groups[pack.category] = []
      groups[pack.category].push({ id, name: pack.name, tier: pack.tier })
    }
    for (const cat of Object.keys(groups)) {
      groups[cat].sort((a, b) => a.tier - b.tier)
    }
    return groups
  }, [packsData])

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
      const items = staged.map(({ template, qty, quality }) => ({ template, qty, quality }))
      const res = await api.players.giveItems(player.id, items)
      setResult(res)
      setStaged([])
      setSelectedKeys(new Set())
      if (res.skipped.length === 0) {
        toast.success(t('players.give.gaveItems', { count: res.given.length, player: player.name }))
        onClose()
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
          ariaLabel={`${t('players.give.qty')} for ${item.template}`}
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
          ariaLabel={`${t('players.give.quality')} for ${item.template}`}
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
      width: 52,
      cell: (item) => (
        <Button
          size="sm"
          variant="danger-soft"
          isIconOnly
          onPress={() => removeFromStaged(item._key)}
          aria-label={t('common.remove')}
        >
          <Icon name="trash" />
        </Button>
      ),
    },
  ]

  return (
    <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={open} onOpenChange={(v) => !v && onClose()}>
      <Modal.Container size="cover" scroll="outside">
        <Modal.Dialog className="p-10">
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading className="text-accent">
              {t('players.give.modalTitle', { name: player.name })}
            </Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-3 h-[80vh] min-h-0">
            {loading
              ? (
                  <LoadingState size="sm" />
                )
              : (
                  <>
                    {/* Load Pack */}
                    <Select
                      aria-label={t('players.give.loadPack')}
                      placeholder={t('players.give.loadPack')}
                      selectedKey={null}
                      onSelectionChange={(k) => {
                        const id = k ? String(k) : ''
                        const pack = packsData.packs[id]
                        if (pack) {
                          setStaged((prev) => [
                            ...prev,
                            ...pack.items.map((item) => ({ ...item, _key: nextKey() })),
                          ])
                        }
                      }}
                      className="w-full shrink-0"
                    >
                      <Select.Trigger>
                        <Select.Value />
                        <Select.Indicator />
                      </Select.Trigger>
                      <Select.Popover>
                        <ListBox>
                          {Object.entries(groupedPacks)
                            .sort(([a], [b]) => a.localeCompare(b))
                            .map(([cat, packs], i, arr) => (
                              <ListBox.Section key={cat}>
                                <Header>{cat.replace(/-/g, ' ')}</Header>
                                {packs.map((p) => (
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

                    {/* Template + Qty + Quality + Add */}
                    <div className="flex items-end gap-3 shrink-0">
                      <TextField className="flex-1 min-w-0" aria-label={t('players.inventory.columns.template')}>
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
                              <SearchField.Input placeholder={t('players.give.searchTemplates')} />
                              <SearchField.ClearButton />
                            </SearchField.Group>
                          </SearchField>
                          {filtered.length > 0 && (
                            <div className="absolute z-50 w-full mt-1 rounded-[var(--radius)] border border-border bg-surface overflow-y-auto max-h-52">
                              {filtered.map((tpl) => (
                                <div
                                  key={tpl.id}
                                  className="px-3 py-1.5 text-xs cursor-pointer hover:bg-surface-hover"
                                  onClick={() => pick(tpl)}
                                >
                                  <span className="font-mono">{tpl.id}</span>
                                  {tpl.name
                                    ? (
                                        <span className="text-muted">
                                          {' '}
                                          —
                                          {tpl.name}
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
                        prefix={t('players.give.qty')}
                        ariaLabel={t('players.give.qty')}
                        min={1}
                        value={qty}
                        onChange={setQty}
                        className="w-56 shrink-0"
                      />
                      <NumberInput
                        prefix={t('players.give.quality')}
                        ariaLabel={t('players.give.quality')}
                        min={0}
                        value={quality}
                        onChange={setQuality}
                        className="w-56 shrink-0"
                      />
                      <Button size="sm" onPress={addToStaged} isDisabled={!selected} className="shrink-0">
                        <Icon name="plus" />
                        {' '}
                        {t('players.give.add')}
                      </Button>
                    </div>

                    {/* Staged items DataGrid */}
                    {staged.length > 0 && (
                      <DataGrid
                        aria-label={t('players.give.modalTitle', { name: player.name })}
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
                  </>
                )}
          </Modal.Body>
          <Modal.Footer>
            <Button variant="tertiary" size="sm" slot="close">{t('common.cancel')}</Button>
            <Button size="sm" onPress={handleSubmit} isDisabled={submitting || staged.length === 0}>
              {submitting ? <Spinner size="sm" color="current" /> : <Icon name="gift" />}
              {' '}
              {t('players.give.giveCount', { count: staged.length })}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>

      <ActionBar aria-label={t('players.give.modalTitle', { name: player.name })} isOpen={selectionCount > 0}>
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
