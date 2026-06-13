import * as React from 'react'
import {
  Button, Chip, CloseButton, ListBox, Modal, SearchField, Select, Separator, Switch, TextArea, TextField, toast,
} from '@heroui/react'
import type { Selection } from '@heroui/react'
import type { DataGridColumn } from '@heroui-pro/react'
import { DataGrid, Segment } from '@heroui-pro/react'
import { useAtom } from 'jotai'
import { useTranslation } from 'react-i18next'
import { ActionBar, FieldInput, FieldSelect, Icon, NumberInput, SectionLabel } from '../../../dune-ui'
import { gameplayTagsSyncAtom } from '../../../data/store'
import { api } from '../../../api/client'
import type { EventDefinition, GivePack, Player } from '../../../api/client'
import type { MilestoneFields, MilestoneSignal, RewardXP, XPType, ZoneRaceFields, KeyedRewardItem } from '../types'
import { XP_TRACKS } from '../types'
import { ManagePacksModal } from '../../PlayersTab/modals/ManagePacksModal'
import { CategorizedPackPicker } from '../../../components/CategorizedPackPicker'
import type { EventEditorModalProps } from './types'

const FormSection: React.FC<{ children: React.ReactNode, className?: string }> = ({ children, className }) => (
  <div className={`flex flex-col gap-3 rounded-[var(--radius)] border border-border bg-surface-secondary p-4 dune-lift ${className ?? ''}`}>
    {children}
  </div>
)

type TagPickerFieldProps = {
  value: string
  onSelect: (tag: string) => void
  options: string[]
  ariaLabel: string
}

const TagPickerField: React.FC<TagPickerFieldProps> = ({ value, onSelect, options, ariaLabel }) => {
  const [query, setQuery] = React.useState('')

  const filtered = React.useMemo(() => {
    if (!query) return []
    const q = query.toLowerCase()
    return options.filter((t) => t.toLowerCase().includes(q)).slice(0, 100)
  }, [options, query])

  const handleSelect = (tag: string) => {
    onSelect(tag)
    setQuery('')
  }

  return (
    <>
      {value && (
        <div className="mb-1 flex items-center gap-1">
          <span className="font-mono text-xs text-foreground">{value}</span>
          <button
            type="button"
            className="text-xs text-muted hover:text-foreground ml-1"
            onClick={() => {
              onSelect('')
              setQuery('')
            }}
          >
            ×
          </button>
        </div>
      )}
      {/* Inline, absolutely-positioned dropdown anchored to the search field —
          mirrors the reward-template picker below. A portal to document.body
          lands outside the React Aria modal subtree, where the underlay blocks
          pointer selection and scroll chains to the whole dialog. The dropdown
          carries the `tag-dropdown` marker so the containing FormSection can
          raise its stacking context (each .dune-lift is isolation:isolate, so a
          plain z-index can't paint over the next sibling panel). */}
      <div className="relative w-full">
        <SearchField value={query} onChange={setQuery} aria-label={ariaLabel} className="w-full">
          <SearchField.Group>
            <SearchField.SearchIcon />
            <SearchField.Input placeholder="Search gameplay tags…" />
            <SearchField.ClearButton />
          </SearchField.Group>
        </SearchField>
        {filtered.length > 0 && (
          <div className="tag-dropdown absolute z-[200] w-full mt-1 max-h-52 overflow-y-auto overscroll-contain rounded-[var(--radius)] border border-border bg-surface shadow-lg">
            {filtered.map((tag) => (
              <div
                key={tag}
                className="px-3 py-1.5 text-xs font-mono cursor-pointer hover:bg-surface-hover"
                onClick={() => handleSelect(tag)}
              >
                {tag}
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  )
}

const parseZoneConfig = (raw: string): ZoneRaceFields => {
  try {
    const c = JSON.parse(raw || '{}') as Record<string, unknown>
    return {
      map: typeof c.map === 'string' ? c.map : '',
      x: typeof c.x === 'number' ? c.x : 0,
      y: typeof c.y === 'number' ? c.y : 0,
      z: typeof c.z === 'number' ? c.z : 0,
      radius: typeof c.radius === 'number' ? c.radius : 500,
      participants: Array.isArray(c.participants) ? (c.participants as number[]) : [],
    }
  }
  catch {
    return { map: '', x: 0, y: 0, z: 0, radius: 500, participants: [] }
  }
}

const parseMilestoneConfig = (raw: string): MilestoneFields => {
  try {
    const c = JSON.parse(raw || '{}') as Record<string, unknown>
    return {
      signal: c.signal === 'achievement_tag' ? 'achievement_tag' : 'level',
      threshold: typeof c.threshold === 'number' ? c.threshold : 50,
      tagName: typeof c.tag_name === 'string' ? c.tag_name : '',
      awardPast: !!c.award_past,
    }
  }
  catch {
    return { signal: 'level', threshold: 50, tagName: '', awardPast: false }
  }
}

const serializeConfig = (
  type: EventDefinition['type'],
  zone: ZoneRaceFields,
  ms: MilestoneFields,
): string => {
  if (type === 'zone_race') {
    return JSON.stringify({
      map: zone.map, x: zone.x, y: zone.y, z: zone.z,
      radius: zone.radius, participants: zone.participants,
    })
  }
  const cfg: Record<string, unknown> = { signal: ms.signal, award_past: ms.awardPast }
  if (ms.signal === 'level') cfg.threshold = ms.threshold
  else cfg.tag_name = ms.tagName
  return JSON.stringify(cfg)
}

const serializeReward = (
  currency: number,
  factionScrip: number,
  items: KeyedRewardItem[],
  xpRewards: RewardXP[],
): string => {
  const r: Record<string, unknown> = {}
  if (currency > 0) r.currency = currency
  if (factionScrip > 0) r.faction_scrip = factionScrip
  if (items.length > 0) r.items = items.map(({ template, qty, quality }) => ({ template, qty, quality }))
  if (xpRewards.length > 0) r.xp = xpRewards
  return Object.keys(r).length === 0 ? '' : JSON.stringify(r)
}

export const EventEditorModal: React.FC<EventEditorModalProps> = ({
  isOpen, onClose, editing, onSaved,
}) => {
  const { t } = useTranslation()
  const isEdit = editing !== null

  const [activeSegment, setActiveSegment] = React.useState<'config' | 'rewards'>('config')

  // Config fields
  const [name, setName] = React.useState('')
  const [type, setType] = React.useState<EventDefinition['type']>('milestone')
  const [zone, setZone] = React.useState<ZoneRaceFields>(
    { map: '', x: 0, y: 0, z: 0, radius: 500, participants: [] },
  )
  const [players, setPlayers] = React.useState<Player[]>([])
  const [participantPickKey, setParticipantPickKey] = React.useState(0)
  const [milestone, setMilestone] = React.useState<MilestoneFields>(
    { signal: 'level', threshold: 50, tagName: '', awardPast: false },
  )
  const [allGameplayTags] = useAtom(gameplayTagsSyncAtom)
  const MILESTONE_DEFAULT_TEMPLATE = '{player} reached level {value} in {event}!'
  const ZONE_RACE_DEFAULT_TEMPLATE = '{player} completed {event}!'
  const [announceTemplate, setAnnounceTemplate] = React.useState(MILESTONE_DEFAULT_TEMPLATE)
  const [pollSeconds, setPollSeconds] = React.useState(7)
  const [jitterSeconds, setJitterSeconds] = React.useState(3)
  const [maps, setMaps] = React.useState<string[]>([])

  // Reward fields
  const [rewardCurrency, setRewardCurrency] = React.useState(0)
  const [rewardFactionScrip, setRewardFactionScrip] = React.useState(0)
  const [rewardItems, setRewardItems] = React.useState<KeyedRewardItem[]>([])
  const [rewardItemKeys, setRewardItemKeys] = React.useState<Selection>(new Set())
  const [rewardXP, setRewardXP] = React.useState<RewardXP[]>([])

  // Item picker state
  const [templates, setTemplates] = React.useState<{ id: string, name: string }[]>([])
  const [packs, setPacks] = React.useState<GivePack[]>([])
  const [managePacksOpen, setManagePacksOpen] = React.useState(false)
  const [templateQuery, setTemplateQuery] = React.useState('')
  const [selectedTemplate, setSelectedTemplate] = React.useState('')
  const [itemQty, setItemQty] = React.useState(1)
  const [itemQuality, setItemQuality] = React.useState(0)

  // XP picker state
  const [xpType, setXpType] = React.useState<XPType>('specialization')
  const [xpSpecTrack, setXpSpecTrack] = React.useState<string>(XP_TRACKS[0])
  const [xpAmount, setXpAmount] = React.useState(0)

  const [saving, setSaving] = React.useState(false)

  const keyCounter = React.useRef(0)
  const nextKey = () => String(keyCounter.current++)

  React.useEffect(() => {
    if (!isOpen) return
    Promise.resolve().then(() => {
      setActiveSegment('config')
      if (editing) {
        setName(editing.name)
        setType(editing.type)
        if (editing.type === 'zone_race') {
          setZone(parseZoneConfig(editing.config))
          setMilestone({ signal: 'level', threshold: 50, tagName: '', awardPast: false })
        }
        else {
          setMilestone(parseMilestoneConfig(editing.config))
          setZone({ map: '', x: 0, y: 0, z: 0, radius: 500, participants: [] })
        }
        try {
          const r = JSON.parse(editing.reward || '{}') as Record<string, unknown>
          setRewardCurrency(typeof r.currency === 'number' ? r.currency : 0)
          setRewardFactionScrip(typeof r.faction_scrip === 'number' ? r.faction_scrip : 0)
          type RawItem = { template: string, qty: number, quality: number }
          const rawItems: RawItem[] = Array.isArray(r.items) ? r.items as RawItem[] : []
          setRewardItems(rawItems.map((item) => ({ ...item, _key: nextKey() })))
          setRewardXP(Array.isArray(r.xp) ? r.xp as RewardXP[] : [])
        }
        catch {
          setRewardCurrency(0)
          setRewardFactionScrip(0)
          setRewardItems([])
          setRewardXP([])
        }
        setAnnounceTemplate(editing.announce_template || (editing.type === 'zone_race' ? ZONE_RACE_DEFAULT_TEMPLATE : MILESTONE_DEFAULT_TEMPLATE))
        setPollSeconds(editing.poll_seconds > 0 ? editing.poll_seconds : 7)
        setJitterSeconds(editing.jitter_seconds > 0 ? editing.jitter_seconds : 3)
      }
      else {
        setName('')
        setType('milestone')
        setZone({ map: '', x: 0, y: 0, z: 0, radius: 500, participants: [] })
        setMilestone({ signal: 'level', threshold: 50, tagName: '', awardPast: false })
        setRewardCurrency(0)
        setRewardFactionScrip(0)
        setRewardItems([])
        setRewardXP([])
        setAnnounceTemplate(MILESTONE_DEFAULT_TEMPLATE)
        setPollSeconds(7)
        setJitterSeconds(3)
      }
      setTemplateQuery('')
      setSelectedTemplate('')
      setItemQty(1)
      setItemQuality(0)
      setXpType('specialization')
      setXpSpecTrack(XP_TRACKS[0])
      setXpAmount(0)
      setParticipantPickKey((k) => k + 1)
      setRewardItemKeys(new Set())
    })
    api.maps.list().then(setMaps).catch(() => {})
    api.players.templates().then(setTemplates).catch(() => {})
    api.players.list().then(setPlayers).catch(() => {})
    api.givePacks.config().then((cfg) => setPacks(cfg.packs ?? [])).catch(() => {})
  }, [isOpen, editing])

  const filteredTemplates = React.useMemo(() => {
    if (!templateQuery) return []
    const q = templateQuery.toLowerCase()
    return templates
      .filter((tpl) => tpl.id.toLowerCase().includes(q) || tpl.name.toLowerCase().includes(q))
      .slice(0, 100)
  }, [templates, templateQuery])

  const nameMap = React.useMemo(
    () => new Map(templates.map((tpl) => [tpl.id, tpl.name])),
    [templates],
  )

  const handleTypeChange = (newType: string) => {
    const t2 = newType as EventDefinition['type']
    setType(t2)
    if (t2 === 'zone_race') {
      setZone({ map: '', x: 0, y: 0, z: 0, radius: 500, participants: [] })
      if (!announceTemplate || announceTemplate === MILESTONE_DEFAULT_TEMPLATE)
        setAnnounceTemplate(ZONE_RACE_DEFAULT_TEMPLATE)
    }
    else {
      setMilestone({ signal: 'level', threshold: 50, tagName: '', awardPast: false })
      if (!announceTemplate || announceTemplate === ZONE_RACE_DEFAULT_TEMPLATE)
        setAnnounceTemplate(MILESTONE_DEFAULT_TEMPLATE)
    }
  }

  const addParticipant = (accountId: number) => {
    if (zone.participants.includes(accountId)) return
    setZone((z) => ({ ...z, participants: [...z.participants, accountId] }))
    setParticipantPickKey((k) => k + 1)
  }

  const playerName = (accountId: number) =>
    players.find((p) => p.account_id === accountId)?.name ?? String(accountId)

  const availablePlayers = players.filter((p) => !zone.participants.includes(p.account_id))

  const pickTemplate = (tpl: { id: string, name: string }) => {
    setSelectedTemplate(tpl.id)
    setTemplateQuery(tpl.name ? `${tpl.id}  —  ${tpl.name}` : tpl.id)
  }

  const addRewardItem = () => {
    if (!selectedTemplate) return
    setRewardItems((prev) => [
      ...prev,
      { template: selectedTemplate, qty: itemQty, quality: itemQuality, _key: nextKey() },
    ])
    setTemplateQuery('')
    setSelectedTemplate('')
    setItemQty(1)
    setItemQuality(0)
  }

  const removeRewardItem = (key: string) => {
    setRewardItems((prev) => prev.filter((it) => it._key !== key))
    setRewardItemKeys((prev) => {
      if (prev === 'all') return new Set(rewardItems.filter((it) => it._key !== key).map((it) => it._key))
      const next = new Set(prev as Set<string>)
      next.delete(key)
      return next
    })
  }

  const updateRewardItem = (key: string, field: 'qty' | 'quality', value: number) => {
    setRewardItems((prev) => prev.map((it) => it._key === key ? { ...it, [field]: value } : it))
  }

  const rewardSelectionCount = rewardItemKeys === 'all'
    ? rewardItems.length
    : (rewardItemKeys as Set<string>).size

  const handleBulkDeleteRewardItems = () => {
    if (rewardItemKeys === 'all') {
      setRewardItems([])
    }
    else {
      const keys = rewardItemKeys as Set<string>
      setRewardItems((prev) => prev.filter((it) => !keys.has(it._key)))
    }
    setRewardItemKeys(new Set())
  }

  const addXP = () => {
    if (xpAmount <= 0) return
    const track = xpType === 'character' ? 'character' : xpSpecTrack
    setRewardXP((prev) => [...prev, { track, amount: xpAmount }])
    setXpAmount(0)
  }

  const handleSave = () => {
    if (!name.trim()) {
      toast.danger(t('events.editor.nameRequired'))
      return
    }
    setSaving(true)
    const payload = {
      name: name.trim(),
      type,
      config: serializeConfig(type, zone, milestone),
      reward: serializeReward(rewardCurrency, rewardFactionScrip, rewardItems, rewardXP),
      announce_channel_id: '',
      announce_template: announceTemplate,
      poll_seconds: pollSeconds,
      jitter_seconds: jitterSeconds,
    }
    const call = isEdit && editing
      ? api.events.update(editing.id, payload)
      : api.events.create(payload)

    call
      .then(() => {
        toast.success(t(isEdit ? 'events.editor.updated' : 'events.editor.created'))
        onSaved()
        onClose()
      })
      .catch((e: unknown) => {
        toast.danger(t('events.editor.saveFailed', { message: e instanceof Error ? e.message : String(e) }))
      })
      .finally(() => setSaving(false))
  }

  const fieldLabelClass = 'text-xs text-muted mb-1 block'

  const rewardItemColumns: DataGridColumn<KeyedRewardItem>[] = [
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
          onChange={(v) => updateRewardItem(item._key, 'qty', v)}
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
          onChange={(v) => updateRewardItem(item._key, 'quality', v)}
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
          onPress={() => removeRewardItem(item._key)}
          aria-label={t('common.remove')}
        >
          <Icon name="trash" />
        </Button>
      ),
    },
  ]

  return (
    <>
      {/* Hidden (not stacked) while Manage Packs is open — stacked sibling
          modals fight over the React Aria overlay and the top one goes inert.
          Component stays mounted, so unsaved edits survive the swap. */}
      <Modal.Backdrop
        variant="blur"
        className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent"
        isOpen={isOpen && !managePacksOpen}
        onOpenChange={(open) => { if (!open) onClose() }}
      >
        <Modal.Container size="cover" scroll="outside">
          <Modal.Dialog className="p-10 dialog-surface-alt">
            <Modal.CloseTrigger />
            <Modal.Header className="flex items-center gap-4 shrink-0">
              <Modal.Heading className="text-accent">
                {isEdit ? t('events.editor.editTitle') : t('events.editor.createTitle')}
              </Modal.Heading>
              <Segment
                className="ml-auto"
                selectedKey={activeSegment}
                onSelectionChange={(k) => setActiveSegment(k as 'config' | 'rewards')}
                size="sm"
                aria-label={t('events.editor.createTitle')}
              >
                <Segment.Item id="config">
                  <Segment.Separator />
                  {t('events.editor.config')}
                </Segment.Item>
                <Segment.Item id="rewards">
                  <Segment.Separator />
                  {t('events.editor.rewards')}
                </Segment.Item>
              </Segment>
            </Modal.Header>

            <Modal.Body className="flex flex-col h-[80vh] min-h-0">

              {/* Config panel — always mounted */}
              <div
                className="flex flex-col gap-4 overflow-y-auto flex-1 min-h-0"
                style={{ display: activeSegment === 'config' ? undefined : 'none' }}
              >
                <FormSection>
                  <SectionLabel>{t('events.editor.basics')}</SectionLabel>
                  <div className="flex gap-4 mt-2">
                    <div className="flex-1">
                      <span className={fieldLabelClass}>{t('events.editor.name')}</span>
                      <FieldInput
                        value={name}
                        onChange={setName}
                        placeholder={t('events.editor.namePlaceholder')}
                        ariaLabel={t('events.editor.name')}
                      />
                    </div>
                    <div className="w-44 shrink-0">
                      <span className={fieldLabelClass}>{t('events.editor.type')}</span>
                      <FieldSelect
                        value={type}
                        onChange={handleTypeChange}
                        options={['milestone', 'zone_race']}
                        isDisabled={isEdit}
                        ariaLabel={t('events.editor.type')}
                      />
                    </div>
                  </div>
                </FormSection>

                {type === 'zone_race' && (
                  <FormSection>
                    <SectionLabel>{t('events.editor.zoneConfig')}</SectionLabel>
                    <div className="flex flex-col gap-3 mt-2">
                      <div>
                        <span className={fieldLabelClass}>{t('events.editor.zoneMap')}</span>
                        <Select
                          selectedKey={zone.map || null}
                          onSelectionChange={(k) => setZone((z) => ({ ...z, map: String(k) }))}
                          aria-label={t('events.editor.zoneMap')}
                          className="w-full"
                          isDisabled={maps.length === 0}
                        >
                          <Select.Trigger>
                            <Select.Value>
                              {zone.map
                                ? <span className="font-mono">{zone.map}</span>
                                : <span className="text-muted">{t('events.editor.zoneMapPlaceholder')}</span>}
                            </Select.Value>
                            <Select.Indicator />
                          </Select.Trigger>
                          <Select.Popover>
                            <ListBox>
                              {maps.map((m) => (
                                <ListBox.Item key={m} id={m} textValue={m}>
                                  <span className="font-mono">{m}</span>
                                  <ListBox.ItemIndicator />
                                </ListBox.Item>
                              ))}
                            </ListBox>
                          </Select.Popover>
                        </Select>
                      </div>
                      <div className="grid grid-cols-4 gap-3">
                        {(['x', 'y', 'z'] as const).map((axis) => (
                          <div key={axis}>
                            <span className={fieldLabelClass}>{axis.toUpperCase()}</span>
                            <NumberInput
                              value={zone[axis]}
                              onChange={(v) => setZone((z) => ({ ...z, [axis]: v }))}
                              ariaLabel={`${axis.toUpperCase()} coordinate`}
                            />
                          </div>
                        ))}
                        <div>
                          <span className={fieldLabelClass}>{t('events.editor.radius')}</span>
                          <NumberInput
                            value={zone.radius}
                            onChange={(v) => setZone((z) => ({ ...z, radius: v }))}
                            ariaLabel={t('events.editor.radius')}
                            min={1}
                          />
                        </div>
                      </div>
                      <div>
                        <span className={fieldLabelClass}>{t('events.editor.participants')}</span>
                        <div className="flex flex-col gap-1.5">
                          {availablePlayers.length > 0 && (
                            <Select
                              key={participantPickKey}
                              selectedKey=""
                              aria-label={t('events.editor.addParticipant')}
                              onSelectionChange={(k) => addParticipant(Number(k))}
                            >
                              <Select.Trigger>
                                <span className="text-sm text-muted flex-1">
                                  {t('events.editor.addParticipant')}
                                </span>
                                <Select.Indicator />
                              </Select.Trigger>
                              <Select.Popover>
                                <ListBox>
                                  {availablePlayers.map((p) => (
                                    <ListBox.Item key={p.account_id} id={p.account_id} textValue={p.name}>
                                      {p.name}
                                      <span className="text-muted text-[10px] ml-1.5 font-mono">
                                        {p.account_id}
                                      </span>
                                      <ListBox.ItemIndicator />
                                    </ListBox.Item>
                                  ))}
                                </ListBox>
                              </Select.Popover>
                            </Select>
                          )}
                          {players.length === 0 && zone.participants.length === 0 && (
                            <p className="text-xs text-muted">{t('events.editor.noPlayersLoaded')}</p>
                          )}
                          {zone.participants.length > 0 && (
                            <div className="flex flex-wrap gap-1">
                              {zone.participants.map((id) => (
                                <span
                                  key={id}
                                  className="inline-flex items-center gap-1 rounded-full bg-accent/15 text-accent px-2 py-0.5 text-xs font-medium"
                                >
                                  {playerName(id)}
                                  <CloseButton
                                    className="size-4 opacity-60 hover:opacity-100"
                                    onPress={() => setZone((z) => ({
                                      ...z,
                                      participants: z.participants.filter((p) => p !== id),
                                    }))}
                                    aria-label={`Remove ${playerName(id)}`}
                                  />
                                </span>
                              ))}
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  </FormSection>
                )}

                {type === 'milestone' && (
                  <FormSection className="has-[.tag-dropdown]:z-30">
                    <SectionLabel>{t('events.editor.milestoneConfig')}</SectionLabel>
                    <div className="flex flex-col gap-3 mt-2">
                      <div>
                        <span className={fieldLabelClass}>{t('events.editor.signal')}</span>
                        <Select
                          selectedKey={milestone.signal}
                          onSelectionChange={(k) => setMilestone((m) => ({
                            ...m,
                            signal: String(k) as MilestoneSignal,
                          }))}
                          aria-label={t('events.editor.signal')}
                          className="w-full"
                        >
                          <Select.Trigger>
                            <Select.Value />
                            <Select.Indicator />
                          </Select.Trigger>
                          <Select.Popover>
                            <ListBox>
                              <ListBox.Item key="level" id="level" textValue={t('events.signals.level')}>
                                {t('events.signals.level')}
                                <ListBox.ItemIndicator />
                              </ListBox.Item>
                              <ListBox.Item
                                key="achievement_tag"
                                id="achievement_tag"
                                textValue={t('events.signals.achievementTag')}
                              >
                                {t('events.signals.achievementTag')}
                                <ListBox.ItemIndicator />
                              </ListBox.Item>
                            </ListBox>
                          </Select.Popover>
                        </Select>
                      </div>
                      {milestone.signal === 'level' && (
                        <div>
                          <span className={fieldLabelClass}>{t('events.editor.levelThreshold')}</span>
                          <NumberInput
                            value={milestone.threshold}
                            onChange={(v) => setMilestone((m) => ({ ...m, threshold: v }))}
                            ariaLabel={t('events.editor.levelThreshold')}
                            min={1}
                            className="w-40"
                          />
                        </div>
                      )}
                      {milestone.signal === 'achievement_tag' && (
                        <div>
                          <span className={fieldLabelClass}>{t('events.editor.tagName')}</span>
                          <TagPickerField
                            value={milestone.tagName}
                            onSelect={(tag) => setMilestone((m) => ({ ...m, tagName: tag }))}
                            options={allGameplayTags ?? []}
                            ariaLabel={t('events.editor.tagName')}
                          />
                        </div>
                      )}
                      <div className="flex items-center gap-3">
                        <Switch
                          size="sm"
                          isSelected={milestone.awardPast}
                          onChange={() => setMilestone((m) => ({ ...m, awardPast: !m.awardPast }))}
                        >
                          <Switch.Control><Switch.Thumb /></Switch.Control>
                          <Switch.Content className="text-xs">{t('events.editor.awardPast')}</Switch.Content>
                        </Switch>
                        <span className="text-xs text-muted">{t('events.editor.awardPastHint')}</span>
                      </div>
                    </div>
                  </FormSection>
                )}

                <FormSection>
                  <SectionLabel>{t('events.editor.announceLabel')}</SectionLabel>
                  <div className="mt-2 flex flex-col gap-2">
                    <TextArea
                      aria-label={t('events.editor.announceTemplate')}
                      fullWidth
                      rows={3}
                      placeholder={type === 'zone_race'
                        ? '{player} completed {event}!'
                        : '{player} reached level {value} in {event}!'}
                      value={announceTemplate}
                      onChange={(e) => setAnnounceTemplate(e.target.value)}
                    />
                    <p className="text-xs text-muted">
                      {t('events.editor.templateHint')}
                      {' '}
                      {t('events.editor.channelDefault')}
                    </p>
                    {announceTemplate.trim() && (
                      <div className="rounded border border-border bg-surface-secondary px-3 py-2">
                        <span className="text-xs text-muted block mb-1">{t('events.editor.templatePreview')}</span>
                        <span className="text-sm">
                          {announceTemplate
                            .replace('{player}', players[0]?.name ?? 'PlayerName')
                            .replace('{event}', name || 'EventName')
                            .replace('{value}', type === 'milestone' ? String(milestone.threshold) : '')}
                        </span>
                      </div>
                    )}
                  </div>
                </FormSection>

                <FormSection>
                  <SectionLabel>{t('events.editor.scheduleLabel')}</SectionLabel>
                  <div className="mt-2 flex gap-4">
                    <NumberInput
                      label={t('events.editor.pollSeconds')}
                      min={1}
                      max={3600}
                      step={1}
                      value={pollSeconds}
                      onChange={setPollSeconds}
                      className="flex-1"
                    />
                    <NumberInput
                      label={t('events.editor.jitterSeconds')}
                      min={1}
                      max={300}
                      step={1}
                      value={jitterSeconds}
                      onChange={setJitterSeconds}
                      className="flex-1"
                    />
                  </div>
                  <p className="text-xs text-muted mt-1">{t('events.editor.scheduleHint')}</p>
                </FormSection>
              </div>

              {/* Rewards panel — always mounted */}
              <div
                className="flex flex-col flex-1 min-h-0 gap-4"
                style={{ display: activeSegment === 'rewards' ? undefined : 'none' }}
              >
                {/* Top row: Currency + XP side by side */}
                <div className="grid grid-cols-2 gap-4 shrink-0">
                  <FormSection>
                    <SectionLabel>{t('events.editor.currency')}</SectionLabel>
                    <div className="flex flex-col gap-2 mt-2">
                      <div>
                        <span className={fieldLabelClass}>{t('events.editor.solari')}</span>
                        <NumberInput
                          value={rewardCurrency}
                          onChange={setRewardCurrency}
                          ariaLabel={t('events.editor.solari')}
                          min={0}
                          prefix="₡"
                          className="w-full"
                        />
                      </div>
                      <div>
                        <span className={fieldLabelClass}>{t('events.editor.factionScrip')}</span>
                        <NumberInput
                          value={rewardFactionScrip}
                          onChange={setRewardFactionScrip}
                          ariaLabel={t('events.editor.factionScrip')}
                          min={0}
                          prefix="FS"
                          className="w-full"
                        />
                      </div>
                    </div>
                  </FormSection>

                  <FormSection>
                    <SectionLabel>{t('events.editor.xpRewards')}</SectionLabel>
                    <div className="flex items-end gap-2 mt-2">
                      <div className="w-40 shrink-0">
                        <span className={fieldLabelClass}>{t('events.editor.xpType')}</span>
                        <FieldSelect
                          value={xpType}
                          onChange={(v) => setXpType(v as XPType)}
                          options={['character', 'specialization']}
                          ariaLabel={t('events.editor.xpType')}
                        />
                      </div>
                      {xpType === 'specialization' && (
                        <div className="w-36 shrink-0">
                          <span className={fieldLabelClass}>{t('events.editor.xpTrack')}</span>
                          <FieldSelect
                            value={xpSpecTrack}
                            onChange={setXpSpecTrack}
                            options={[...XP_TRACKS]}
                            ariaLabel={t('events.editor.xpTrack')}
                          />
                        </div>
                      )}
                      <div className="flex-1 min-w-0">
                        <span className={fieldLabelClass}>{t('events.editor.xpAmount')}</span>
                        <NumberInput
                          value={xpAmount}
                          onChange={setXpAmount}
                          ariaLabel={t('events.editor.xpAmount')}
                          prefix="XP"
                          min={0}
                        />
                      </div>
                      <Button
                        size="sm"
                        variant="ghost"
                        onPress={addXP}
                        isDisabled={xpAmount <= 0}
                        className="shrink-0 self-end"
                      >
                        <Icon name="plus" />
                        {' '}
                        {t('common.add')}
                      </Button>
                    </div>
                    {rewardXP.length > 0 && (
                      <div className="flex flex-col gap-1 mt-2">
                        {rewardXP.map((x, idx) => (
                          <div
                            key={idx}
                            className="flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius)] text-xs bg-surface border border-border"
                          >
                            <span className="flex-1 font-mono text-foreground">{x.track}</span>
                            <span className="text-muted">
                              {x.amount.toLocaleString()}
                              {' XP'}
                            </span>
                            <Button
                              size="sm"
                              variant="danger-soft"
                              onPress={() => setRewardXP((prev) => prev.filter((_, i) => i !== idx))}
                              aria-label={t('common.remove')}
                            >
                              <Icon name="trash" />
                            </Button>
                          </div>
                        ))}
                      </div>
                    )}
                  </FormSection>
                </div>

                {/* Items — fills remaining space */}
                <FormSection className="flex-1 min-h-0 flex flex-col">
                  <SectionLabel>{t('events.editor.items')}</SectionLabel>
                  <div className="flex items-center gap-2 mt-2 shrink-0">
                    <CategorizedPackPicker
                      packs={packs}
                      onSelectPack={(id) => {
                        const pack = packs.find((p) => p.id === id)
                        if (pack) {
                          setRewardItems((prev) => [
                            ...prev,
                            ...pack.items.map((item) => ({ ...item, _key: nextKey() })),
                          ])
                        }
                      }}
                      className="flex-1"
                    />
                    <Button
                      size="sm"
                      variant="ghost"
                      onPress={() => setManagePacksOpen(true)}
                      aria-label={t('players.give.managePacks')}
                    >
                      <Icon name="settings-2" />
                      {' '}
                      {t('players.give.managePacks')}
                    </Button>
                  </div>
                  <div className="flex items-end gap-2 mt-2 shrink-0">
                    <TextField className="flex-1 min-w-0" aria-label={t('players.inventory.columns.template')}>
                      <div className="relative w-full">
                        <SearchField
                          className="w-full"
                          value={templateQuery}
                          onChange={(v) => {
                            setTemplateQuery(v)
                            setSelectedTemplate('')
                          }}
                        >
                          <SearchField.Group>
                            <SearchField.SearchIcon />
                            <SearchField.Input placeholder={t('players.give.searchTemplates')} />
                            <SearchField.ClearButton />
                          </SearchField.Group>
                        </SearchField>
                        {filteredTemplates.length > 0 && (
                          <div className="absolute z-[200] w-full mt-1 rounded-[var(--radius)] border border-border bg-surface overflow-y-auto max-h-48">
                            {filteredTemplates.map((tpl) => (
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
                    </TextField>
                    <NumberInput
                      prefix={t('players.give.qty')}
                      ariaLabel={t('players.give.qty')}
                      min={1}
                      value={itemQty}
                      onChange={setItemQty}
                      className="w-40 shrink-0"
                    />
                    <NumberInput
                      prefix={t('players.give.quality')}
                      ariaLabel={t('players.give.quality')}
                      min={0}
                      value={itemQuality}
                      onChange={setItemQuality}
                      className="w-40 shrink-0"
                    />
                    <Button
                      size="sm"
                      variant="ghost"
                      onPress={addRewardItem}
                      isDisabled={!selectedTemplate}
                      className="shrink-0"
                    >
                      <Icon name="plus" />
                      {' '}
                      {t('players.give.add')}
                    </Button>
                  </div>
                  {rewardItems.length > 0 && (
                    <DataGrid
                      aria-label={t('events.editor.items')}
                      columns={rewardItemColumns}
                      data={rewardItems}
                      getRowId={(item) => item._key}
                      selectedKeys={rewardItemKeys}
                      selectionMode="multiple"
                      showSelectionCheckboxes
                      onSelectionChange={setRewardItemKeys}
                      className="mt-2 flex-1 min-h-0"
                      scrollContainerClassName="h-full overflow-y-auto"
                      allowsColumnResize
                    />
                  )}
                </FormSection>
              </div>

            </Modal.Body>

            <Modal.Footer className="flex items-center gap-2 shrink-0">
              <Button size="sm" variant="tertiary" slot="close" onPress={onClose}>
                {t('common.cancel')}
              </Button>
              <Button size="sm" variant="secondary" onPress={handleSave} isDisabled={saving}>
                {isEdit ? t('common.save') : t('common.create')}
              </Button>
            </Modal.Footer>

            {/* Inside the dialog: outside it, React Aria's modal underlay
                makes the bar inert. The dialog's filter creates a containing
                block, so position:fixed pins it to the dialog bottom. */}
            <ActionBar aria-label={t('events.editor.items')} isOpen={rewardSelectionCount > 0}>
              <ActionBar.Prefix>
                <Chip size="sm" className="shrink-0 tabular-nums">{rewardSelectionCount}</Chip>
              </ActionBar.Prefix>
              <Separator />
              <ActionBar.Content>
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-danger"
                  onPress={handleBulkDeleteRewardItems}
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
                  onPress={() => setRewardItemKeys(new Set())}
                  aria-label={t('common.clearSelection')}
                >
                  <Icon name="x" />
                </Button>
              </ActionBar.Suffix>
            </ActionBar>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>

      <ManagePacksModal
        isOpen={managePacksOpen}
        onClose={() => setManagePacksOpen(false)}
        onSaved={(savedPacks) => {
          setPacks(savedPacks)
          setManagePacksOpen(false)
        }}
        templates={templates}
      />
    </>
  )
}
