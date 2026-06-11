import type React from 'react'
import { useState, useEffect, useMemo } from 'react'
import {
  Button, ListBox, Modal, SearchField, Select, Switch, TextField, toast,
} from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { FieldInput, FieldSelect, Icon, NumberInput, Panel, SectionLabel } from '../../../dune-ui'
import { api } from '../../../api/client'
import type { EventDefinition, Player } from '../../../api/client'
import type { MilestoneFields, MilestoneSignal, RewardFields, XPType, ZoneRaceFields } from '../types'
import { XP_TRACKS } from '../types'

interface EventEditorModalProps {
  isOpen: boolean
  onClose: () => void
  editing: EventDefinition | null
  onSaved: () => void
}

function parseZoneConfig(raw: string): ZoneRaceFields {
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

function parseMilestoneConfig(raw: string): MilestoneFields {
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

function parseReward(raw: string): RewardFields {
  try {
    const r = JSON.parse(raw || '{}') as Record<string, unknown>
    return {
      currency: typeof r.currency === 'number' ? r.currency : 0,
      items: Array.isArray(r.items) ? r.items as RewardFields['items'] : [],
      xpRewards: Array.isArray(r.xp) ? r.xp as RewardFields['xpRewards'] : [],
    }
  }
  catch {
    return { currency: 0, items: [], xpRewards: [] }
  }
}

function serializeConfig(
  type: EventDefinition['type'],
  zone: ZoneRaceFields,
  ms: MilestoneFields,
): string {
  if (type === 'zone_race') {
    return JSON.stringify({
      map: zone.map,
      x: zone.x,
      y: zone.y,
      z: zone.z,
      radius: zone.radius,
      participants: zone.participants,
    })
  }
  const cfg: Record<string, unknown> = { signal: ms.signal, award_past: ms.awardPast }
  if (ms.signal === 'level') {
    cfg.threshold = ms.threshold
  }
  else {
    cfg.tag_name = ms.tagName
  }
  return JSON.stringify(cfg)
}

function serializeReward(reward: RewardFields): string {
  const r: Record<string, unknown> = {}
  if (reward.currency > 0) r.currency = reward.currency
  if (reward.items.length > 0) r.items = reward.items
  if (reward.xpRewards.length > 0) r.xp = reward.xpRewards
  if (Object.keys(r).length === 0) return ''
  return JSON.stringify(r)
}

export const EventEditorModal: React.FC<EventEditorModalProps> = ({
  isOpen,
  onClose,
  editing,
  onSaved,
}) => {
  const { t } = useTranslation()
  const isEdit = editing !== null

  const [name, setName] = useState('')
  const [type, setType] = useState<EventDefinition['type']>('milestone')
  const [zone, setZone] = useState<ZoneRaceFields>(
    { map: '', x: 0, y: 0, z: 0, radius: 500, participants: [] },
  )
  const [players, setPlayers] = useState<Player[]>([])
  const [participantPickKey, setParticipantPickKey] = useState(0)
  const [milestone, setMilestone] = useState<MilestoneFields>(
    { signal: 'level', threshold: 50, tagName: '', awardPast: false },
  )
  const [reward, setReward] = useState<RewardFields>({ currency: 0, items: [], xpRewards: [] })
  const [maps, setMaps] = useState<string[]>([])
  const [templates, setTemplates] = useState<{ id: string, name: string }[]>([])
  const [templateQuery, setTemplateQuery] = useState('')
  const [selectedTemplate, setSelectedTemplate] = useState('')
  const [itemQty, setItemQty] = useState(1)
  const [itemQuality, setItemQuality] = useState(0)
  const [xpType, setXpType] = useState<XPType>('specialization')
  const [xpSpecTrack, setXpSpecTrack] = useState<string>(XP_TRACKS[0])
  const [xpAmount, setXpAmount] = useState(0)
  const [announceTemplate, setAnnounceTemplate] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!isOpen) return
    Promise.resolve().then(() => {
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
        setReward(parseReward(editing.reward))
        setAnnounceTemplate(editing.announce_template || '')
      }
      else {
        setName('')
        setType('milestone')
        setZone({ map: '', x: 0, y: 0, z: 0, radius: 500, participants: [] })
        setMilestone({ signal: 'level', threshold: 50, tagName: '', awardPast: false })
        setReward({ currency: 0, items: [], xpRewards: [] })
        setAnnounceTemplate('')
      }
      setTemplateQuery('')
      setSelectedTemplate('')
      setItemQty(1)
      setItemQuality(0)
      setXpType('specialization')
      setXpSpecTrack(XP_TRACKS[0])
      setXpAmount(0)
      setParticipantPickKey((k) => k + 1)
    })
    api.maps.list().then(setMaps).catch(() => {})
    api.players.templates().then(setTemplates).catch(() => {})
    api.players.list().then(setPlayers).catch(() => {})
  }, [isOpen, editing])

  const filteredTemplates = useMemo(() => {
    if (!templateQuery) return []
    const q = templateQuery.toLowerCase()
    return templates
      .filter((tpl) => tpl.id.toLowerCase().includes(q) || tpl.name.toLowerCase().includes(q))
      .slice(0, 100)
  }, [templates, templateQuery])

  const nameMap = useMemo(
    () => new Map(templates.map((tpl) => [tpl.id, tpl.name])),
    [templates],
  )

  const handleTypeChange = (newType: string) => {
    const t2 = newType as EventDefinition['type']
    setType(t2)
    if (t2 === 'zone_race') {
      setZone({ map: '', x: 0, y: 0, z: 0, radius: 500, participants: [] })
    }
    else {
      setMilestone({ signal: 'level', threshold: 50, tagName: '', awardPast: false })
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

  const addItem = () => {
    if (!selectedTemplate) return
    setReward((r) => ({
      ...r,
      items: [...r.items, { template: selectedTemplate, qty: itemQty, quality: itemQuality }],
    }))
    setTemplateQuery('')
    setSelectedTemplate('')
    setItemQty(1)
    setItemQuality(0)
  }

  const updateItem = (idx: number, field: 'qty' | 'quality', value: number) => {
    setReward((r) => ({
      ...r,
      items: r.items.map((item, i) => i === idx ? { ...item, [field]: value } : item),
    }))
  }

  const addXP = () => {
    if (xpAmount <= 0) return
    const track = xpType === 'character' ? 'character' : xpSpecTrack
    setReward((r) => ({
      ...r,
      xpRewards: [...r.xpRewards, { track, amount: xpAmount }],
    }))
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
      reward: serializeReward(reward),
      announce_channel_id: '',
      announce_template: announceTemplate,
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
        toast.danger(
          t('events.editor.saveFailed', { message: e instanceof Error ? e.message : String(e) }),
        )
      })
      .finally(() => setSaving(false))
  }

  const lbl = 'text-xs text-muted mb-1 block'
  const rowCard = 'flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius)] text-xs bg-surface border border-border'

  return (
    <Modal>
      <Modal.Backdrop isOpen={isOpen} onOpenChange={(open) => { if (!open) onClose() }}>
        <Modal.Container size="cover" scroll="outside">
          <Modal.Dialog className="max-h-[90vh] flex flex-col">
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading className="text-accent">
                {isEdit ? t('events.editor.editTitle') : t('events.editor.createTitle')}
              </Modal.Heading>
            </Modal.Header>
            <Modal.Body className="flex flex-col gap-4 overflow-y-auto flex-1 min-h-0">

              {/* Basic Info */}
              <Panel>
                <SectionLabel>{t('events.editor.basics')}</SectionLabel>
                <div className="flex gap-4 mt-2">
                  <div className="flex-1">
                    <span className={lbl}>{t('events.editor.name')}</span>
                    <FieldInput
                      value={name}
                      onChange={setName}
                      placeholder={t('events.editor.namePlaceholder')}
                      ariaLabel={t('events.editor.name')}
                    />
                  </div>
                  <div className="w-44 shrink-0">
                    <span className={lbl}>{t('events.editor.type')}</span>
                    <FieldSelect
                      value={type}
                      onChange={handleTypeChange}
                      options={['milestone', 'zone_race']}
                      isDisabled={isEdit}
                      ariaLabel={t('events.editor.type')}
                    />
                  </div>
                </div>
              </Panel>

              {/* Zone Race Config */}
              {type === 'zone_race' && (
                <Panel>
                  <SectionLabel>{t('events.editor.zoneConfig')}</SectionLabel>
                  <div className="flex flex-col gap-3 mt-2">
                    <div>
                      <span className={lbl}>{t('events.editor.zoneMap')}</span>
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
                          <span className={lbl}>{axis.toUpperCase()}</span>
                          <NumberInput
                            value={zone[axis]}
                            onChange={(v) => setZone((z) => ({ ...z, [axis]: v }))}
                            ariaLabel={`${axis.toUpperCase()} coordinate`}
                          />
                        </div>
                      ))}
                      <div>
                        <span className={lbl}>{t('events.editor.radius')}</span>
                        <NumberInput
                          value={zone.radius}
                          onChange={(v) => setZone((z) => ({ ...z, radius: v }))}
                          ariaLabel={t('events.editor.radius')}
                          min={1}
                        />
                      </div>
                    </div>
                    <div>
                      <span className={lbl}>{t('events.editor.participants')}</span>
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
                                  <ListBox.Item
                                    key={p.account_id}
                                    id={p.account_id}
                                    textValue={p.name}
                                  >
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
                                <button
                                  type="button"
                                  className="leading-none opacity-60 hover:opacity-100"
                                  onClick={() => setZone((z) => ({
                                    ...z,
                                    participants: z.participants.filter((p) => p !== id),
                                  }))}
                                  aria-label={`Remove ${playerName(id)}`}
                                >
                                  ×
                                </button>
                              </span>
                            ))}
                          </div>
                        )}
                      </div>
                    </div>
                  </div>
                </Panel>
              )}

              {/* Milestone Config */}
              {type === 'milestone' && (
                <Panel>
                  <SectionLabel>{t('events.editor.milestoneConfig')}</SectionLabel>
                  <div className="flex flex-col gap-3 mt-2">
                    <div>
                      <span className={lbl}>{t('events.editor.signal')}</span>
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
                        <span className={lbl}>{t('events.editor.levelThreshold')}</span>
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
                        <span className={lbl}>{t('events.editor.tagName')}</span>
                        <FieldInput
                          value={milestone.tagName}
                          onChange={(v) => setMilestone((m) => ({ ...m, tagName: v }))}
                          placeholder="e.g. SpiceVision_Unlocked"
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
                        <span className="text-xs">{t('events.editor.awardPast')}</span>
                      </Switch>
                      <span className="text-xs text-muted">{t('events.editor.awardPastHint')}</span>
                    </div>
                  </div>
                </Panel>
              )}

              {/* Reward */}
              <Panel>
                <SectionLabel>
                  {`${t('events.editor.rewardLabel')} (${t('common.optional')})`}
                </SectionLabel>
                <div className="flex flex-col gap-3 mt-2">
                  <div>
                    <span className={lbl}>{t('events.editor.currency')}</span>
                    <NumberInput
                      value={reward.currency}
                      onChange={(v) => setReward((r) => ({ ...r, currency: v }))}
                      ariaLabel={t('events.editor.currency')}
                      min={0}
                      prefix="₡"
                      className="w-48"
                    />
                  </div>

                  {/* Item picker */}
                  <div>
                    <span className={lbl}>{t('events.editor.items')}</span>
                    <div className="flex items-end gap-2">
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
                        onPress={addItem}
                        isDisabled={!selectedTemplate}
                        className="shrink-0"
                      >
                        <Icon name="plus" />
                        {' '}
                        {t('players.give.add')}
                      </Button>
                    </div>
                    {reward.items.length > 0 && (
                      <div className="flex flex-col gap-1 mt-2">
                        {reward.items.map((item, idx) => (
                          <div key={idx} className={rowCard}>
                            <div className="flex-1 min-w-0 leading-tight">
                              <div className="truncate text-foreground">
                                {nameMap.get(item.template) || item.template}
                              </div>
                              {nameMap.get(item.template) && (
                                <div className="font-mono text-[10px] text-muted truncate">
                                  {item.template}
                                </div>
                              )}
                            </div>
                            <NumberInput
                              ariaLabel={`qty for ${item.template}`}
                              prefix={t('players.give.qty')}
                              min={1}
                              value={item.qty}
                              onChange={(v) => updateItem(idx, 'qty', v)}
                              className="w-40"
                            />
                            <NumberInput
                              ariaLabel={`quality for ${item.template}`}
                              prefix={t('players.give.quality')}
                              min={0}
                              value={item.quality}
                              onChange={(v) => updateItem(idx, 'quality', v)}
                              className="w-40"
                            />
                            <Button
                              size="sm"
                              variant="danger-soft"
                              onPress={() => setReward((r) => ({
                                ...r,
                                items: r.items.filter((_, i) => i !== idx),
                              }))}
                              aria-label={t('common.remove')}
                            >
                              <Icon name="x" />
                            </Button>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>

                  {/* XP rewards */}
                  <div>
                    <span className={lbl}>{t('events.editor.xpRewards')}</span>
                    <div className="flex items-end gap-2">
                      <div className="w-48 shrink-0">
                        <span className={lbl}>{t('events.editor.xpType')}</span>
                        <FieldSelect
                          value={xpType}
                          onChange={(v) => setXpType(v as XPType)}
                          options={['character', 'specialization']}
                          ariaLabel={t('events.editor.xpType')}
                        />
                      </div>
                      {xpType === 'specialization' && (
                        <div className="w-44 shrink-0">
                          <span className={lbl}>{t('events.editor.xpTrack')}</span>
                          <FieldSelect
                            value={xpSpecTrack}
                            onChange={setXpSpecTrack}
                            options={[...XP_TRACKS]}
                            ariaLabel={t('events.editor.xpTrack')}
                          />
                        </div>
                      )}
                      <div className="flex-1">
                        <span className={lbl}>{t('events.editor.xpAmount')}</span>
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
                    {reward.xpRewards.length > 0 && (
                      <div className="flex flex-col gap-1 mt-2">
                        {reward.xpRewards.map((x, idx) => (
                          <div key={idx} className={rowCard}>
                            <span className="flex-1 font-mono text-foreground">{x.track}</span>
                            <span className="text-muted">
                              {x.amount.toLocaleString()}
                              {' XP'}
                            </span>
                            <Button
                              size="sm"
                              variant="danger-soft"
                              onPress={() => setReward((r) => ({
                                ...r,
                                xpRewards: r.xpRewards.filter((_, i) => i !== idx),
                              }))}
                              aria-label={t('common.remove')}
                            >
                              <Icon name="x" />
                            </Button>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              </Panel>

              {/* Announcement */}
              <Panel>
                <SectionLabel>{t('events.editor.announceLabel')}</SectionLabel>
                <div className="mt-2">
                  <span className={lbl}>{t('events.editor.announceTemplate')}</span>
                  <FieldInput
                    value={announceTemplate}
                    onChange={setAnnounceTemplate}
                    placeholder="{player} won {event}!"
                    ariaLabel={t('events.editor.announceTemplate')}
                  />
                  <p className="text-xs text-muted mt-1">
                    {t('events.editor.templateHint')}
                    {' '}
                    {t('events.editor.channelDefault')}
                  </p>
                </div>
              </Panel>

            </Modal.Body>
            <Modal.Footer className="flex items-center gap-2">
              <Button size="sm" variant="tertiary" slot="close" onPress={onClose}>
                {t('common.cancel')}
              </Button>
              <Button size="sm" variant="secondary" onPress={handleSave} isDisabled={saving}>
                {isEdit ? t('common.save') : t('common.create')}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
