import { useState, useEffect, useMemo } from 'react'
import {
  Button, Header, Input, InputGroup, ListBox, Modal,
  SearchField, Select, Separator, Spinner, TextField, toast,
} from '@heroui/react'
import { api } from '../../../api/client'
import type { Player } from '../../../api/client'
import { Icon } from '../../../dune-ui'
import type { PacksData } from '../types'

interface Props {
  player: Player
  open: boolean
  onClose: () => void
}

export function GiveItemsModal({ player, open, onClose }: Props) {
  const [templates, setTemplates] = useState<{id: string; name: string}[]>([])
  const [packsData, setPacksData] = useState<PacksData>({ packs: {} })
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
    Promise.resolve()
      .then(() => { setLoading(true); setQuery(''); setSelected(''); setQty(1); setQuality(0); setStaged([]); setResult(null) })
      .then(() => Promise.all([
        api.players.templates(),
        fetch('/packs.json').then(r => r.json() as Promise<PacksData>).catch(() => ({ packs: {} } as PacksData)),
      ]))
      .then(([tmpls, packs]) => { setTemplates(tmpls); setPacksData(packs) })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [open])

  const filtered = useMemo(() => {
    if (!query) return []
    const q = query.toLowerCase()
    return templates.filter(t => t.id.toLowerCase().includes(q) || t.name.toLowerCase().includes(q)).slice(0, 100)
  }, [templates, query])

  const groupedPacks = useMemo(() => {
    const groups: Record<string, { id: string; name: string; tier: number }[]> = {}
    for (const [id, pack] of Object.entries(packsData.packs)) {
      if (!groups[pack.category]) groups[pack.category] = []
      groups[pack.category].push({ id, name: pack.name, tier: pack.tier })
    }
    for (const cat of Object.keys(groups)) {
      groups[cat].sort((a, b) => a.tier - b.tier)
    }
    return groups
  }, [packsData])

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
      if (res.skipped.length === 0) {
        toast.success(`Gave ${res.given.length} item${res.given.length !== 1 ? 's' : ''} to ${player.name}`)
        onClose()
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
        <Modal.Container size="cover" scroll="outside">
          <Modal.Dialog>
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading className="text-accent">{player.name} — Give Items</Modal.Heading>
            </Modal.Header>
            <Modal.Body className="flex flex-col gap-3">
              {loading ? (
                <div className="flex justify-center py-6"><Spinner size="lg" /></div>
              ) : (
                <>
                  {/* Load Pack — HeroUI Select with grouped sections */}
                  <Select
                    aria-label="Load pack"
                    placeholder="Load Pack…"
                    selectedKey={null}
                    onSelectionChange={k => {
                      const id = k ? String(k) : ''
                      const pack = packsData.packs[id]
                      if (pack) setStaged(prev => [...prev, ...pack.items])
                    }}
                    className="w-full"
                  >
                    <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
                    <Select.Popover>
                      <ListBox>
                        {Object.entries(groupedPacks).sort(([a], [b]) => a.localeCompare(b)).map(([cat, packs], i, arr) => (
                          <ListBox.Section key={cat}>
                            <Header>{cat.replace(/-/g, ' ')}</Header>
                            {packs.map(p => (
                              <ListBox.Item key={p.id} id={p.id} textValue={p.name}>
                                {p.name}<ListBox.ItemIndicator />
                              </ListBox.Item>
                            ))}
                            {i < arr.length - 1 && <Separator />}
                          </ListBox.Section>
                        ))}
                      </ListBox>
                    </Select.Popover>
                  </Select>

                  {/* Template (flex-1) + Qty / Quality + Add — single row, all
                      using InputGroup.Prefix so heights match and items-center
                      aligns everything visually. */}
                  <div className="flex items-center gap-3">
                    <TextField className="flex-1 min-w-0" aria-label="Template">
                      <div className="relative w-full">
                        <SearchField
                          className="w-full"
                          value={query}
                          onChange={v => { setQuery(v); setSelected('') }}
                        >
                          <SearchField.Group>
                            <SearchField.SearchIcon />
                            <SearchField.Input placeholder="Search templates..." />
                            <SearchField.ClearButton />
                          </SearchField.Group>
                        </SearchField>
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
                        <div className="text-success">✓ Gave: {result.given.join(', ')}</div>
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
              <Button variant="tertiary" size="sm" slot="close">Cancel</Button>
              <Button size="sm" onPress={handleSubmit} isDisabled={submitting || staged.length === 0}>
                {submitting ? <Spinner size="sm" color="current" /> : <Icon name="gift" />}
                Give {staged.length} Item{staged.length !== 1 ? 's' : ''}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
