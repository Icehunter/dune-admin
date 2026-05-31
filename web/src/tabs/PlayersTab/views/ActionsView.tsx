import { useState, useEffect, useMemo, useCallback, memo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button,
  Chip,
  Input,
  ListBox,
  ListLayout,
  SearchField,
  Select,
  Spinner,
  Virtualizer,
  toast,
} from '@heroui/react'
import { ConfirmDialog, DataTable, Icon, Panel, SectionLabel } from '../../../dune-ui'
import allGameplayTags from '../../../data/gameplayTags.json'
import allSkillModules from '../../../data/skillModules.json'
import allVehicles from '../../../data/vehicles.json'
import { api } from '../../../api/client'
import type {
  Player,
  JourneyNode,
  SpecTrack,
  KeystoneRow,
  TeleportLocation,
  GameEvent,
  DungeonRecord,
  ProgressionPreset,
} from '../../../api/client'
import { ACTION_SECTIONS, XP_TRACKS, FACTIONS, type ActionSection } from '../types'

interface Props {
  player: Player
}

const TRAINERS = ['BeneGesserit', 'Mentat', 'Planetologist', 'Swordmaster', 'Trooper'] as const
type TrainerKey = (typeof TRAINERS)[number]

const MAIN_QUESTS = [
  { id: 'DA_MQ_ANewBeginning', label: '1. A New Beginning', nodes: 132 },
  { id: 'DA_MQ_AssassinsHandbook', label: '2. Assassin’s Handbook', nodes: 91 },
  { id: 'DA_MQ_FindTheFremen', label: '3. Find the Fremen', nodes: 46 },
  { id: 'DA_MQ_TheGreatConvention', label: '4. The Great Convention', nodes: 90 },
  { id: 'DA_MQ_TheGreatConventionPt2', label: '5. Great Convention Pt 2', nodes: 109 },
  { id: 'DA_MQ_TheBloodline', label: '6. The Bloodline (standalone)', nodes: 0 },
] as const

function useDebounce<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(t)
  }, [value, delay])
  return debounced
}

function DebouncedSearchField({
  onSearch,
  placeholder,
  className,
}: {
  onSearch: (q: string) => void
  placeholder?: string
  className?: string
}) {
  const [value, setValue] = useState('')
  const debounced = useDebounce(value)
  useEffect(() => {
    onSearch(debounced)
  }, [debounced, onSearch])
  return (
    <SearchField aria-label="Search" className={className} value={value} onChange={setValue}>
      <SearchField.Group>
        <SearchField.SearchIcon />
        <SearchField.Input placeholder={placeholder} />
        <SearchField.ClearButton />
      </SearchField.Group>
    </SearchField>
  )
}

function KeystonesToggle({ keystones }: { keystones: KeystoneRow[] }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="mt-0.5">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="text-xs text-muted/70 hover:text-muted flex items-center gap-0.5"
      >
        <span>{open ? '▾' : '▸'}</span>
        {keystones.length}
        {' '}
        keystone
        {keystones.length !== 1 ? 's' : ''}
      </button>
      {open && (
        <div className="flex flex-col gap-0.5 mt-0.5">
          {keystones.map((k) => (
            <span key={k.id} className="text-xs font-mono text-muted">
              ↳
              {' '}
              {k.name.replace(/^DA_\w+Keystone_/, '').replace(/_/g, ' ')}
              {k.cost > 0 && (
                <span className="ml-1 text-muted/60">
                  {k.cost}
                  m
                </span>
              )}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}

const AddTagsPanel = memo(function AddTagsPanel({
  tags,
  pendingTags,
  onAdd,
}: {
  tags: string[]
  pendingTags: string[]
  onAdd: (tag: string) => void
}) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const debouncedQuery = useDebounce(query)

  const matches = useMemo(() => {
    if (!debouncedQuery) return []
    const tagsSet = new Set(tags)
    const pendingSet = new Set(pendingTags)
    const q = debouncedQuery.toLowerCase()
    return (allGameplayTags as string[])
      .filter((t) => !tagsSet.has(t) && !pendingSet.has(t) && t.toLowerCase().includes(q))
      .slice(0, 100)
  }, [debouncedQuery, tags, pendingTags])

  return (
    <div className="relative">
      <SearchField value={query} onChange={setQuery} variant="secondary">
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input placeholder={t('players.actions.tags.searchPlaceholder')} />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>
      {query && matches.length > 0 && (
        <div className="absolute z-50 w-full mt-1 max-h-52 overflow-y-auto rounded-[var(--radius)] border border-border bg-surface">
          {matches.map((t) => (
            <div
              key={t}
              className="px-3 py-1.5 text-xs font-mono cursor-pointer hover:bg-surface-hover"
              onMouseDown={(e) => {
                e.preventDefault()
                onAdd(t)
                setQuery('')
              }}
            >
              {t}
            </div>
          ))}
        </div>
      )}
    </div>
  )
})

export function ActionsView({ player }: Props) {
  const { t } = useTranslation()
  const [section, setSection] = useState<ActionSection>('resources')
  const [busy, setBusy] = useState(false)

  const [currency, setCurrency] = useState(100)
  const [scrip, setScrip] = useState(100)
  const [intel, setIntel] = useState(100)

  const [charXP, setCharXP] = useState(1000)
  const [charXPCurrent, setCharXPCurrent] = useState<{ xp: number, level: number } | null>(null)

  const [factionId, setFactionId] = useState(player.faction_id || 0)
  const [repDelta, setRepDelta] = useState(100)

  const [playerSpecs, setPlayerSpecs] = useState<SpecTrack[]>([])
  const [playerKeystones, setPlayerKeystones] = useState<KeystoneRow[]>([])
  const [specsLoaded, setSpecsLoaded] = useState(false)
  const [specsLoading, setSpecsLoading] = useState(false)

  const [nodes, setNodes] = useState<JourneyNode[]>([])
  const [nodesLoaded, setNodesLoaded] = useState(false)
  const [nodesLoading, setNodesLoading] = useState(false)
  const [nodeSearch, setNodeSearch] = useState('')
  const [unlockFaction, setUnlockFaction] = useState('atreides')
  const [unlockPreset, setUnlockPreset] = useState('ch3_start')

  const [customScriptName, setCustomScriptName] = useState('')

  const [selectedTrainer, setSelectedTrainer] = useState<TrainerKey>('BeneGesserit')
  const [selectedMQ, setSelectedMQ] = useState<string>('DA_MQ_ANewBeginning')

  const [presets, setPresets] = useState<ProgressionPreset[]>([])
  const [presetsLoaded, setPresetsLoaded] = useState(false)

  const [contractCatalog, setContractCatalog] = useState<{ id: string, alias: string, tag_count: number }[]>([])
  const [contractCatalogLoaded, setContractCatalogLoaded] = useState(false)
  const [contractCatalogError, setContractCatalogError] = useState('')
  const [contractSearch, setContractSearch] = useState('')
  const [selectedContracts, setSelectedContracts] = useState<string[]>([])

  const [tags, setTags] = useState<string[]>([])
  const [tagsLoaded, setTagsLoaded] = useState(false)
  const [tagsLoading, setTagsLoading] = useState(false)
  const [pendingTags, setPendingTags] = useState<string[]>([])
  const [tagRemoveSearch, setTagRemoveSearch] = useState('')

  const handleAddTag = useCallback((tag: string) => {
    setPendingTags((prev) => [...prev, tag])
  }, [])

  const filteredActiveTags = useMemo(() => {
    const q = tagRemoveSearch.toLowerCase()
    return q ? tags.filter((t) => t.toLowerCase().includes(q)) : tags
  }, [tags, tagRemoveSearch])

  const [skillPointsAmount, setSkillPointsAmount] = useState(10)
  const [skillModule, setSkillModule] = useState('')
  const [skillModuleLevel, setSkillModuleLevel] = useState(1)
  const [confirmPending, setConfirmPending] = useState<{
    title: string
    description: string
    confirmLabel: string
    onConfirm: () => void
  } | null>(null)

  const [partitions, setPartitions] = useState<TeleportLocation[]>([])
  const [selectedPartition, setSelectedPartition] = useState('')
  const [allPlayers, setAllPlayers] = useState<Player[]>([])
  const [selectedTeleportTarget, setSelectedTeleportTarget] = useState<number | null>(null)
  const [targetSearch, setTargetSearch] = useState('')

  const [whisperText, setWhisperText] = useState('')
  const [whisperSenderName, setWhisperSenderName] = useState('GM')

  const [spawnVehicleId, setSpawnVehicleId] = useState('')
  const [spawnVehicleTemplate, setSpawnVehicleTemplate] = useState('')
  const [spawnVehiclePartition, setSpawnVehiclePartition] = useState('')
  const [spawnVehiclePersistent, setSpawnVehiclePersistent] = useState(true)

  const [events, setEvents] = useState<GameEvent[]>([])
  const [dungeons, setDungeons] = useState<DungeonRecord[]>([])
  const [historyLoaded, setHistoryLoaded] = useState(false)
  const [historyLoading, setHistoryLoading] = useState(false)

  // Reset per-player state when player changes
  useEffect(() => {
    Promise.resolve().then(() => {
      setSection('resources')
      setNodesLoaded(false)
      setNodes([])
      setPlayerSpecs([])
      setPlayerKeystones([])
      setSpecsLoaded(false)
      setHistoryLoaded(false)
      setEvents([])
      setDungeons([])
      setCharXPCurrent(null)
      setTagsLoaded(false)
      setTags([])
      setPendingTags([])
      setConfirmPending(null)
      setContractCatalogLoaded(false)
      setContractCatalog([])
      setContractCatalogError('')
      setPresetsLoaded(false)
      setPresets([])
      setSelectedContracts([])
    })
  }, [player.id])

  // Load always-needed data on mount / player change
  useEffect(() => {
    Promise.resolve()
      .then(() => setFactionId(player.faction_id > 0 ? player.faction_id : 1))
      .then(() => Promise.all([api.players.partitions(), api.players.charXPCurrent(player.id), api.players.list()]))
      .then(([parts, xp, ps]) => {
        setPartitions(parts)
        setCharXPCurrent(xp)
        setAllPlayers(ps.filter((p) => p.id !== player.id))
      })
      .catch(() => {})
  }, [player.id, player.faction_id])

  useEffect(() => {
    if (section === 'journey' && !nodesLoaded) {
      Promise.resolve()
        .then(() => setNodesLoading(true))
        .then(() => api.players.journey(player.account_id))
        .then((n) => {
          setNodes(n)
          setNodesLoaded(true)
        })
        .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
        .finally(() => setNodesLoading(false))
    }
    if ((section === 'progression' || section === 'contracts') && !contractCatalogLoaded) {
      api.contracts
        .list()
        .then((c) => {
          setContractCatalog(c)
          setContractCatalogLoaded(true)
          setContractCatalogError('')
        })
        .catch((e: unknown) => {
          setContractCatalogError(e instanceof Error ? e.message : String(e))
          setContractCatalogLoaded(true)
        })
    }
    if (section === 'progression' && !presetsLoaded) {
      api.progression
        .presets()
        .then((p) => {
          setPresets(p)
          setPresetsLoaded(true)
        })
        .catch(() => setPresetsLoaded(true))
    }
  }, [section, nodesLoaded, contractCatalogLoaded, presetsLoaded, player.account_id])

  useEffect(() => {
    if (section === 'specs' && !specsLoaded) {
      Promise.resolve()
        .then(() => setSpecsLoading(true))
        .then(() =>
          Promise.all([api.players.specs_for(player.controller_id), api.players.keystones(player.controller_id)]),
        )
        .then(([s, k]) => {
          setPlayerSpecs(s)
          setPlayerKeystones(k)
          setSpecsLoaded(true)
        })
        .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
        .finally(() => setSpecsLoading(false))
    }
  }, [section, specsLoaded, player.controller_id])

  useEffect(() => {
    if (section === 'history' && !historyLoaded) {
      Promise.resolve()
        .then(() => setHistoryLoading(true))
        .then(() => Promise.all([api.players.events(player.id), api.players.dungeons(player.id)]))
        .then(([evts, dngns]) => {
          setEvents(evts)
          setDungeons(dngns)
          setHistoryLoaded(true)
        })
        .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
        .finally(() => setHistoryLoading(false))
    }
  }, [section, historyLoaded, player.id])

  useEffect(() => {
    if (section !== 'tags' || tagsLoaded) return
    Promise.resolve()
      .then(() => setTagsLoading(true))
      .then(() => api.players.tags(player.account_id))
      .then((t) => {
        setTags(t)
        setTagsLoaded(true)
      })
      .catch(() => {})
      .finally(() => setTagsLoading(false))
  }, [section, tagsLoaded, player.account_id])

  const run = async (fn: () => Promise<unknown>, label: string) => {
    setBusy(true)
    try {
      await fn()
      toast.success(label)
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
    finally {
      setBusy(false)
    }
  }

  const gate = (title: string, description: string, confirmLabel: string, action: () => void) => {
    setConfirmPending({ title, description, confirmLabel, onConfirm: action })
  }

  const filteredNodes = useMemo(() => {
    if (!nodeSearch) return nodes
    const q = nodeSearch.toLowerCase()
    return nodes.filter((n) => n.node_id.toLowerCase().includes(q))
  }, [nodes, nodeSearch])

  const numInput = (val: number, set: (v: number) => void, min = 1, max = 9999999) => (
    <Input
      type="number"
      min={min}
      max={max}
      value={val}
      onChange={(e) => set(Math.max(min, Math.min(max, parseInt(e.target.value) || min)))}
      aria-label="number"
      className="w-28"
    />
  )

  const actionRow = (
    label: string,
    inputs: ReactNode,
    btnLabel: string,
    onAction: () => void,
    danger = false,
    confirmGate?: { title: string, description: string },
  ) => (
    <div className="flex items-end gap-3 py-3 border-b border-border/40 last:border-b-0">
      <div className="w-36 shrink-0 text-sm text-muted">{label}</div>
      <div className="flex items-end gap-2 flex-1 flex-wrap">{inputs}</div>
      <Button
        size="sm"
        variant={danger ? 'danger-soft' : 'ghost'}
        isDisabled={busy}
        onPress={confirmGate ? () => gate(confirmGate.title, confirmGate.description, btnLabel, onAction) : onAction}
      >
        {btnLabel}
      </Button>
    </div>
  )

  const onlineWarning = (
    <div className="text-xs px-3 py-2 rounded mb-3 bg-warning/10 border border-warning text-warning">
      {t('players.actions.specs.onlineWarning')}
    </div>
  )

  const formatDuration = (ms: number) => {
    const secs = Math.floor(ms / 1000)
    const m = Math.floor(secs / 60)
    const s = secs % 60
    return `${m}:${String(s).padStart(2, '0')}`
  }

  return (
    <>
      <div className="flex gap-3 h-full min-h-0 overflow-hidden">
        {/* Section nav */}
        <div className="shrink-0">
          <div className="flex flex-col gap-1 p-0 border border-border/60 rounded-[var(--radius)] bg-background w-[140px]">
            {ACTION_SECTIONS.map((s) => {
              const isActive = section === s.key
              return (
                <button
                  key={s.key}
                  onClick={() => setSection(s.key)}
                  className={
                    'text-left px-3 py-2 rounded-[var(--radius)] text-sm transition-colors '
                    + (isActive
                      ? 'bg-accent text-accent-foreground font-semibold'
                      : 'text-foreground hover:bg-surface-hover')
                  }
                >
                  {t(s.label as never)}
                </button>
              )
            })}
          </div>
        </div>

        {/* Section content */}
        <div className="flex-1 min-w-0 min-h-0 flex flex-col overflow-hidden">
          {section === 'resources' && (
            <div className="flex-1 overflow-y-auto flex flex-col gap-3 pr-2">
              <Panel>
                <SectionLabel>{t('players.actions.resources.currencyResources')}</SectionLabel>
                {actionRow(t('players.actions.resources.giveCurrency'), numInput(currency, setCurrency, 1, 9999999), t('players.actions.resources.give'), () =>
                  run(
                    () => api.players.giveCurrency(player.controller_id, currency),
                    `Gave ${currency} Solari to ${player.name}`,
                  ),
                )}
                {actionRow(t('players.actions.resources.giveScrip'), numInput(scrip, setScrip, 1, 9999999), t('players.actions.resources.give'), () =>
                  run(
                    () => api.players.giveScrip(player.controller_id, scrip),
                    `Gave ${scrip} scrip to ${player.name}`,
                  ),
                )}
                {actionRow(t('players.actions.resources.awardIntel'), numInput(intel, setIntel, 1, 9999999), t('players.actions.resources.award'), () =>
                  run(
                    () => api.players.awardIntel(player.id, intel),
                    `Awarded ${intel} intel to ${player.name}`,
                  ),
                )}
              </Panel>

              <Panel>
                <SectionLabel>{t('players.actions.resources.characterXP')}</SectionLabel>
                {charXPCurrent && (
                  <div className="text-xs text-muted mb-2">
                    {t('players.actions.resources.currentXP', { xp: charXPCurrent.xp.toLocaleString(), level: charXPCurrent.level })}
                  </div>
                )}
                {actionRow(
                  t('players.actions.resources.awardCharXP'),
                  <div className="flex flex-col gap-0.5">
                    {numInput(charXP, setCharXP, 0, 344440)}
                    <span className="text-xs text-muted">
                      {t('players.actions.resources.charXPNote')}
                    </span>
                  </div>,
                  t('players.actions.resources.award'),
                  () =>
                    run(
                      () => api.players.awardCharXP(player.id, charXP, player.fls_id),
                      `Awarded ${charXP} char XP to ${player.name}`,
                    ).then(() =>
                      api.players
                        .charXPCurrent(player.id)
                        .then(setCharXPCurrent)
                        .catch(() => {}),
                    ),
                )}
              </Panel>

              <Panel>
                <SectionLabel>{t('players.actions.resources.liveActions')}</SectionLabel>
                <div className="text-xs text-muted mb-2">{t('players.actions.resources.liveActionsNote')}</div>
                {actionRow(
                  t('players.actions.resources.skillPoints'),
                  <div className="flex flex-col gap-0.5">
                    {numInput(skillPointsAmount, setSkillPointsAmount, 0, 9999)}
                    <span className="text-xs text-muted">{t('players.actions.resources.skillPointsNote')}</span>
                  </div>,
                  t('players.actions.resources.set'),
                  () =>
                    run(
                      () => api.players.setSkillPoints(player.fls_id, skillPointsAmount),
                      `Set skill points for ${player.name}`,
                    ),
                )}
                {actionRow(
                  t('players.actions.resources.fillWater'),
                  <span className="text-xs text-muted">{t('players.actions.resources.fillWaterNote')}</span>,
                  t('players.actions.resources.fill'),
                  () =>
                    run(
                      () => api.players.fillWater(player.fls_id),
                      `Fill water command sent for ${player.name}`,
                    ),
                )}
                {actionRow(
                  t('players.actions.resources.setSkillModule'),
                  <div className="flex items-center gap-2">
                    <Select
                      aria-label={t('players.actions.resources.skillModules')}
                      placeholder={t('players.actions.resources.selectModule')}
                      selectedKey={skillModule || null}
                      onSelectionChange={(k) => setSkillModule(k ? String(k) : '')}
                      className="w-52"
                    >
                      <Select.Trigger className="overflow-hidden">
                        <Select.Value className="truncate" />
                        <Select.Indicator />
                      </Select.Trigger>
                      <Select.Popover className="!w-[380px]">
                        <Virtualizer layout={ListLayout} layoutOptions={{ rowHeight: 32 }}>
                          <ListBox
                            aria-label={t('players.actions.resources.skillModules')}
                            className="h-[300px] overflow-y-auto"
                            items={(allSkillModules as { id: string, label: string }[]).map((m) => ({
                              id: m.id,
                              label: m.label,
                            }))}
                          >
                            {(item: { id: string, label: string }) => (
                              <ListBox.Item key={item.id} id={item.id} textValue={item.label}>
                                {item.label}
                                <ListBox.ItemIndicator />
                              </ListBox.Item>
                            )}
                          </ListBox>
                        </Virtualizer>
                      </Select.Popover>
                    </Select>
                    {numInput(skillModuleLevel, setSkillModuleLevel, 0, 5)}
                  </div>,
                  t('players.actions.resources.set'),
                  () =>
                    run(
                      () => api.players.setSkillModule(player.fls_id, skillModule, skillModuleLevel),
                      `Set ${skillModule} level ${skillModuleLevel} for ${player.name}`,
                    ),
                )}
              </Panel>

              <Panel>
                <SectionLabel>{t('players.actions.resources.factionReputation')}</SectionLabel>
                <div className="flex items-center gap-2 py-3 border-b border-border/40">
                  <div className="w-36 shrink-0 text-sm text-muted">{t('players.actions.resources.faction')}</div>
                  <Select
                    selectedKey={String(factionId)}
                    onSelectionChange={(k) => setFactionId(Number(k))}
                    className="w-40"
                  >
                    <Select.Trigger>
                      <Select.Value />
                      <Select.Indicator />
                    </Select.Trigger>
                    <Select.Popover>
                      <ListBox>
                        {FACTIONS.map((f) => (
                          <ListBox.Item key={String(f.id)} id={String(f.id)} textValue={f.name}>
                            {f.name}
                            <ListBox.ItemIndicator />
                          </ListBox.Item>
                        ))}
                      </ListBox>
                    </Select.Popover>
                  </Select>
                </div>
                {actionRow(
                  t('players.actions.resources.reputation'),
                  <div className="flex flex-col gap-0.5">
                    {numInput(repDelta, setRepDelta, 0, 12474)}
                    <span className="text-xs text-muted">{t('players.actions.resources.reputationNote')}</span>
                  </div>,
                  t('players.actions.resources.give'),
                  () =>
                    run(
                      () => api.players.giveFactionRep(player.controller_id, factionId, repDelta),
                      `Gave ${repDelta} rep (faction ${factionId}) to ${player.name}`,
                    ),
                )}
              </Panel>
            </div>
          )}

          {section === 'specs' && (
            <div className="flex flex-col gap-3 flex-1 min-h-0 overflow-hidden">
              <div className="flex items-center gap-3 min-h-8">
                <div className="flex-1"><SectionLabel>{t('players.actions.specs.specializations')}</SectionLabel></div>
                <Button size="sm" variant="ghost" isDisabled={specsLoading} onPress={() => setSpecsLoaded(false)}>
                  {specsLoading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  isDisabled={busy || player.online_status === 'Online'}
                  onPress={() =>
                    run(
                      () => api.players.grantAllKeystones(player.controller_id),
                      `Grant all keystones to ${player.name}`,
                    ).then(() => setSpecsLoaded(false))}
                >
                  {t('players.actions.specs.grantMaxKeystones')}
                </Button>
                <Button
                  size="sm"
                  variant="danger-soft"
                  isDisabled={busy || player.online_status === 'Online'}
                  onPress={() =>
                    gate(
                      t('players.actions.specs.resetKeystonesTitle'),
                      t('players.actions.specs.resetKeystonesDesc', { player: player.name }),
                      t('players.actions.specs.resetAllKeystones'),
                      () =>
                        run(
                          () => api.players.resetAllKeystones(player.controller_id),
                          `Reset all keystones for ${player.name}`,
                        ).then(() => setSpecsLoaded(false)),
                    )}
                >
                  {t('players.actions.specs.resetAllKeystones')}
                </Button>
              </div>
              {player.online_status === 'Online' && onlineWarning}
              {specsLoading
                ? <div className="flex justify-center py-8"><Spinner size="lg" /></div>
                : (
                    <DataTable<string, 'track' | 'xp' | 'level' | 'grant' | 'reset'>
                      aria-label={t('players.actions.specs.specsLabel')}
                      className="min-h-0 max-h-full"
                      columns={[
                        { key: 'track', label: t('players.actions.specs.columns.track'), isRowHeader: true },
                        { key: 'xp', label: t('players.actions.specs.columns.xp') },
                        { key: 'level', label: t('players.actions.specs.columns.level') },
                        { key: 'grant', label: '', sortable: false },
                        { key: 'reset', label: '', sortable: false },
                      ]}
                      rows={XP_TRACKS}
                      rowId={(t) => t}
                      initialSort={{ column: 'track', direction: 'ascending' }}
                      sortValue={(t, k) => {
                        const found = playerSpecs.find((s) => s.track_type === t)
                        if (k === 'track') return t
                        if (k === 'xp') return found?.xp ?? 0
                        if (k === 'level') return found?.level ?? 0
                        return ''
                      }}
                      renderCell={(track, key) => {
                        const found = playerSpecs.find((s) => s.track_type === track)
                        const trackKeystones = playerKeystones.filter((k) => k.track === track)
                        switch (key) {
                          case 'track':
                            return (
                              <span className="inline-flex flex-col font-semibold align-top">
                                <span>{track}</span>
                                {trackKeystones.length > 0 && (
                                  <KeystonesToggle keystones={trackKeystones} />
                                )}
                              </span>
                            )
                          case 'xp':
                            return <span className="font-mono text-muted">{(found?.xp ?? 0).toLocaleString()}</span>
                          case 'level':
                            return <span className="font-mono text-muted">{found?.level ?? 0}</span>
                          case 'grant':
                            return (
                              <Button
                                size="sm"
                                variant="ghost"
                                isDisabled={busy || player.online_status === 'Online'}
                                onPress={() =>
                                  run(
                                    () => api.players.grantMaxSpec(player.controller_id, track),
                                    `Grant max ${track} spec to ${player.name}`,
                                  ).then(() => {
                                    setPlayerSpecs((prev) => {
                                      const exists = prev.find((s) => s.track_type === track)
                                      if (exists)
                                        return prev.map((s) =>
                                          s.track_type === track ? { ...s, xp: 44182, level: 100 } : s,
                                        )
                                      return [
                                        ...prev,
                                        { player_id: player.controller_id, track_type: track, xp: 44182, level: 100 },
                                      ]
                                    })
                                  })}
                              >
                                {t('players.actions.specs.grantMax')}
                              </Button>
                            )
                          case 'reset':
                            return (
                              <Button
                                size="sm"
                                variant="danger-soft"
                                isDisabled={busy}
                                onPress={() =>
                                  gate(
                                    t('players.actions.specs.resetSpecTitle', { track }),
                                    t('players.actions.specs.resetSpecDesc', { track }),
                                    t('players.actions.specs.resetSpec'),
                                    () =>
                                      run(
                                        () => api.players.resetSpec(player.controller_id, track),
                                        `Reset ${track} spec for ${player.name}`,
                                      ).then(() =>
                                        setPlayerSpecs((prev) => prev.filter((s) => s.track_type !== track)),
                                      ),
                                  )}
                              >
                                {t('players.actions.specs.resetSpec')}
                              </Button>
                            )
                        }
                      }}
                    />
                  )}
            </div>
          )}

          {section === 'progression'
            && (() => {
              const trainerMatches = (() => {
                const re = new RegExp(`^Trainer_${selectedTrainer}\\d+(_|$)`)
                return contractCatalog.map((c) => c.alias || c.id).filter((id) => re.test(id))
              })()
              const selectedMQDef = MAIN_QUESTS.find((m) => m.id === selectedMQ)
              return (
                <div className="flex-1 overflow-y-auto flex flex-col gap-3 pr-2">
                  <Panel>
                    <SectionLabel>{t('players.actions.progression.quickPresets')}</SectionLabel>
                    <div className="text-xs text-muted">
                      {t('players.actions.progression.quickPresetsDesc')}
                    </div>
                    {!presetsLoaded
                      ? <div className="text-xs text-muted py-2">{t('players.actions.progression.loadingPresets')}</div>
                      : presets.length === 0
                        ? <div className="text-xs text-muted py-2">{t('players.actions.progression.noPresets')}</div>
                        : (
                            <div className="flex flex-col">
                              {presets.map((p) => (
                                <div
                                  key={p.id}
                                  className="flex items-center gap-3 py-2 border-b border-border/40 last:border-0"
                                >
                                  <div className="flex-1 min-w-0">
                                    <div className="text-sm font-semibold">{p.name}</div>
                                    <div className="text-xs text-muted">{p.description}</div>
                                  </div>
                                  <Chip size="sm" variant="soft">
                                    {t('players.actions.progression.nodes', { count: p.node_count })}
                                  </Chip>
                                  <Button
                                    size="sm"
                                    variant="secondary"
                                    isDisabled={busy}
                                    onPress={() =>
                                      run(
                                        () => api.progression.applyPreset(player.account_id, p.id),
                                        `Applied preset '${p.name}' to ${player.name}`,
                                      ).then(() => setNodesLoaded(false))}
                                  >
                                    {t('players.actions.progression.apply')}
                                  </Button>
                                </div>
                              ))}
                            </div>
                          )}
                  </Panel>

                  <Panel>
                    <SectionLabel>{t('players.actions.progression.progressionUnlock')}</SectionLabel>
                    <div className="text-xs text-muted">
                      {t('players.actions.progression.progressionUnlockDesc')}
                    </div>
                    <div className="flex items-center gap-2 flex-wrap">
                      <Select
                        selectedKey={unlockFaction}
                        onSelectionChange={(k) => setUnlockFaction(String(k))}
                        className="w-36"
                      >
                        <Select.Trigger>
                          <Select.Value />
                          <Select.Indicator />
                        </Select.Trigger>
                        <Select.Popover>
                          <ListBox>
                            <ListBox.Item key="atreides" id="atreides" textValue="Atreides">
                              Atreides
                              <ListBox.ItemIndicator />
                            </ListBox.Item>
                            <ListBox.Item key="harkonnen" id="harkonnen" textValue="Harkonnen">
                              Harkonnen
                              <ListBox.ItemIndicator />
                            </ListBox.Item>
                          </ListBox>
                        </Select.Popover>
                      </Select>
                      <Select
                        selectedKey={unlockPreset}
                        onSelectionChange={(k) => setUnlockPreset(String(k))}
                        className="w-48"
                      >
                        <Select.Trigger>
                          <Select.Value />
                          <Select.Indicator />
                        </Select.Trigger>
                        <Select.Popover>
                          <ListBox>
                            <ListBox.Item key="ch3_start" id="ch3_start" textValue="Ch3 Start">
                              Ch3 Start
                              <ListBox.ItemIndicator />
                            </ListBox.Item>
                            <ListBox.Item key="rank19_eligible" id="rank19_eligible" textValue="Rank 19 Eligible">
                              Rank 19 Eligible
                              <ListBox.ItemIndicator />
                            </ListBox.Item>
                          </ListBox>
                        </Select.Popover>
                      </Select>
                      <Button
                        size="sm"
                        variant="secondary"
                        isDisabled={busy}
                        onPress={() =>
                          run(
                            () => api.players.progressionUnlock(player.id, unlockFaction, unlockPreset),
                            `Applied ${unlockPreset} (${unlockFaction}) to ${player.name}`,
                          ).then(() => setNodesLoaded(false))}
                      >
                        {t('players.actions.progression.applyUnlock')}
                      </Button>
                      <Button
                        size="sm"
                        variant="danger-soft"
                        isDisabled={busy}
                        onPress={() =>
                          gate(
                            t('players.actions.progression.reverseUnlockTitle'),
                            t('players.actions.progression.reverseUnlockDesc', { preset: unlockPreset, faction: unlockFaction, player: player.name }),
                            t('players.actions.progression.reverseUnlock'),
                            () =>
                              run(
                                () => api.players.progressionReverse(player.id, unlockFaction, unlockPreset),
                                `Reversed ${unlockPreset} (${unlockFaction}) for ${player.name}`,
                              ).then(() => setNodesLoaded(false)),
                          )}
                      >
                        {t('players.actions.progression.reverseUnlock')}
                      </Button>
                    </div>
                  </Panel>

                  <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                    {contractCatalogLoaded && !contractCatalogError && (
                      <Panel>
                        <SectionLabel>{t('players.actions.progression.unlockTrainer')}</SectionLabel>
                        <div className="text-xs text-muted">
                          {t('players.actions.progression.unlockTrainerDesc')}
                        </div>
                        <div className="flex items-center gap-2">
                          <Select
                            aria-label={t('players.actions.progression.trainerLabel')}
                            selectedKey={selectedTrainer}
                            onSelectionChange={(k) => setSelectedTrainer(k as TrainerKey)}
                            className="flex-1"
                          >
                            <Select.Trigger>
                              <Select.Value />
                              <Select.Indicator />
                            </Select.Trigger>
                            <Select.Popover>
                              <ListBox>
                                {TRAINERS.map((t) => (
                                  <ListBox.Item key={t} id={t} textValue={t}>
                                    {t}
                                    <ListBox.ItemIndicator />
                                  </ListBox.Item>
                                ))}
                              </ListBox>
                            </Select.Popover>
                          </Select>
                          <Button
                            size="sm"
                            variant="secondary"
                            isDisabled={busy || trainerMatches.length === 0}
                            onPress={() =>
                              run(async () => {
                                const r = await api.players.completeContracts(player.account_id, trainerMatches)
                                await api.players.grantJobSkills(player.account_id, selectedTrainer)
                                return r
                              }, `Unlocked ${selectedTrainer} (${trainerMatches.length} contracts + skill tree) for ${player.name}`).then(
                                () => setNodesLoaded(false),
                              )}
                          >
                            Apply
                            {' '}
                            <span className="text-muted ml-1">
                              (
                              {trainerMatches.length}
                              )
                            </span>
                          </Button>
                          <Button
                            size="sm"
                            variant="danger-soft"
                            isDisabled={busy}
                            onPress={() =>
                              gate(
                                t('players.actions.progression.resetSkillTreeTitle', { trainer: selectedTrainer }),
                                t('players.actions.progression.resetSkillTreeDesc', { trainer: selectedTrainer, player: player.name }),
                                t('players.actions.progression.resetSkillTree'),
                                () =>
                                  run(
                                    () => api.players.resetJobSkills(player.account_id, selectedTrainer),
                                    `Reset ${selectedTrainer} skill tree for ${player.name}`,
                                  ),
                              )}
                          >
                            {t('players.actions.progression.resetSkillTree')}
                          </Button>
                        </div>
                      </Panel>
                    )}

                    <Panel>
                      <SectionLabel>{t('players.actions.progression.unlockMainQuest')}</SectionLabel>
                      <div className="text-xs text-muted">
                        {t('players.actions.progression.unlockMainQuestDesc')}
                      </div>
                      <div className="flex items-center gap-2">
                        <Select
                          aria-label={t('players.actions.progression.mainQuestLabel')}
                          selectedKey={selectedMQ}
                          onSelectionChange={(k) => setSelectedMQ(String(k))}
                          className="flex-1"
                        >
                          <Select.Trigger>
                            <Select.Value />
                            <Select.Indicator />
                          </Select.Trigger>
                          <Select.Popover>
                            <ListBox>
                              {MAIN_QUESTS.map((mq) => (
                                <ListBox.Item key={mq.id} id={mq.id} textValue={mq.label}>
                                  {mq.label}
                                  {mq.nodes > 0 && (
                                    <span className="text-muted ml-2">
                                      (
                                      {mq.nodes}
                                      )
                                    </span>
                                  )}
                                  <ListBox.ItemIndicator />
                                </ListBox.Item>
                              ))}
                            </ListBox>
                          </Select.Popover>
                        </Select>
                        <Button
                          size="sm"
                          variant="secondary"
                          isDisabled={busy}
                          onPress={() =>
                            run(
                              () => api.players.journeyComplete(player.account_id, selectedMQ),
                              `Unlocked ${selectedMQDef?.label ?? selectedMQ} for ${player.name}`,
                            ).then(() => setNodesLoaded(false))}
                        >
                          {t('players.actions.progression.apply')}
                        </Button>
                      </div>
                    </Panel>
                  </div>
                </div>
              )
            })()}

          {section === 'contracts' && (
            <div className="flex-1 min-h-0 overflow-hidden flex flex-col gap-3">
              <div className="flex items-center gap-2 min-h-8">
                <SectionLabel>{t('players.actions.contracts.title')}</SectionLabel>
                <div className="text-xs text-muted">
                  {contractCatalogError
                    ? (
                        <span className="text-danger">
                          {t('players.actions.contracts.loadFailed', { error: contractCatalogError })}
                        </span>
                      )
                    : contractCatalogLoaded
                      ? t('players.actions.contracts.count', { count: contractCatalog.length })
                      : t('players.actions.contracts.loadingContracts')}
                </div>
              </div>
              <div className="text-xs text-muted">
                {t('players.actions.contracts.desc')}
              </div>

              {selectedContracts.length > 0 && (
                <div className="flex flex-wrap gap-1">
                  {selectedContracts.map((id) => (
                    <Chip key={id} size="sm" variant="soft">
                      <span className="font-mono">{id}</span>
                      <button
                        type="button"
                        onClick={() => setSelectedContracts((prev) => prev.filter((x) => x !== id))}
                        className="ml-1 text-muted hover:text-foreground"
                        aria-label={`Remove ${id}`}
                      >
                        ×
                      </button>
                    </Chip>
                  ))}
                  <button
                    type="button"
                    onClick={() => setSelectedContracts([])}
                    className="text-xs underline text-muted"
                  >
                    {t('players.actions.contracts.clearAll')}
                  </button>
                </div>
              )}

              <div className="flex items-center gap-2 flex-wrap">
                <SearchField
                  aria-label={t('players.actions.contracts.filterLabel')}
                  className="flex-1 min-w-48"
                  value={contractSearch}
                  onChange={setContractSearch}
                >
                  <SearchField.Group>
                    <SearchField.SearchIcon />
                    <SearchField.Input placeholder={t('players.actions.contracts.filterPlaceholder')} />
                    <SearchField.ClearButton />
                  </SearchField.Group>
                </SearchField>
                <Button
                  size="sm"
                  variant="secondary"
                  isDisabled={busy || selectedContracts.length === 0}
                  onPress={() =>
                    run(
                      () => api.players.completeContracts(player.account_id, selectedContracts),
                      `Completed ${selectedContracts.length} contract(s) for ${player.name}`,
                    ).then(() => {
                      setSelectedContracts([])
                      setNodesLoaded(false)
                    })}
                >
                  {t('players.actions.contracts.applyContracts', { count: selectedContracts.length })}
                </Button>
                <Button
                  size="sm"
                  variant="danger-soft"
                  isDisabled={busy || selectedContracts.length === 0}
                  onPress={() =>
                    run(
                      () => api.players.reverseContracts(player.account_id, selectedContracts),
                      `Reversed ${selectedContracts.length} contract(s) for ${player.name}`,
                    ).then(() => {
                      setSelectedContracts([])
                      setNodesLoaded(false)
                    })}
                >
                  {t('players.actions.contracts.reverseContracts', { count: selectedContracts.length })}
                </Button>
              </div>

              {contractCatalogLoaded && !contractCatalogError && (
                <div className="flex-1 min-h-0 overflow-y-auto rounded border border-border bg-surface-alt">
                  {(() => {
                    const q = contractSearch.trim().toLowerCase()
                    const matches = contractCatalog.filter(
                      (c) =>
                        q === ''
                        || c.id.toLowerCase().includes(q)
                        || (c.alias && c.alias.toLowerCase().includes(q)),
                    )
                    if (matches.length === 0) {
                      return <div className="px-2 py-3 text-xs text-center text-muted">{t('players.actions.contracts.noMatching')}</div>
                    }
                    return matches.map((c) => {
                      const id = c.alias || c.id
                      const picked = selectedContracts.includes(id)
                      return (
                        <button
                          key={c.id}
                          type="button"
                          onClick={() =>
                            setSelectedContracts((prev) =>
                              picked ? prev.filter((x) => x !== id) : [...prev, id],
                            )}
                          className={
                            'flex w-full items-center justify-between px-2 py-1 text-xs font-mono hover:bg-surface '
                            + (picked ? 'bg-surface text-accent' : 'bg-transparent text-foreground')
                          }
                        >
                          <span>
                            {picked ? '✓ ' : '  '}
                            {id}
                          </span>
                          <span className="text-muted">
                            {c.tag_count === 1
                              ? t('players.actions.contracts.tagCount', { count: c.tag_count })
                              : t('players.actions.contracts.tagCountPlural', { count: c.tag_count })}
                          </span>
                        </button>
                      )
                    })
                  })()}
                </div>
              )}
            </div>
          )}

          {section === 'journey' && (
            <div className="flex-1 min-h-0 flex flex-col gap-2 overflow-y-hidden">
              <div className="flex items-center gap-2 shrink-0 min-h-8">
                <SectionLabel>{t('players.actions.journey.title')}</SectionLabel>
                <div className="flex-1" />
                <Button size="sm" variant="ghost" onPress={() => setNodesLoaded(false)} isDisabled={nodesLoading}>
                  {nodesLoading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
                </Button>
                <Button
                  size="sm"
                  variant="danger-soft"
                  isDisabled={busy}
                  onPress={() =>
                    gate(
                      t('players.actions.journey.wipeAllTitle'),
                      t('players.actions.journey.wipeAllDesc', { player: player.name }),
                      t('players.actions.journey.wipeAll'),
                      () =>
                        run(
                          () => api.players.journeyWipe(player.account_id),
                          `Wiped all journey nodes for ${player.name}`,
                        ).then(() => setNodes([])),
                    )}
                >
                  {t('players.actions.journey.wipeAll')}
                </Button>
              </div>
              <DebouncedSearchField className="shrink-0" placeholder={t('players.actions.journey.filterPlaceholder')} onSearch={setNodeSearch} />
              {nodesLoading
                ? <div className="flex justify-center py-8"><Spinner size="lg" /></div>
                : (
                    <DataTable<JourneyNode, 'node' | 'done' | 'revealed' | 'reward' | 'actions'>
                      aria-label={t('players.actions.journey.journeyLabel')}
                      className="min-h-0 max-h-full"
                      virtualized
                      rowHeight={36}
                      columns={[
                        { key: 'node', label: t('players.actions.journey.columns.nodeId'), isRowHeader: true, minWidth: 240 },
                        { key: 'done', label: t('players.actions.journey.columns.done'), width: 80 },
                        { key: 'revealed', label: t('players.actions.journey.columns.revealed'), width: 100 },
                        { key: 'reward', label: t('players.actions.journey.columns.reward'), width: 90 },
                        { key: 'actions', label: '', sortable: false, width: 220 },
                      ]}
                      rows={filteredNodes}
                      rowId={(n) => n.node_id}
                      initialSort={{ column: 'node', direction: 'ascending' }}
                      sortValue={(n, k) => {
                        if (k === 'node') return n.node_id
                        if (k === 'done') return n.is_complete ? 1 : 0
                        if (k === 'revealed') return n.is_revealed ? 1 : 0
                        if (k === 'reward') return n.has_pending_reward ? 1 : 0
                        return ''
                      }}
                      emptyState={(
                        <div className="text-center py-8 text-xs text-muted">
                          {nodes.length === 0 ? t('players.actions.journey.noNodes') : t('players.actions.journey.noMatching')}
                        </div>
                      )}
                      renderCell={(n, key) => {
                        switch (key) {
                          case 'node': return <span className="font-mono">{n.node_id}</span>
                          case 'done': return n.is_complete ? '✓' : '—'
                          case 'revealed': return n.is_revealed ? '✓' : '—'
                          case 'reward': return n.has_pending_reward ? '✓' : '—'
                          case 'actions':
                            return (
                              <div className="grid grid-cols-2 gap-1 w-full">
                                <Button
                                  size="sm"
                                  variant="ghost"
                                  isDisabled={busy}
                                  className="w-full"
                                  onPress={() =>
                                    run(
                                      () => api.players.journeyComplete(player.account_id, n.node_id),
                                      `Completed ${n.node_id}`,
                                    ).then(() => {
                                      setNodes((prev) =>
                                        prev.map((x) =>
                                          x.node_id === n.node_id || x.node_id.startsWith(n.node_id + '.')
                                            ? { ...x, is_complete: true, is_revealed: true }
                                            : x,
                                        ),
                                      )
                                    })}
                                >
                                  {n.is_complete ? t('players.actions.journey.redo') : t('players.actions.journey.complete')}
                                </Button>
                                <Button
                                  size="sm"
                                  variant="danger-soft"
                                  isDisabled={busy}
                                  className="w-full"
                                  onPress={() =>
                                    run(
                                      () => api.players.journeyReset(player.account_id, n.node_id),
                                      `Reset ${n.node_id}`,
                                    ).then(() => {
                                      setNodes((prev) =>
                                        prev.map((x) =>
                                          x.node_id === n.node_id || x.node_id.startsWith(n.node_id + '.')
                                            ? { ...x, is_complete: false, has_pending_reward: false }
                                            : x,
                                        ),
                                      )
                                    })}
                                >
                                  {t('players.actions.journey.reset')}
                                </Button>
                              </div>
                            )
                        }
                      }}
                    />
                  )}
            </div>
          )}

          {section === 'experimental' && (
            <div className="flex-1 overflow-y-auto flex flex-col gap-3 pr-2">
              <div className="text-xs px-3 py-2 rounded bg-danger/10 border border-danger/40 text-danger">
                {t('players.actions.experimental.warning')}
              </div>
              <Panel>
                <SectionLabel>{t('players.actions.experimental.knownScripts')}</SectionLabel>
                <div className="text-xs text-muted mb-2">{t('players.actions.experimental.knownScriptsDesc')}</div>
                {(
                  [
                    {
                      name: 'LeaveMeAlone',
                      label: 'Leave Me Alone',
                      desc: 'Destroys all NPCs, clears encounters + sandstorms, disables sandworm (server-wide)',
                      danger: false,
                    },
                    {
                      name: 'AwardPlayerXP',
                      label: 'Award Player XP',
                      desc: 'Awards 10,000 Combat + 10,000 Exploration + 10,000 Science XP',
                      danger: false,
                    },
                    {
                      name: 'UnlockAllSkills',
                      label: 'Unlock All Skills',
                      desc: 'Sets all skill tree modules to level 1',
                      danger: false,
                    },
                    {
                      name: 'UnlockAllAbilities',
                      label: 'Unlock All Abilities',
                      desc: 'Sets all ability modules to level 1',
                      danger: false,
                    },
                    {
                      name: 'PlaytestSetup',
                      label: 'Playtest Setup',
                      desc: '⚠ DESTRUCTIVE — ResetProgression + CleanInventory, then full gear kit',
                      danger: true,
                    },
                    {
                      name: 'PlaytestSetupAdmin',
                      label: 'Playtest Setup (Admin)',
                      desc: '⚠ DESTRUCTIVE — same as Playtest Setup but uses raw template names',
                      danger: true,
                    },
                  ] as { name: string, label: string, desc: string, danger: boolean }[]
                ).map(({ name, label, desc, danger }) => (
                  <div key={name} className="flex items-center gap-3 py-3 border-b border-border/40 last:border-b-0">
                    <div className="flex-1">
                      <div className="text-sm">{label}</div>
                      <div className="text-xs text-muted">{desc}</div>
                    </div>
                    <Button
                      size="sm"
                      variant={danger ? 'danger-soft' : 'ghost'}
                      isDisabled={busy}
                      onPress={
                        danger
                          ? () =>
                              gate(t('players.actions.experimental.runTitle', { label }), desc.replace(/^⚠ DESTRUCTIVE — /, ''), t('players.actions.experimental.confirmRun'), () =>
                                run(
                                  () => api.players.cheatScript(player.fls_id, name),
                                  `CheatScript ${name} sent for ${player.name}`,
                                ),
                              )
                          : () =>
                              run(
                                () => api.players.cheatScript(player.fls_id, name),
                                `CheatScript ${name} sent for ${player.name}`,
                              )
                      }
                    >
                      {t('players.actions.experimental.run')}
                    </Button>
                  </div>
                ))}
              </Panel>
              <Panel>
                <SectionLabel>{t('players.actions.experimental.customScript')}</SectionLabel>
                <div className="text-xs text-muted mb-2">
                  {t('players.actions.experimental.customScriptDesc')}
                </div>
                <div className="flex items-center gap-2">
                  <Input
                    placeholder={t('players.actions.experimental.customScriptPlaceholder')}
                    value={customScriptName}
                    onChange={(e) => setCustomScriptName(e.target.value)}
                    className="flex-1"
                    aria-label={t('players.actions.experimental.customScriptLabel')}
                  />
                  <Button
                    size="sm"
                    variant="ghost"
                    isDisabled={busy || !customScriptName}
                    onPress={() =>
                      run(
                        () => api.players.cheatScript(player.fls_id, customScriptName),
                        `CheatScript "${customScriptName}" sent for ${player.name}`,
                      )}
                  >
                    {t('players.actions.experimental.try')}
                  </Button>
                </div>
              </Panel>
            </div>
          )}

          {section === 'admin' && (
            <div className="flex-1 overflow-y-auto flex flex-col gap-3 pr-2">
              <Panel>
                <SectionLabel>{t('players.actions.admin.liveActions')}</SectionLabel>
                <div className="text-xs text-muted mb-2">{t('players.actions.admin.liveActionsDesc')}</div>
                {actionRow(
                  t('players.actions.admin.kickPlayer'),
                  <span className="text-xs text-muted">{t('players.actions.admin.kickDesc')}</span>,
                  t('players.actions.admin.kick'),
                  () => run(() => api.players.kick(player.fls_id), `Kick command sent for ${player.name}`),
                )}
              </Panel>

              <Panel>
                <SectionLabel>{t('players.actions.admin.destructive')}</SectionLabel>
                <div className="text-xs text-muted mb-2">{t('players.actions.admin.destructiveDesc')}</div>
                <div className="flex items-end gap-3 py-3 border-b border-border/40">
                  <div className="w-36 shrink-0 text-sm text-muted">{t('players.actions.admin.wipeInventory')}</div>
                  <div className="flex-1 text-xs text-muted">{t('players.actions.admin.wipeInventoryDesc')}</div>
                  <Button
                    size="sm"
                    variant="danger-soft"
                    isDisabled={busy}
                    onPress={() =>
                      gate(
                        t('players.actions.admin.wipeInventoryTitle'),
                        t('players.actions.admin.wipeInventoryConfirmDesc', { player: player.name }),
                        t('players.actions.admin.confirmWipe'),
                        () =>
                          run(
                            () => api.players.cleanInventory(player.fls_id),
                            `Inventory wiped for ${player.name}`,
                          ),
                      )}
                  >
                    {t('players.actions.admin.wipe')}
                  </Button>
                </div>
                <div className="flex items-end gap-3 py-3">
                  <div className="w-36 shrink-0 text-sm text-muted">{t('players.actions.admin.resetProgression')}</div>
                  <div className="flex-1 text-xs text-muted">{t('players.actions.admin.resetProgressionDesc')}</div>
                  <Button
                    size="sm"
                    variant="danger-soft"
                    isDisabled={busy}
                    onPress={() =>
                      gate(
                        t('players.actions.admin.resetProgressionTitle'),
                        t('players.actions.admin.resetProgressionConfirmDesc', { player: player.name }),
                        t('players.actions.admin.confirmReset'),
                        () =>
                          run(
                            () => api.players.resetProgression(player.fls_id),
                            `Progression reset for ${player.name}`,
                          ),
                      )}
                  >
                    {t('players.actions.admin.confirmReset')}
                  </Button>
                </div>
              </Panel>

              <Panel>
                <SectionLabel>{t('players.actions.admin.resetActions')}</SectionLabel>
                {actionRow(
                  t('players.actions.admin.deleteTutorials'),
                  <span className="text-xs text-muted">{t('players.actions.admin.deleteTutorialsDesc')}</span>,
                  t('players.actions.admin.delete'),
                  () =>
                    run(() => api.players.deleteTutorials(player.id), `Deleted tutorials for ${player.name}`),
                  true,
                  { title: t('players.actions.admin.deleteTutorialsTitle'), description: t('players.actions.admin.deleteTutorialsConfirmDesc', { player: player.name }) },
                )}
                {actionRow(
                  t('players.actions.admin.wipeCodex'),
                  <span className="text-xs text-muted">{t('players.actions.admin.wipeCodexDesc')}</span>,
                  t('players.actions.admin.wipe'),
                  () => run(() => api.players.wipeCodex(player.account_id), `Wiped codex for ${player.name}`),
                  true,
                  { title: t('players.actions.admin.wipeCodexTitle'), description: t('players.actions.admin.wipeCodexConfirmDesc', { player: player.name }) },
                )}
                {actionRow(
                  t('players.actions.admin.dismissReturning'),
                  <span className="text-xs text-muted">{t('players.actions.admin.dismissReturningDesc')}</span>,
                  t('players.actions.admin.dismiss'),
                  () =>
                    run(
                      () => api.players.dismissReturningPlayerAward(player.account_id),
                      `Dismissed returning player popup for ${player.name}`,
                    ),
                  true,
                )}
              </Panel>

              <Panel>
                <SectionLabel>{t('players.actions.admin.characterExport')}</SectionLabel>
                <div className="flex items-end gap-3 py-1">
                  <div className="flex-1 text-xs text-muted">{t('players.actions.admin.characterExportDesc')}</div>
                  <Button
                    size="sm"
                    variant="ghost"
                    isDisabled={busy}
                    onPress={() => run(() => api.players.exportPlayer(player.account_id), t('players.actions.admin.exportDownloaded'))}
                  >
                    {t('players.actions.admin.downloadExport')}
                  </Button>
                </div>
              </Panel>

              <Panel>
                <SectionLabel>{t('players.actions.admin.teleport')}</SectionLabel>
                <div className="flex items-end gap-3 py-1">
                  <Select
                    aria-label={t('players.actions.admin.teleport')}
                    placeholder={t('players.actions.admin.teleportPlaceholder')}
                    selectedKey={selectedPartition || null}
                    onSelectionChange={(k) => setSelectedPartition(k ? String(k) : '')}
                    className="flex-1"
                  >
                    <Select.Trigger>
                      <Select.Value />
                      <Select.Indicator />
                    </Select.Trigger>
                    <Select.Popover>
                      <ListBox>
                        {partitions.map((p) => (
                          <ListBox.Item key={p.name} id={p.name} textValue={p.name}>
                            {p.name}
                            <ListBox.ItemIndicator />
                          </ListBox.Item>
                        ))}
                      </ListBox>
                    </Select.Popover>
                  </Select>
                  <Button
                    size="sm"
                    variant="ghost"
                    isDisabled={busy || !selectedPartition}
                    onPress={() =>
                      run(
                        () => api.players.teleport(player.fls_id, selectedPartition),
                        `Teleported ${player.name} to ${selectedPartition}`,
                      )}
                  >
                    {t('players.actions.admin.move')}
                  </Button>
                </div>
                <span className="text-xs text-muted">{t('players.actions.admin.teleportNote')}</span>
              </Panel>

              <Panel>
                <SectionLabel>{t('players.actions.admin.teleportToPlayer')}</SectionLabel>
                <div className="text-xs text-muted mb-2">
                  Drop
                  {' '}
                  {player.name}
                  {' '}
                  exactly on another character&apos;s current position.
                </div>
                <div className="flex flex-col gap-2">
                  <SearchField value={targetSearch} onChange={setTargetSearch} className="w-full">
                    <SearchField.Group>
                      <SearchField.SearchIcon />
                      <SearchField.Input placeholder={t('players.searchPlaceholder')} aria-label={t('players.searchLabel')} />
                      <SearchField.ClearButton />
                    </SearchField.Group>
                  </SearchField>
                  <div className="flex items-end gap-3">
                    <Select
                      aria-label={t('players.actions.admin.selectTargetLabel')}
                      placeholder={allPlayers.length === 0 ? t('players.actions.admin.loadingPlayers') : t('players.actions.admin.pickTarget')}
                      selectedKey={selectedTeleportTarget != null ? String(selectedTeleportTarget) : null}
                      onSelectionChange={(k) => setSelectedTeleportTarget(k ? Number(k) : null)}
                      className="flex-1"
                    >
                      <Select.Trigger>
                        <Select.Value />
                        <Select.Indicator />
                      </Select.Trigger>
                      <Select.Popover>
                        <ListBox>
                          {allPlayers
                            .filter((p) => !targetSearch || p.name.toLowerCase().includes(targetSearch.toLowerCase()))
                            .slice(0, 100)
                            .map((p) => (
                              <ListBox.Item key={p.id} id={String(p.id)} textValue={p.name}>
                                <div className="flex items-center justify-between gap-2 w-full">
                                  <span>{p.name}</span>
                                  <span className="text-xs text-muted">
                                    {p.map || '—'}
                                    {' · '}
                                    {p.online_status}
                                  </span>
                                </div>
                                <ListBox.ItemIndicator />
                              </ListBox.Item>
                            ))}
                        </ListBox>
                      </Select.Popover>
                    </Select>
                    <Button
                      size="sm"
                      variant="ghost"
                      isDisabled={busy || selectedTeleportTarget == null}
                      onPress={() => {
                        const target = allPlayers.find((p) => p.id === selectedTeleportTarget)
                        if (!target) return
                        run(
                          () => api.players.teleportToPlayer(player.fls_id, target.id),
                          `Teleported ${player.name} to ${target.name}`,
                        )
                      }}
                    >
                      {t('players.actions.admin.move')}
                    </Button>
                  </div>
                </div>
              </Panel>

              <Panel>
                <SectionLabel>{t('players.actions.admin.whisper')}</SectionLabel>
                <div className="text-xs text-muted mb-2">
                  Send a private chat message to
                  {' '}
                  {player.name}
                  .
                  {' '}
                  <span className="text-warning">Experimental</span>
                </div>
                <div className="flex flex-col gap-2">
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-muted shrink-0">{t('players.actions.admin.whisperFrom')}</span>
                    <input
                      type="text"
                      value={whisperSenderName}
                      onChange={(e) => setWhisperSenderName(e.target.value)}
                      placeholder="GM"
                      maxLength={32}
                      className="w-32 bg-surface border border-border rounded px-2 py-1 text-xs text-foreground focus:outline-none focus:border-accent/60"
                    />
                  </div>
                  <textarea
                    value={whisperText}
                    onChange={(e) => setWhisperText(e.target.value)}
                    placeholder={`Message to ${player.name}…`}
                    rows={2}
                    maxLength={500}
                    className="w-full bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground focus:outline-none focus:border-accent/60 resize-y"
                  />
                  <div className="flex items-center justify-end gap-2">
                    <span className="text-xs text-muted">
                      {whisperText.length}
                      {' '}
                      / 500
                    </span>
                    <Button
                      size="sm"
                      variant="ghost"
                      isDisabled={busy || !whisperText.trim()}
                      onPress={() =>
                        run(
                          () =>
                            api.chat.whisper(
                              player.fls_id,
                              player.name,
                              whisperSenderName.trim() || 'GM',
                              whisperText.trim(),
                            ),
                          t('players.actions.admin.whisperSent', { player: player.name }),
                        ).then(() => setWhisperText(''))}
                    >
                      Send
                    </Button>
                  </div>
                </div>
              </Panel>

              <Panel>
                <SectionLabel>{t('players.actions.admin.spawnVehicle')}</SectionLabel>
                <div className="text-xs text-muted mb-2">{t('players.actions.admin.spawnVehicleDesc')}</div>
                <div className="flex flex-col gap-2">
                  <div className="flex items-center gap-2">
                    <Select
                      aria-label={t('players.actions.admin.vehicleLabel')}
                      placeholder={t('players.actions.admin.selectVehicle')}
                      selectedKey={spawnVehicleId || null}
                      onSelectionChange={(k) => {
                        const id = k ? String(k) : ''
                        setSpawnVehicleId(id)
                        const v = (allVehicles as { id: string, templates: string[] }[]).find((x) => x.id === id)
                        setSpawnVehicleTemplate(v?.templates[0] ?? '')
                      }}
                      className="flex-1"
                    >
                      <Select.Trigger>
                        <Select.Value />
                        <Select.Indicator />
                      </Select.Trigger>
                      <Select.Popover>
                        <ListBox>
                          {(allVehicles as { id: string, label: string }[]).map((v) => (
                            <ListBox.Item key={v.id} id={v.id} textValue={v.label}>
                              {v.label}
                              <ListBox.ItemIndicator />
                            </ListBox.Item>
                          ))}
                        </ListBox>
                      </Select.Popover>
                    </Select>
                    {spawnVehicleId
                      && (() => {
                        const templates
                          = (allVehicles as { id: string, templates: string[] }[]).find(
                            (v) => v.id === spawnVehicleId,
                          )?.templates ?? []
                        return templates.length > 1
                          ? (
                              <Select
                                aria-label={t('players.actions.admin.templateLabel')}
                                selectedKey={spawnVehicleTemplate || null}
                                onSelectionChange={(k) => setSpawnVehicleTemplate(k ? String(k) : '')}
                                className="w-44"
                              >
                                <Select.Trigger>
                                  <Select.Value />
                                  <Select.Indicator />
                                </Select.Trigger>
                                <Select.Popover>
                                  <ListBox>
                                    {templates.map((t) => (
                                      <ListBox.Item key={t} id={t} textValue={t}>
                                        {t}
                                        <ListBox.ItemIndicator />
                                      </ListBox.Item>
                                    ))}
                                  </ListBox>
                                </Select.Popover>
                              </Select>
                            )
                          : null
                      })()}
                  </div>
                  <div className="flex items-center gap-2">
                    <Select
                      aria-label={t('players.actions.admin.spawnLocationLabel')}
                      placeholder={t('players.actions.admin.selectSpawnLocation')}
                      selectedKey={spawnVehiclePartition || null}
                      onSelectionChange={(k) => setSpawnVehiclePartition(k ? String(k) : '')}
                      className="flex-1"
                    >
                      <Select.Trigger>
                        <Select.Value />
                        <Select.Indicator />
                      </Select.Trigger>
                      <Select.Popover>
                        <ListBox>
                          {partitions.map((p) => (
                            <ListBox.Item key={p.name} id={p.name} textValue={p.name}>
                              {p.name}
                              <ListBox.ItemIndicator />
                            </ListBox.Item>
                          ))}
                        </ListBox>
                      </Select.Popover>
                    </Select>
                    <label className="flex items-center gap-1.5 cursor-pointer select-none">
                      <input
                        type="checkbox"
                        checked={spawnVehiclePersistent}
                        onChange={(e) => setSpawnVehiclePersistent(e.target.checked)}
                      />
                      <span className="text-xs">{t('players.actions.admin.persistent')}</span>
                    </label>
                    <Button
                      size="sm"
                      variant="ghost"
                      isDisabled={busy || !spawnVehicleId || !spawnVehiclePartition}
                      onPress={() => {
                        const v = (allVehicles as { id: string, actor_class: string }[]).find(
                          (x) => x.id === spawnVehicleId,
                        )
                        const p = partitions.find((x) => x.name === spawnVehiclePartition)
                        if (!v || !p) return
                        run(
                          () =>
                            api.players.spawnVehicle(player.fls_id, v.actor_class, p.x, p.y, p.z, {
                              template_name: spawnVehicleTemplate || undefined,
                              persistent: spawnVehiclePersistent,
                            }),
                          `Spawn ${spawnVehicleId} command sent for ${player.name}`,
                        )
                      }}
                    >
                      {t('players.actions.admin.spawn')}
                    </Button>
                  </div>
                </div>
              </Panel>
            </div>
          )}

          {section === 'tags' && (
            <div className="flex-1 min-h-0 flex flex-col gap-3 overflow-hidden">
              <div className="shrink-0 flex flex-col gap-2">
                <SectionLabel>{t('players.actions.tags.addTags')}</SectionLabel>
                <AddTagsPanel tags={tags} pendingTags={pendingTags} onAdd={handleAddTag} />
                {pendingTags.length > 0 && (
                  <>
                    <div className="flex flex-col gap-1 mt-1">
                      {pendingTags.map((tag) => (
                        <div
                          key={tag}
                          className="flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius)] text-xs bg-surface border border-border"
                        >
                          <span className="flex-1 font-mono">{tag}</span>
                          <Button
                            size="sm"
                            variant="danger-soft"
                            onPress={() => setPendingTags((prev) => prev.filter((t) => t !== tag))}
                            aria-label={`Unstage ${tag}`}
                          >
                            ✕
                          </Button>
                        </div>
                      ))}
                    </div>
                    <Button
                      size="sm"
                      onPress={() => {
                        const toAdd = pendingTags
                        run(
                          () => api.players.updateTags(player.account_id, toAdd, []),
                          `Added ${toAdd.length} tag${toAdd.length > 1 ? 's' : ''}`,
                        ).then(() => {
                          setTags((prev) => [...new Set([...prev, ...toAdd])].sort())
                          setPendingTags([])
                        })
                      }}
                    >
                      {t('players.actions.tags.addCount', { count: pendingTags.length })}
                    </Button>
                  </>
                )}
              </div>

              {tagsLoading
                ? <div className="flex justify-center py-8"><Spinner size="lg" /></div>
                : (
                    <div className="flex-1 min-h-0 flex flex-col gap-2 overflow-hidden">
                      <div className="flex items-center gap-2 shrink-0 min-h-8">
                        <SectionLabel>
                          {t('players.actions.tags.activeTags', { count: tags.length })}
                        </SectionLabel>
                        <DebouncedSearchField className="flex-1" placeholder={t('players.actions.tags.filterPlaceholder')} onSearch={setTagRemoveSearch} />
                      </div>
                      <DataTable<string, 'tag' | 'actions'>
                        aria-label={t('players.actions.tags.activeTagsLabel')}
                        className="min-h-0 max-h-full"
                        columns={[
                          { key: 'tag', label: t('players.actions.tags.tagColumn'), isRowHeader: true },
                          { key: 'actions', label: '', sortable: false },
                        ]}
                        rows={filteredActiveTags}
                        rowId={(t) => t}
                        initialSort={{ column: 'tag', direction: 'ascending' }}
                        sortValue={(t) => t}
                        emptyState={<div className="py-6 text-center text-muted">{t('players.actions.tags.noTags')}</div>}
                        renderCell={(tag, key) => {
                          if (key === 'tag') return <span className="font-mono">{tag}</span>
                          return (
                            <Button
                              size="sm"
                              variant="danger-soft"
                              onPress={() => {
                                setTags((prev) => prev.filter((t) => t !== tag))
                                run(() => api.players.updateTags(player.account_id, [], [tag]), t('players.actions.tags.removedTag'))
                              }}
                              aria-label={`Remove ${tag}`}
                            >
                              ✕
                            </Button>
                          )
                        }}
                      />
                    </div>
                  )}
            </div>
          )}

          {section === 'history' && (
            <div className="flex-1 overflow-y-auto flex flex-col gap-3 pr-2">
              {historyLoading
                ? <div className="flex justify-center py-8"><Spinner size="lg" /></div>
                : (
                    <>
                      <Panel>
                        <SectionLabel>{t('players.actions.history.gameEvents')}</SectionLabel>
                        <DataTable<GameEvent, 'time' | 'map' | 'event_type' | 'location'>
                          aria-label={t('players.actions.history.gameEventsLabel')}
                          className="max-h-[40vh]"
                          columns={[
                            { key: 'time', label: t('players.actions.history.columns.time'), isRowHeader: true },
                            { key: 'map', label: t('players.actions.history.columns.map') },
                            { key: 'event_type', label: t('players.actions.history.columns.eventType') },
                            { key: 'location', label: t('players.actions.history.columns.location'), sortable: false },
                          ]}
                          rows={events}
                          rowId={(evt) => `${evt.actor_id}-${evt.universe_time}`}
                          initialSort={{ column: 'time', direction: 'descending' }}
                          sortValue={(evt, k) => {
                            if (k === 'time') return evt.universe_time
                            if (k === 'map') return evt.map
                            if (k === 'event_type') return evt.event_type
                            return ''
                          }}
                          emptyState={<div className="py-6 text-center text-muted">{t('players.actions.history.noEvents')}</div>}
                          renderCell={(evt, key) => {
                            switch (key) {
                              case 'time': return <span className="font-mono text-muted">{evt.universe_time}</span>
                              case 'map': return <span className="text-muted">{evt.map}</span>
                              case 'event_type':
                                return (
                                  <Chip size="sm" color={eventColor(evt.event_type)} variant="soft">
                                    {evt.event_type}
                                  </Chip>
                                )
                              case 'location':
                                return (
                                  <span className="font-mono text-muted">
                                    {Math.round(evt.x)}
                                    ,
                                    {Math.round(evt.y)}
                                    ,
                                    {Math.round(evt.z)}
                                  </span>
                                )
                            }
                          }}
                        />
                      </Panel>

                      <Panel>
                        <SectionLabel>{t('players.actions.history.dungeonRecords')}</SectionLabel>
                        <DataTable<DungeonRecord, 'dungeon' | 'difficulty' | 'duration' | 'party'>
                          aria-label={t('players.actions.history.dungeonLabel')}
                          className="max-h-[40vh]"
                          columns={[
                            { key: 'dungeon', label: t('players.actions.history.columns.dungeon'), isRowHeader: true },
                            { key: 'difficulty', label: t('players.actions.history.columns.difficulty') },
                            { key: 'duration', label: t('players.actions.history.columns.duration') },
                            { key: 'party', label: t('players.actions.history.columns.partySize') },
                          ]}
                          rows={dungeons}
                          rowId={(d) => String(d.completion_id)}
                          initialSort={{ column: 'dungeon', direction: 'ascending' }}
                          sortValue={(d, k) => {
                            if (k === 'dungeon') return d.dungeon_id
                            if (k === 'difficulty') return d.difficulty
                            if (k === 'duration') return d.duration_ms
                            return d.players_num
                          }}
                          emptyState={<div className="py-6 text-center text-muted">{t('players.actions.history.noDungeons')}</div>}
                          renderCell={(d, key) => {
                            switch (key) {
                              case 'dungeon': return <span className="font-semibold">{d.dungeon_id}</span>
                              case 'difficulty':
                                return (
                                  <Chip size="sm" color={difficultyColor(d.difficulty)} variant="soft">
                                    {d.difficulty}
                                  </Chip>
                                )
                              case 'duration':
                                return <span className="font-mono text-muted">{formatDuration(d.duration_ms)}</span>
                              case 'party': return <span className="text-muted">{d.players_num}</span>
                            }
                          }}
                        />
                      </Panel>
                    </>
                  )}
            </div>
          )}
        </div>
      </div>

      <ConfirmDialog
        open={confirmPending !== null}
        title={confirmPending?.title ?? ''}
        description={confirmPending?.description ?? ''}
        confirmLabel={confirmPending?.confirmLabel}
        onConfirm={() => {
          const action = confirmPending?.onConfirm
          setConfirmPending(null)
          action?.()
        }}
        onCancel={() => setConfirmPending(null)}
      />
    </>
  )
}

type ChipColor = 'default' | 'accent' | 'success' | 'warning' | 'danger'

function eventColor(eventType: number): ChipColor {
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
