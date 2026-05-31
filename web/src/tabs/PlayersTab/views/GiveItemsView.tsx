import { useState, useEffect, useMemo } from 'react'
import {
  Button, Header, ListBox, SearchField, Select, Separator, Spinner, TextField, toast,
} from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { Player } from '../../../api/client'
import { Icon, NumberInput } from '../../../dune-ui'
import type { PacksData } from '../types'

interface Props {
  player: Player
}

type SkippedItem = { template: string, reason: string }
type GiveResult = { given: string[], skipped: SkippedItem[] } | null
type StagedItem = { template: string, qty: number, quality: number }

export function GiveItemsView({ player }: Props) {
  const { t } = useTranslation()
  const [templates, setTemplates] = useState<{ id: string, name: string }[]>([])
  const [packsData, setPacksData] = useState<PacksData>({ packs: {} })
  const [loading, setLoading] = useState(false)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState('')
  const [qty, setQty] = useState(1)
  const [quality, setQuality] = useState(0)
  const [staged, setStaged] = useState<StagedItem[]>([])
  const [submitting, setSubmitting] = useState(false)
  const [result, setResult] = useState<GiveResult>(null)

  useEffect(() => {
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
      .then(() => Promise.all([
        api.players.templates(),
        fetch('/packs.json').then((r) => r.json() as Promise<PacksData>).catch(() => ({ packs: {} } as PacksData)),
      ]))
      .then(([tmpls, packs]) => {
        setTemplates(tmpls)
        setPacksData(packs)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [player.id])

  const filtered = useMemo(() => {
    if (!query) return []
    const q = query.toLowerCase()
    return templates.filter((t) => t.id.toLowerCase().includes(q) || t.name.toLowerCase().includes(q)).slice(0, 100)
  }, [templates, query])

  const groupedPacks = useMemo(() => {
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
      const res = await api.players.giveItems(player.id, staged)
      setResult(res)
      setStaged([])
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

  if (loading) {
    return <div className="flex justify-center py-12"><Spinner size="lg" /></div>
  }

  return (
    <div className="flex flex-col h-full min-h-0">
      <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden flex flex-col gap-3 pb-2 pr-2">
        <Select
          aria-label={t('players.give.loadPack')}
          placeholder={t('players.give.loadPack')}
          selectedKey={null}
          onSelectionChange={(k) => {
            const id = k ? String(k) : ''
            const pack = packsData.packs[id]
            if (pack) setStaged((prev) => [...prev, ...pack.items])
          }}
          className="w-full"
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

        <div className="flex items-end gap-3">
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

        {staged.length > 0 && (
          <>
            <div className="flex flex-col gap-1">
              {staged.map((item, idx) => (
                <div
                  key={idx}
                  className="flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius)] text-xs bg-surface border border-border"
                >
                  <span className="flex-1 min-w-0 truncate font-mono text-foreground">{item.template}</span>
                  <NumberInput
                    ariaLabel={`${t('players.give.qty')} for ${item.template}`}
                    prefix={t('players.give.qty')}
                    min={1}
                    value={item.qty}
                    onChange={(v) => updateStaged(idx, 'qty', v)}
                    className="w-56"
                  />
                  <NumberInput
                    ariaLabel={`${t('players.give.quality')} for ${item.template}`}
                    prefix={t('players.give.quality')}
                    min={0}
                    value={item.quality}
                    onChange={(v) => updateStaged(idx, 'quality', v)}
                    className="w-56"
                  />
                  <Button
                    size="sm"
                    variant="danger-soft"
                    onPress={() => removeFromStaged(idx)}
                    aria-label={t('common.remove')}
                  >
                    <Icon name="x" />
                  </Button>
                </div>
              ))}
            </div>
          </>
        )}

        {result && (
          <div className="text-xs rounded-[var(--radius)] px-3 py-2 bg-surface border border-border">
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

      </div>

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
  )
}
