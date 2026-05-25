import { useState, useEffect, useMemo, type ReactNode } from 'react'
import {
  Button, Chip, Input, ListBox, ListLayout, Modal, Select,
  Spinner, Virtualizer, toast,
} from '@heroui/react'
import { DataTable, Panel } from '../../../dune-ui'
import allGameplayTags from '../../../data/gameplayTags.json'
import { api } from '../../../api/client'
import type {
  Player, JourneyNode, SpecTrack, KeystoneRow,
  TeleportLocation, GameEvent, DungeonRecord,
} from '../../../api/client'
import {
  ACTION_SECTIONS, XP_TRACKS, FACTIONS,
  type ActionSection,
} from '../types'

interface Props {
  player: Player
  open: boolean
  onClose: () => void
}

const TRAINERS = ['BeneGesserit', 'Mentat', 'Planetologist', 'Swordmaster', 'Trooper'] as const
type TrainerKey = typeof TRAINERS[number]

const MAIN_QUESTS = [
  { id: 'DA_MQ_ANewBeginning',         label: '1. A New Beginning',           nodes: 132 },
  { id: 'DA_MQ_AssassinsHandbook',     label: '2. Assassin’s Handbook',  nodes: 91  },
  { id: 'DA_MQ_FindTheFremen',         label: '3. Find the Fremen',           nodes: 46  },
  { id: 'DA_MQ_TheGreatConvention',    label: '4. The Great Convention',      nodes: 90  },
  { id: 'DA_MQ_TheGreatConventionPt2', label: '5. Great Convention Pt 2',     nodes: 109 },
  { id: 'DA_MQ_TheBloodline',          label: '6. The Bloodline (standalone)', nodes: 0  },
] as const

function useDebounce<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(t)
  }, [value, delay])
  return debounced
}

export function PlayerActionsModal({ player, open, onClose }: Props) {
  const [section, setSection] = useState<ActionSection>('resources')
  const [busy, setBusy] = useState(false)

  // Resources
  const [currency, setCurrency] = useState(100)
  const [scrip, setScrip] = useState(100)
  const [intel, setIntel] = useState(100)

  // XP
  const [charXP, setCharXP] = useState(1000)
  const [charXPCurrent, setCharXPCurrent] = useState<{xp: number; level: number} | null>(null)

  // Faction
  const [factionId, setFactionId] = useState(player.faction_id || 0)
  const [repDelta, setRepDelta] = useState(100)

  // Specs
  const [playerSpecs, setPlayerSpecs] = useState<SpecTrack[]>([])
  const [playerKeystones, setPlayerKeystones] = useState<KeystoneRow[]>([])
  const [specsLoaded, setSpecsLoaded] = useState(false)
  const [specsLoading, setSpecsLoading] = useState(false)

  // Journey
  const [nodes, setNodes] = useState<JourneyNode[]>([])
  const [nodesLoaded, setNodesLoaded] = useState(false)
  const [nodesLoading, setNodesLoading] = useState(false)
  const [nodeSearch, setNodeSearch] = useState('')
  const debouncedNodeSearch = useDebounce(nodeSearch)
  const [unlockFaction, setUnlockFaction] = useState('atreides')
  const [unlockPreset, setUnlockPreset] = useState('ch3_start')

  // Trainer / MQ selectors
  const [selectedTrainer, setSelectedTrainer] = useState<TrainerKey>('BeneGesserit')
  const [selectedMQ, setSelectedMQ] = useState<string>('DA_MQ_ANewBeginning')

  // Contracts
  const [contractCatalog, setContractCatalog] = useState<{id: string; alias: string; tag_count: number}[]>([])
  const [contractCatalogLoaded, setContractCatalogLoaded] = useState(false)
  const [contractCatalogError, setContractCatalogError] = useState('')
  const [contractSearch, setContractSearch] = useState('')
  const [selectedContracts, setSelectedContracts] = useState<string[]>([])

  // Tags
  const [tags, setTags] = useState<string[]>([])
  const [tagsLoaded, setTagsLoaded] = useState(false)
  const [tagsLoading, setTagsLoading] = useState(false)
  const [pendingTags, setPendingTags] = useState<string[]>([])

  // Admin / Teleport
  const [partitions, setPartitions] = useState<TeleportLocation[]>([])
  const [selectedPartition, setSelectedPartition] = useState('')

  // History
  const [events, setEvents] = useState<GameEvent[]>([])
  const [dungeons, setDungeons] = useState<DungeonRecord[]>([])
  const [historyLoaded, setHistoryLoaded] = useState(false)
  const [historyLoading, setHistoryLoading] = useState(false)

  useEffect(() => {
    if (!open) {
      setSection('resources')
      setNodesLoaded(false); setNodes([])
      setPlayerSpecs([]); setPlayerKeystones([]); setSpecsLoaded(false)
      setHistoryLoaded(false); setEvents([]); setDungeons([])
      setCharXPCurrent(null)
      setTagsLoaded(false); setTags([]); setPendingTags([])
    } else {
      setFactionId(player.faction_id > 0 ? player.faction_id : 1)
      api.players.partitions().then(setPartitions).catch(() => {})
      api.players.charXPCurrent(player.id).then(setCharXPCurrent).catch(() => {})
    }
  }, [open, player.faction_id, player.id])

  useEffect(() => {
    if (section === 'journey' && !nodesLoaded && open) {
      setNodesLoading(true)
      api.players.journey(player.account_id)
        .then(n => { setNodes(n); setNodesLoaded(true) })
        .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
        .finally(() => setNodesLoading(false))
    }
    if (section === 'progression' && !contractCatalogLoaded && open) {
      api.contracts.list()
        .then(c => { setContractCatalog(c); setContractCatalogLoaded(true); setContractCatalogError('') })
        .catch((e: unknown) => { setContractCatalogError(e instanceof Error ? e.message : String(e)); setContractCatalogLoaded(true) })
    }
  }, [section, nodesLoaded, contractCatalogLoaded, open, player.account_id])

  useEffect(() => {
    if (section === 'specs' && !specsLoaded && open) {
      setSpecsLoading(true)
      Promise.all([
        api.players.specs_for(player.controller_id),
        api.players.keystones(player.controller_id),
      ]).then(([s, k]) => {
        setPlayerSpecs(s); setPlayerKeystones(k); setSpecsLoaded(true)
      })
        .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
        .finally(() => setSpecsLoading(false))
    }
  }, [section, specsLoaded, open, player.controller_id])

  useEffect(() => {
    if (section === 'history' && !historyLoaded && open) {
      setHistoryLoading(true)
      Promise.all([
        api.players.events(player.id),
        api.players.dungeons(player.id),
      ]).then(([evts, dngns]) => {
        setEvents(evts); setDungeons(dngns); setHistoryLoaded(true)
      })
        .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
        .finally(() => setHistoryLoading(false))
    }
  }, [section, historyLoaded, open, player.id])

  useEffect(() => {
    if (section !== 'tags' || tagsLoaded || !open) return
    setTagsLoading(true)
    api.players.tags(player.account_id)
      .then(t => { setTags(t); setTagsLoaded(true) })
      .catch(() => {})
      .finally(() => setTagsLoading(false))
  }, [section, tagsLoaded, open, player.account_id])

  const run = async (fn: () => Promise<unknown>, label: string) => {
    setBusy(true)
    try {
      await fn()
      toast.success(label)
    } catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const filteredNodes = useMemo(() => {
    if (!debouncedNodeSearch) return nodes
    const q = debouncedNodeSearch.toLowerCase()
    return nodes.filter(n => n.node_id.toLowerCase().includes(q))
  }, [nodes, debouncedNodeSearch])

  const numInput = (val: number, set: (v: number) => void, min = 1, max = 9999999) => (
    <Input
      type="number" min={min} max={max} value={val}
      onChange={e => set(Math.max(min, Math.min(max, parseInt(e.target.value) || min)))}
      aria-label="number"
      className="w-28"
    />
  )

  const actionRow = (label: string, inputs: ReactNode, btnLabel: string, onAction: () => void, danger = false) => (
    <div className="flex items-end gap-3 py-3 border-b border-border/40">
      <div className="w-36 shrink-0 text-sm text-muted">{label}</div>
      <div className="flex items-end gap-2 flex-1 flex-wrap">{inputs}</div>
      <Button size="sm" variant={danger ? 'danger-soft' : 'ghost'} onPress={onAction} isDisabled={busy}>{btnLabel}</Button>
    </div>
  )

  const onlineWarning = (
    <div className="text-xs px-3 py-2 rounded mb-3 bg-warning/10 border border-warning text-warning">
      ⚠ Player is online — XP changes may not take effect until they reconnect
    </div>
  )

  const formatDuration = (ms: number) => {
    const secs = Math.floor(ms / 1000)
    const m = Math.floor(secs / 60)
    const s = secs % 60
    return `${m}:${String(s).padStart(2, '0')}`
  }

  return (
    <Modal>
      <Modal.Backdrop isOpen={open} onOpenChange={v => !v && onClose()}>
        <Modal.Container size="cover">
          <Modal.Dialog className="h-[92vh] flex flex-col dialog-surface-alt">
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading className="text-accent">
                {player.name} — Actions
                <span className="ml-3 text-sm font-mono font-normal text-muted">
                  actor:{player.id} · ctrl:{player.controller_id} · acct:{player.account_id}
                </span>
              </Modal.Heading>
            </Modal.Header>
            <Modal.Body className="flex gap-0 overflow-hidden p-0 flex-1">

              {/* Section nav */}
              <div className="shrink-0 flex flex-col gap-1 p-2 m-3 mr-0 border border-border/60 rounded-[var(--radius)] bg-background dune-lift min-w-[140px]">
                {ACTION_SECTIONS.map(s => {
                  const isActive = section === s.key
                  return (
                    <button
                      key={s.key}
                      onClick={() => setSection(s.key)}
                      className={
                        'text-left px-3 py-2 rounded-[var(--radius)] text-sm transition-colors ' +
                        (isActive
                          ? 'bg-accent text-accent-foreground font-semibold'
                          : 'text-foreground hover:bg-surface-hover')
                      }
                    >
                      {s.label}
                    </button>
                  )
                })}
              </div>

              {/* Section content */}
              <div className="flex-1 overflow-hidden flex flex-col p-4">

                {section === 'resources' && (
                  <div className="overflow-y-auto flex-1 flex flex-col">

                    <SectionHeader>Currency &amp; Resources</SectionHeader>
                    {actionRow('Give Currency', numInput(currency, setCurrency, 1, 9999999), 'Give',
                      () => run(() => api.players.giveCurrency(player.controller_id, currency), `Gave ${currency} Solari to ${player.name}`))}
                    {actionRow('Give Scrip', numInput(scrip, setScrip, 1, 9999999), 'Give',
                      () => run(() => api.players.giveScrip(player.controller_id, scrip), `Gave ${scrip} scrip to ${player.name}`))}
                    {actionRow('Award Intel', numInput(intel, setIntel, 1, 9999999), 'Award',
                      () => run(() => api.players.awardIntel(player.id, intel), `Awarded ${intel} intel to ${player.name}`))}

                    <SectionHeader className="mt-4">Character XP</SectionHeader>
                    {player.online_status === 'Online' && onlineWarning}
                    {charXPCurrent && (
                      <div className="px-1 py-2 text-xs text-muted">
                        Current: <span className="text-foreground">{charXPCurrent.xp.toLocaleString()} XP</span>
                        {' '}— Level <span className="text-foreground">{charXPCurrent.level}</span>
                        <span className="text-muted/60"> / 200</span>
                      </div>
                    )}
                    {actionRow('Award Char XP',
                      <div className="flex flex-col gap-0.5">
                        {numInput(charXP, setCharXP, 0, 344440)}
                        <span className="text-xs text-muted">Max 344,440 (level 200)</span>
                      </div>,
                      'Award',
                      () => run(() => api.players.awardCharXP(player.id, charXP), `Awarded ${charXP} char XP to ${player.name}`)
                        .then(() => api.players.charXPCurrent(player.id).then(setCharXPCurrent).catch(() => {})))}

                    <SectionHeader className="mt-4">Faction Reputation</SectionHeader>
                    <div className="flex items-center gap-2 py-3 border-b border-border/40">
                      <div className="w-36 shrink-0 text-sm text-muted">Faction</div>
                      <Select selectedKey={String(factionId)} onSelectionChange={k => setFactionId(Number(k))} className="w-40">
                        <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
                        <Select.Popover>
                          <ListBox>
                            {FACTIONS.map(f => (
                              <ListBox.Item key={String(f.id)} id={String(f.id)} textValue={f.name}>
                                {f.name}<ListBox.ItemIndicator />
                              </ListBox.Item>
                            ))}
                          </ListBox>
                        </Select.Popover>
                      </Select>
                    </div>
                    {actionRow('Reputation',
                      <div className="flex flex-col gap-0.5">
                        {numInput(repDelta, setRepDelta, 0, 12474)}
                        <span className="text-xs text-muted">Adds to current, max 12,474</span>
                      </div>,
                      'Give',
                      () => run(() => api.players.giveFactionRep(player.controller_id, factionId, repDelta), `Gave ${repDelta} rep (faction ${factionId}) to ${player.name}`))}
                  </div>
                )}

                {section === 'specs' && (
                  <div className="flex flex-col gap-3 flex-1 min-h-0">
                    <div className="flex items-center gap-3 shrink-0">
                      <h3 className="text-base font-semibold text-accent flex-1">Specializations</h3>
                      <Button size="sm" variant="ghost" isDisabled={specsLoading}
                        onPress={() => setSpecsLoaded(false)}>
                        {specsLoading ? <Spinner size="sm" color="current" /> : '↻ Refresh'}
                      </Button>
                      <Button size="sm" variant="outline" isDisabled={busy}
                        onPress={() => run(
                          () => api.players.grantAllKeystones(player.controller_id),
                          `Grant all keystones to ${player.name}`,
                        ).then(() => setSpecsLoaded(false))}>
                        Grant Max Keystones
                      </Button>
                    </div>
                    {player.online_status === 'Online' && onlineWarning}
                    {specsLoading ? (
                      <div className="flex justify-center py-8"><Spinner size="lg" /></div>
                    ) : (
                      <DataTable<string, 'track' | 'xp' | 'level' | 'grant' | 'reset'>
                        aria-label="Specializations"
                        className="min-h-0 max-h-full"
                        columns={[
                          { key: 'track', label: 'Track', isRowHeader: true },
                          { key: 'xp',    label: 'XP' },
                          { key: 'level', label: 'Level' },
                          { key: 'grant', label: '', sortable: false },
                          { key: 'reset', label: '', sortable: false },
                        ]}
                        rows={XP_TRACKS}
                        rowId={t => t}
                        initialSort={{ column: 'track', direction: 'ascending' }}
                        sortValue={(t, k) => {
                          const found = playerSpecs.find(s => s.track_type === t)
                          if (k === 'track') return t
                          if (k === 'xp')    return found?.xp ?? 0
                          if (k === 'level') return found?.level ?? 0
                          return ''
                        }}
                        renderCell={(track, key) => {
                          const found = playerSpecs.find(s => s.track_type === track)
                          const trackKeystones = playerKeystones.filter(k => k.track === track)
                          switch (key) {
                            case 'track':
                              return (
                                <span className="inline-flex flex-col font-semibold align-top">
                                  <span>{track}</span>
                                  {trackKeystones.length > 0 && (
                                    <span className="flex flex-col gap-0.5 mt-1">
                                      {trackKeystones.map(k => (
                                        <span key={k.id} className="text-xs font-mono text-muted">
                                          ↳ {k.name.replace(/^DA_\w+Keystone_/, '').replace(/_/g, ' ')}
                                          {k.cost > 0 && <span className="ml-1 text-muted/60">{k.cost}m</span>}
                                        </span>
                                      ))}
                                    </span>
                                  )}
                                </span>
                              )
                            case 'xp':    return <span className="font-mono text-muted">{(found?.xp ?? 0).toLocaleString()}</span>
                            case 'level': return <span className="font-mono text-muted">{found?.level ?? 0}</span>
                            case 'grant':
                              return (
                                <Button size="sm" variant="ghost" isDisabled={busy}
                                  onPress={() => run(
                                    () => api.players.grantMaxSpec(player.controller_id, track),
                                    `Grant max ${track} spec to ${player.name}`,
                                  ).then(() => {
                                    setPlayerSpecs(prev => {
                                      const exists = prev.find(s => s.track_type === track)
                                      if (exists) return prev.map(s => s.track_type === track ? { ...s, xp: 44182, level: 100 } : s)
                                      return [...prev, { player_id: player.controller_id, track_type: track, xp: 44182, level: 100 }]
                                    })
                                  })}>
                                  Grant Max
                                </Button>
                              )
                            case 'reset':
                              return (
                                <Button size="sm" variant="danger-soft" isDisabled={busy}
                                  onPress={() => run(
                                    () => api.players.resetSpec(player.controller_id, track),
                                    `Reset ${track} spec for ${player.name}`,
                                  ).then(() => {
                                    setPlayerSpecs(prev => prev.filter(s => s.track_type !== track))
                                  })}>
                                  Reset
                                </Button>
                              )
                          }
                        }}
                      />
                    )}
                  </div>
                )}

                {section === 'progression' && (() => {
                  const trainerMatches = (() => {
                    const re = new RegExp(`^Trainer_${selectedTrainer}\\d+(_|$)`)
                    return contractCatalog.map(c => c.alias || c.id).filter(id => re.test(id))
                  })()
                  const selectedMQDef = MAIN_QUESTS.find(m => m.id === selectedMQ)
                  return (
                  <div className="flex flex-col gap-3 flex-1 min-h-0">
                    {/* Progression Unlock */}
                    <Panel>
                      <SectionLabel>Progression Unlock</SectionLabel>
                      <div className="text-xs text-muted">Completes DA_FQ_ClimbTheRanks journey nodes and writes faction tier tags.</div>
                      <div className="flex items-center gap-2 flex-wrap">
                        <Select selectedKey={unlockFaction} onSelectionChange={k => setUnlockFaction(String(k))} className="w-36">
                          <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
                          <Select.Popover>
                            <ListBox>
                              <ListBox.Item key="atreides" id="atreides" textValue="Atreides">Atreides<ListBox.ItemIndicator /></ListBox.Item>
                              <ListBox.Item key="harkonnen" id="harkonnen" textValue="Harkonnen">Harkonnen<ListBox.ItemIndicator /></ListBox.Item>
                            </ListBox>
                          </Select.Popover>
                        </Select>
                        <Select selectedKey={unlockPreset} onSelectionChange={k => setUnlockPreset(String(k))} className="w-48">
                          <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
                          <Select.Popover>
                            <ListBox>
                              <ListBox.Item key="ch3_start" id="ch3_start" textValue="Ch3 Start">Ch3 Start<ListBox.ItemIndicator /></ListBox.Item>
                              <ListBox.Item key="rank19_eligible" id="rank19_eligible" textValue="Rank 19 Eligible">Rank 19 Eligible<ListBox.ItemIndicator /></ListBox.Item>
                            </ListBox>
                          </Select.Popover>
                        </Select>
                        <Button size="sm" variant="secondary" isDisabled={busy}
                          onPress={() => run(
                            () => api.players.progressionUnlock(player.id, unlockFaction, unlockPreset),
                            `Applied ${unlockPreset} (${unlockFaction}) to ${player.name}`,
                          ).then(() => setNodesLoaded(false))}>
                          Apply Unlock
                        </Button>
                      </div>
                    </Panel>

                    {/* Trainer + Main Quest — 2-column row */}
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-3 shrink-0">
                      {contractCatalogLoaded && !contractCatalogError && (
                        <Panel>
                          <SectionLabel>Unlock Trainer</SectionLabel>
                          <div className="text-xs text-muted">Completes every <code>Trainer_X_*</code> contract and grants the full job skill tree (Key.&lt;Job&gt;1/2/3 + 3 capstones). Reset wipes the SkillArea so the class is fully undone.</div>
                          <div className="flex items-center gap-2">
                            <Select
                              aria-label="Trainer"
                              selectedKey={selectedTrainer}
                              onSelectionChange={k => setSelectedTrainer(k as TrainerKey)}
                              className="flex-1"
                            >
                              <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
                              <Select.Popover>
                                <ListBox>
                                  {TRAINERS.map(t => (
                                    <ListBox.Item key={t} id={t} textValue={t}>
                                      {t}<ListBox.ItemIndicator />
                                    </ListBox.Item>
                                  ))}
                                </ListBox>
                              </Select.Popover>
                            </Select>
                            <Button size="sm" variant="secondary" isDisabled={busy || trainerMatches.length === 0}
                              onPress={() => run(
                                async () => {
                                  const r = await api.players.completeContracts(player.account_id, trainerMatches)
                                  await api.players.grantJobSkills(player.account_id, selectedTrainer)
                                  return r
                                },
                                `Unlocked ${selectedTrainer} (${trainerMatches.length} contracts + skill tree) for ${player.name}`,
                              ).then(() => setNodesLoaded(false))}>
                              Apply <span className="text-muted ml-1">({trainerMatches.length})</span>
                            </Button>
                            <Button size="sm" variant="danger-soft" isDisabled={busy}
                              onPress={() => run(
                                () => api.players.resetJobSkills(player.account_id, selectedTrainer),
                                `Reset ${selectedTrainer} skill tree for ${player.name}`,
                              )}>
                              Reset
                            </Button>
                          </div>
                        </Panel>
                      )}

                      <Panel>
                        <SectionLabel>Unlock Main Quest</SectionLabel>
                        <div className="text-xs text-muted">Flips every <code>DA_MQ_&lt;name&gt;.*</code> journey row complete and applies the m_TagsToAdd union (Act/Chapter markers, BigMoments triggers, Fremkit set tags, etc.).</div>
                        <div className="flex items-center gap-2">
                          <Select
                            aria-label="Main quest"
                            selectedKey={selectedMQ}
                            onSelectionChange={k => setSelectedMQ(String(k))}
                            className="flex-1"
                          >
                            <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
                            <Select.Popover>
                              <ListBox>
                                {MAIN_QUESTS.map(mq => (
                                  <ListBox.Item key={mq.id} id={mq.id} textValue={mq.label}>
                                    {mq.label}{mq.nodes > 0 && <span className="text-muted ml-2">({mq.nodes})</span>}
                                    <ListBox.ItemIndicator />
                                  </ListBox.Item>
                                ))}
                              </ListBox>
                            </Select.Popover>
                          </Select>
                          <Button size="sm" variant="secondary" isDisabled={busy}
                            onPress={() => run(
                              () => api.players.journeyComplete(player.account_id, selectedMQ),
                              `Unlocked ${selectedMQDef?.label ?? selectedMQ} for ${player.name}`,
                            ).then(() => setNodesLoaded(false))}>
                            Apply
                          </Button>
                        </div>
                      </Panel>
                    </div>

                    {/* Complete Contract(s) — fills remaining space, owns its scroll */}
                    <Panel className="flex-1 min-h-0">
                      <div className="flex items-baseline gap-2">
                        <SectionLabel>Complete Contract(s)</SectionLabel>
                        <div className="text-xs text-muted">
                          {contractCatalogError
                            ? <span className="text-danger">load failed: {contractCatalogError} — restart the server</span>
                            : contractCatalogLoaded ? `${contractCatalog.length} contracts` : 'loading…'}
                        </div>
                      </div>
                      <div className="text-xs text-muted">Applies the contract&apos;s <code>AddedFlagsOnCompletion</code> tags + tier-promotion side effects. Multi-select supported.</div>

                      {selectedContracts.length > 0 && (
                        <div className="flex flex-wrap gap-1">
                          {selectedContracts.map(id => (
                            <Chip key={id} size="sm" variant="soft">
                              <span className="font-mono">{id}</span>
                              <button
                                type="button"
                                onClick={() => setSelectedContracts(prev => prev.filter(x => x !== id))}
                                className="ml-1 text-muted hover:text-foreground"
                                aria-label={`Remove ${id}`}
                              >×</button>
                            </Chip>
                          ))}
                          <button
                            type="button"
                            onClick={() => setSelectedContracts([])}
                            className="text-xs underline text-muted"
                          >clear all</button>
                        </div>
                      )}

                      <div className="flex items-center gap-2 flex-wrap">
                        <Input
                          aria-label="Filter contracts"
                          className="flex-1 min-w-48"
                          placeholder="Filter contracts (e.g. Trainer_Mentat, Atre_Rank01)..."
                          value={contractSearch}
                          onChange={e => setContractSearch(e.target.value)}
                        />
                        <Button size="sm" variant="secondary" isDisabled={busy || selectedContracts.length === 0}
                          onPress={() => run(
                            () => api.players.completeContracts(player.account_id, selectedContracts),
                            `Completed ${selectedContracts.length} contract(s) for ${player.name}`,
                          ).then(() => { setSelectedContracts([]); setNodesLoaded(false) })}>
                          Apply Contract(s) ({selectedContracts.length})
                        </Button>
                      </div>

                      {contractCatalogLoaded && !contractCatalogError && (
                        <div className="flex-1 min-h-0 overflow-y-auto rounded border border-border bg-surface-alt">
                          {(() => {
                            const q = contractSearch.trim().toLowerCase()
                            const matches = contractCatalog.filter(c =>
                              q === '' || c.id.toLowerCase().includes(q) || (c.alias && c.alias.toLowerCase().includes(q)),
                            )
                            if (matches.length === 0) {
                              return <div className="px-2 py-3 text-xs text-center text-muted">No matching contracts</div>
                            }
                            return matches.map(c => {
                              const id = c.alias || c.id
                              const picked = selectedContracts.includes(id)
                              return (
                                <button
                                  key={c.id}
                                  type="button"
                                  onClick={() => setSelectedContracts(prev =>
                                    picked ? prev.filter(x => x !== id) : [...prev, id],
                                  )}
                                  className={
                                    'flex w-full items-center justify-between px-2 py-1 text-xs font-mono hover:bg-surface ' +
                                    (picked ? 'bg-surface text-accent' : 'bg-transparent text-foreground')
                                  }
                                >
                                  <span>{picked ? '✓ ' : '  '}{id}</span>
                                  <span className="text-muted">{c.tag_count} tag{c.tag_count === 1 ? '' : 's'}</span>
                                </button>
                              )
                            })
                          })()}
                        </div>
                      )}
                    </Panel>
                  </div>
                  )
                })()}

                {section === 'journey' && (
                  <div className="flex flex-col gap-3 flex-1 min-h-0">
                    <Panel className="flex-1 min-h-0">
                      <div className="flex items-center gap-2 shrink-0">
                        <SectionLabel>Journey Nodes</SectionLabel>
                        <div className="flex-1" />
                        <Button size="sm" variant="ghost" onPress={() => setNodesLoaded(false)} isDisabled={nodesLoading}>
                          {nodesLoading ? <Spinner size="sm" color="current" /> : '↻'}
                        </Button>
                        <Button size="sm" variant="danger-soft" isDisabled={busy}
                          onPress={() => run(() => api.players.journeyWipe(player.account_id), `Wiped all journey nodes for ${player.name}`)
                            .then(() => setNodes([]))}>
                          Wipe All
                        </Button>
                      </div>
                      <Input
                        aria-label="Filter journey nodes"
                        className="shrink-0"
                        placeholder="Filter nodes..."
                        value={nodeSearch}
                        onChange={e => setNodeSearch(e.target.value)}
                      />
                    {nodesLoading ? (
                      <div className="flex justify-center py-8"><Spinner size="lg" /></div>
                    ) : (
                      <DataTable<JourneyNode, 'node' | 'done' | 'revealed' | 'reward' | 'actions'>
                        aria-label="Journey nodes"
                        className="min-h-0 max-h-full"
                        virtualized
                        rowHeight={36}
                        columns={[
                          { key: 'node',     label: 'Node ID', isRowHeader: true, minWidth: 240 },
                          { key: 'done',     label: 'Done',     width: 80 },
                          { key: 'revealed', label: 'Revealed', width: 100 },
                          { key: 'reward',   label: 'Reward',   width: 90 },
                          { key: 'actions',  label: '', sortable: false, width: 220 },
                        ]}
                        rows={filteredNodes}
                        rowId={n => n.node_id}
                        initialSort={{ column: 'node', direction: 'ascending' }}
                        sortValue={(n, k) => {
                          if (k === 'node')     return n.node_id
                          if (k === 'done')     return n.is_complete ? 1 : 0
                          if (k === 'revealed') return n.is_revealed ? 1 : 0
                          if (k === 'reward')   return n.has_pending_reward ? 1 : 0
                          return ''
                        }}
                        emptyState={
                          <div className="text-center py-8 text-xs text-muted">
                            {nodes.length === 0 ? 'No journey nodes found' : 'No matching nodes'}
                          </div>
                        }
                        renderCell={(n, key) => {
                          switch (key) {
                            case 'node':     return <span className="font-mono">{n.node_id}</span>
                            case 'done':     return n.is_complete ? '✓' : '—'
                            case 'revealed': return n.is_revealed ? '✓' : '—'
                            case 'reward':   return n.has_pending_reward ? '✓' : '—'
                            case 'actions':
                              return (
                                <div className="grid grid-cols-2 gap-1 w-full">
                                  <Button size="sm" variant="ghost" isDisabled={busy} className="w-full"
                                    onPress={() => run(
                                      () => api.players.journeyComplete(player.account_id, n.node_id),
                                      `Completed ${n.node_id}`,
                                    ).then(() => {
                                      setNodes(prev => prev.map(x =>
                                        x.node_id === n.node_id || x.node_id.startsWith(n.node_id + '.')
                                          ? { ...x, is_complete: true, is_revealed: true }
                                          : x,
                                      ))
                                    })}>
                                    {n.is_complete ? '↻ Re-do' : 'Complete'}
                                  </Button>
                                  <Button size="sm" variant="danger-soft" isDisabled={busy} className="w-full"
                                    onPress={() => run(
                                      () => api.players.journeyReset(player.account_id, n.node_id),
                                      `Reset ${n.node_id}`,
                                    ).then(() => {
                                      setNodes(prev => prev.map(x =>
                                        x.node_id === n.node_id || x.node_id.startsWith(n.node_id + '.')
                                          ? { ...x, is_complete: false, has_pending_reward: false }
                                          : x,
                                      ))
                                    })}>
                                    Reset
                                  </Button>
                                </div>
                              )
                          }
                        }}
                      />
                    )}
                    </Panel>
                  </div>
                )}

                {section === 'admin' && (
                  <div className="overflow-y-auto flex-1 flex flex-col gap-3 pr-1">
                    <Panel>
                      <SectionLabel>Reset Actions</SectionLabel>
                      {actionRow('Delete Tutorials',
                        <span className="text-xs text-muted">Removes all tutorial completion records</span>,
                        'Delete',
                        () => run(() => api.players.deleteTutorials(player.id), `Deleted tutorials for ${player.name}`), true)}
                      {actionRow('Wipe Codex',
                        <span className="text-xs text-muted">Clears all codex discoveries</span>,
                        'Wipe',
                        () => run(() => api.players.wipeCodex(player.account_id), `Wiped codex for ${player.name}`), true)}
                      <div className="hidden">
                        {actionRow('Returning Player Award',
                          <span className="text-xs text-muted">Reset returning player status — triggers award on next login</span>,
                          'Grant',
                          () => run(() => api.players.returningPlayerAward(player.account_id), `Returning player award reset for ${player.name}`), true)}
                      </div>
                      {actionRow('Dismiss Returning Player Popup',
                        <span className="text-xs text-muted">Marks the award as claimed so the login popup stops appearing</span>,
                        'Dismiss',
                        () => run(() => api.players.dismissReturningPlayerAward(player.account_id), `Dismissed returning player popup for ${player.name}`), true)}
                    </Panel>

                    <Panel>
                      <SectionLabel>Character Export</SectionLabel>
                      <div className="flex items-end gap-3 py-1">
                        <div className="flex-1 text-xs text-muted">Download character data as JSON</div>
                        <a href={api.players.exportUrl(player.account_id)} download>
                          <Button size="sm" variant="ghost" isDisabled={busy}>Download</Button>
                        </a>
                      </div>
                    </Panel>

                    <Panel>
                      <SectionLabel>Teleport</SectionLabel>
                      <div className="flex items-end gap-3 py-1">
                        <Select
                          aria-label="Teleport destination"
                          placeholder="Select location..."
                          selectedKey={selectedPartition || null}
                          onSelectionChange={k => setSelectedPartition(k ? String(k) : '')}
                          className="flex-1"
                        >
                          <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
                          <Select.Popover>
                            <ListBox>
                              {partitions.map(p => (
                                <ListBox.Item key={p.name} id={p.name} textValue={p.name}>
                                  {p.name}<ListBox.ItemIndicator />
                                </ListBox.Item>
                              ))}
                            </ListBox>
                          </Select.Popover>
                        </Select>
                        <Button size="sm" variant="ghost" isDisabled={busy || !selectedPartition || player.online_status === 'Online'}
                          onPress={() => run(() => api.players.teleport(player.fls_id, selectedPartition), `Teleported ${player.name} to ${selectedPartition}`)}>
                          Move
                        </Button>
                      </div>
                      {player.online_status === 'Online' && (
                        <span className="text-xs text-muted">Player must be offline</span>
                      )}
                    </Panel>
                  </div>
                )}

                {section === 'tags' && (
                  <div className="flex flex-col gap-3 flex-1 min-h-0 overflow-y-auto pr-1">
                    <Panel>
                      <SectionLabel>Add Tags</SectionLabel>
                      <div className="flex gap-2 items-center">
                        <Select
                          aria-label="Add tag"
                          placeholder="Select a tag to stage…"
                          selectedKey={null}
                          onSelectionChange={k => {
                            const tag = k ? String(k) : ''
                            if (tag && !tags.includes(tag) && !pendingTags.includes(tag)) {
                              setPendingTags(prev => [...prev, tag])
                            }
                          }}
                          className="flex-1"
                        >
                          <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
                          <Select.Popover className="!w-[500px] !max-w-[90vw]">
                            {/* Virtualized — only renders visible rows, so 2,676 tags don't freeze the UI */}
                            <Virtualizer layout={ListLayout} layoutOptions={{ rowHeight: 32 }}>
                              <ListBox
                                aria-label="Gameplay tags"
                                className="h-[300px] overflow-y-auto"
                                items={(allGameplayTags as string[])
                                  .filter(t => !tags.includes(t) && !pendingTags.includes(t))
                                  .map(t => ({ id: t }))}
                              >
                                {(item: { id: string }) => (
                                  <ListBox.Item id={item.id} textValue={item.id}>
                                    {item.id}<ListBox.ItemIndicator />
                                  </ListBox.Item>
                                )}
                              </ListBox>
                            </Virtualizer>
                          </Select.Popover>
                        </Select>
                        <Button
                          size="sm" isDisabled={pendingTags.length === 0}
                          onPress={() => {
                            const toAdd = pendingTags
                            run(() => api.players.updateTags(player.account_id, toAdd, []), `Added ${toAdd.length} tag${toAdd.length > 1 ? 's' : ''}`)
                              .then(() => { setTags(prev => [...new Set([...prev, ...toAdd])].sort()); setPendingTags([]) })
                          }}
                        >Add {pendingTags.length > 0 ? `(${pendingTags.length})` : ''}</Button>
                      </div>
                      {pendingTags.length > 0 && (
                        <div className="flex flex-wrap gap-1.5">
                          {pendingTags.map(tag => (
                            <Chip key={tag} size="sm" color="success" variant="soft">
                              <span className="font-mono">{tag}</span>
                              <button
                                onClick={() => setPendingTags(prev => prev.filter(t => t !== tag))}
                                className="ml-1 text-success hover:opacity-70"
                                aria-label={`Remove ${tag}`}
                              >✕</button>
                            </Chip>
                          ))}
                        </div>
                      )}
                    </Panel>

                    {tagsLoading ? (
                      <div className="flex justify-center py-8"><Spinner size="lg" /></div>
                    ) : (
                      <DataTable<string, 'tag' | 'actions'>
                        aria-label="Active tags"
                        className="min-h-0 max-h-full"
                        columns={[
                          { key: 'tag',     label: `Tag (${tags.length})`, isRowHeader: true },
                          { key: 'actions', label: '', sortable: false },
                        ]}
                        rows={tags}
                        rowId={t => t}
                        initialSort={{ column: 'tag', direction: 'ascending' }}
                        sortValue={t => t}
                        emptyState={<div className="py-6 text-center text-muted">No tags</div>}
                        renderCell={(tag, key) => {
                          if (key === 'tag') return <span className="font-mono">{tag}</span>
                          return (
                            <Button
                              size="sm" variant="danger-soft"
                              onPress={() => run(() => api.players.updateTags(player.account_id, [], [tag]), 'Removed tag')
                                .then(() => setTags(prev => prev.filter(t => t !== tag)))}
                              aria-label={`Remove ${tag}`}
                            >✕</Button>
                          )
                        }}
                      />
                    )}
                  </div>
                )}

                {section === 'history' && (
                  <div className="flex flex-col gap-3 flex-1 min-h-0 overflow-y-auto pr-1">
                    {historyLoading ? (
                      <div className="flex justify-center py-8"><Spinner size="lg" /></div>
                    ) : (
                      <>
                        <Panel>
                          <SectionLabel>Game Events</SectionLabel>
                          <DataTable<GameEvent, 'time' | 'map' | 'event_type' | 'location'>
                            aria-label="Game events"
                            className="max-h-[40vh]"
                            columns={[
                              { key: 'time',       label: 'Time', isRowHeader: true },
                              { key: 'map',        label: 'Map' },
                              { key: 'event_type', label: 'Event Type' },
                              { key: 'location',   label: 'Location', sortable: false },
                            ]}
                            rows={events}
                            rowId={evt => `${evt.actor_id}-${evt.universe_time}`}
                            initialSort={{ column: 'time', direction: 'descending' }}
                            sortValue={(evt, k) => {
                              if (k === 'time')       return evt.universe_time
                              if (k === 'map')        return evt.map
                              if (k === 'event_type') return evt.event_type
                              return ''
                            }}
                            emptyState={<div className="py-6 text-center text-muted">No events</div>}
                            renderCell={(evt, key) => {
                              switch (key) {
                                case 'time': return <span className="font-mono text-muted">{evt.universe_time}</span>
                                case 'map':  return <span className="text-muted">{evt.map}</span>
                                case 'event_type':
                                  return <Chip size="sm" color={eventColor(evt.event_type)} variant="soft">{evt.event_type}</Chip>
                                case 'location':
                                  return <span className="font-mono text-muted">{Math.round(evt.x)}, {Math.round(evt.y)}, {Math.round(evt.z)}</span>
                              }
                            }}
                          />
                        </Panel>

                        <Panel>
                          <SectionLabel>Dungeon Records</SectionLabel>
                          <DataTable<DungeonRecord, 'dungeon' | 'difficulty' | 'duration' | 'party'>
                            aria-label="Dungeon records"
                            className="max-h-[40vh]"
                            columns={[
                              { key: 'dungeon',    label: 'Dungeon', isRowHeader: true },
                              { key: 'difficulty', label: 'Difficulty' },
                              { key: 'duration',   label: 'Duration' },
                              { key: 'party',      label: 'Party Size' },
                            ]}
                            rows={dungeons}
                            rowId={d => String(d.completion_id)}
                            initialSort={{ column: 'dungeon', direction: 'ascending' }}
                            sortValue={(d, k) => {
                              if (k === 'dungeon')    return d.dungeon_id
                              if (k === 'difficulty') return d.difficulty
                              if (k === 'duration')   return d.duration_ms
                              return d.players_num
                            }}
                            emptyState={<div className="py-6 text-center text-muted">No dungeon records</div>}
                            renderCell={(d, key) => {
                              switch (key) {
                                case 'dungeon':    return <span className="font-semibold">{d.dungeon_id}</span>
                                case 'difficulty': return <Chip size="sm" color={difficultyColor(d.difficulty)} variant="soft">{d.difficulty}</Chip>
                                case 'duration':   return <span className="font-mono text-muted">{formatDuration(d.duration_ms)}</span>
                                case 'party':      return <span className="text-muted">{d.players_num}</span>
                              }
                            }}
                          />
                        </Panel>
                      </>
                    )}
                  </div>
                )}
              </div>
            </Modal.Body>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

type ChipColor = 'default' | 'accent' | 'success' | 'warning' | 'danger'

function eventColor(eventType: number): ChipColor {
  // event_type is a numeric ID; cycle through palette colors for at-a-glance distinction
  const palette: ChipColor[] = ['accent', 'success', 'warning', 'danger', 'default']
  return palette[eventType % palette.length]
}

function difficultyColor(difficulty: string): ChipColor {
  const d = difficulty?.toLowerCase() ?? ''
  if (d.includes('easy') || d.includes('normal')) return 'success'
  if (d.includes('hard') || d.includes('elite')) return 'warning'
  if (d.includes('nightmare') || d.includes('extreme') || d.includes('mythic')) return 'danger'
  return 'accent'
}

function SectionHeader({ children, className = '' }: { children: ReactNode; className?: string }) {
  return (
    <h3 className={'text-xs font-semibold uppercase tracking-widest text-accent pb-2 mb-2 border-b border-border ' + className}>
      {children}
    </h3>
  )
}

function SectionLabel({ children }: { children: ReactNode }) {
  return <h4 className="text-xs font-semibold uppercase tracking-widest text-accent">{children}</h4>
}

