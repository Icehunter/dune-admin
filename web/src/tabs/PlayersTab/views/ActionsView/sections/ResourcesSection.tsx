import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useAtom } from 'jotai'
import { Button, ListBox, ListLayout, Select, Virtualizer } from '@heroui/react'
import { NumberInput, Panel, SectionLabel } from '../../../../../dune-ui'
import { skillModulesSyncAtom } from '../../../../../data/store'
import { api } from '../../../../../api/client'
import { FACTIONS } from '../../../types'
import { busyAtom, charXPCurrentAtom } from '../store'
import { useRun } from '../hooks/useActions'
import type { ResourcesSectionProps } from './types'

export const ResourcesSection: React.FC<ResourcesSectionProps> = ({ player }) => {
  const { t } = useTranslation()
  const [busy] = useAtom(busyAtom(player.id))
  const [charXPCurrent, setCharXPCurrent] = useAtom(charXPCurrentAtom(player.id))
  const run = useRun(player.id)
  const [allSkillModules] = useAtom(skillModulesSyncAtom)

  const [currency, setCurrency] = React.useState(100)
  const [scrip, setScrip] = React.useState(100)
  const [intel, setIntel] = React.useState(100)
  const [charXP, setCharXP] = React.useState(1000)
  const [factionId, setFactionId] = React.useState(player.faction_id > 0 ? player.faction_id : 1)
  const [repDelta, setRepDelta] = React.useState(100)
  const [skillPointsAmount, setSkillPointsAmount] = React.useState(10)
  const [skillModule, setSkillModule] = React.useState('')
  const [skillModuleLevel, setSkillModuleLevel] = React.useState(1)

  React.useEffect(() => {
    Promise.resolve().then(() => setFactionId(player.faction_id > 0 ? player.faction_id : 1))
  }, [player.faction_id])

  const handleGiveCurrency = () =>
    run(
      () => api.players.giveCurrency(player.controller_id, currency),
      `Gave ${currency} Solari to ${player.name}`,
    )

  const handleGiveScrip = () =>
    run(
      () => api.players.giveScrip(player.controller_id, scrip),
      `Gave ${scrip} scrip to ${player.name}`,
    )

  const handleAwardIntel = () =>
    run(
      () => api.players.awardIntel(player.id, intel),
      `Awarded ${intel} intel to ${player.name}`,
    )

  const handleAwardCharXP = () =>
    run(
      () => api.players.awardCharXP(player.id, charXP, player.fls_id),
      `Awarded ${charXP} char XP to ${player.name}`,
    ).then(() => api.players.charXPCurrent(player.id).then(setCharXPCurrent).catch(() => {}))

  const handleSetSkillPoints = () =>
    run(
      () => api.players.setSkillPoints(player.fls_id, skillPointsAmount),
      `Set skill points for ${player.name}`,
    )

  const handleFillWater = () =>
    run(
      () => api.players.fillWater(player.fls_id),
      `Fill water command sent for ${player.name}`,
    )

  const handleSetSkillModule = () =>
    run(
      () => api.players.setSkillModule(player.fls_id, skillModule, skillModuleLevel),
      `Set ${skillModule} level ${skillModuleLevel} for ${player.name}`,
    )

  const handleFactionSelect = (k: React.Key | null) => setFactionId(Number(k))

  const handleGiveFactionRep = () =>
    run(
      () => api.players.giveFactionRep(player.controller_id, factionId, repDelta),
      `Gave ${repDelta} rep (faction ${factionId}) to ${player.name}`,
    )

  const numInput = (val: number, set: (v: number) => void, min = 1, max = 9999999) => (
    <NumberInput
      ariaLabel="number"
      min={min}
      max={max}
      value={val}
      onChange={(v: number) => set(Math.max(min, Math.min(max, v)))}
      className="w-40"
    />
  )

  const actionRow = (label: string, inputs: React.ReactNode, btnLabel: string, onAction: () => void) => (
    <div className="flex items-end gap-3 py-3 border-b border-border/40 last:border-b-0">
      <div className="w-36 shrink-0 text-sm text-muted">{label}</div>
      <div className="flex items-end gap-2 flex-1 flex-wrap">{inputs}</div>
      <Button size="sm" variant="ghost" isDisabled={busy} onPress={onAction}>{btnLabel}</Button>
    </div>
  )

  return (
    <div className="flex-1 overflow-y-auto flex flex-col gap-3 pr-2">
      <Panel>
        <SectionLabel>{t('players.actions.resources.currencyResources')}</SectionLabel>
        {actionRow(
          t('players.actions.resources.giveCurrency'),
          numInput(currency, setCurrency, 1, 9999999),
          t('players.actions.resources.give'),
          handleGiveCurrency,
        )}
        {actionRow(
          t('players.actions.resources.giveScrip'),
          numInput(scrip, setScrip, 1, 9999999),
          t('players.actions.resources.give'),
          handleGiveScrip,
        )}
        {actionRow(
          t('players.actions.resources.awardIntel'),
          numInput(intel, setIntel, 1, 9999999),
          t('players.actions.resources.award'),
          handleAwardIntel,
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
            <span className="text-xs text-muted">{t('players.actions.resources.charXPNote')}</span>
          </div>,
          t('players.actions.resources.award'),
          handleAwardCharXP,
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
          handleSetSkillPoints,
        )}
        {actionRow(
          t('players.actions.resources.fillWater'),
          <span className="text-xs text-muted">{t('players.actions.resources.fillWaterNote')}</span>,
          t('players.actions.resources.fill'),
          handleFillWater,
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
                    items={allSkillModules.map((m) => ({ id: m.id, label: m.label }))}
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
          handleSetSkillModule,
        )}
      </Panel>

      <Panel>
        <SectionLabel>{t('players.actions.resources.factionReputation')}</SectionLabel>
        <div className="flex items-center gap-2 py-3 border-b border-border/40">
          <div className="w-36 shrink-0 text-sm text-muted">{t('players.actions.resources.faction')}</div>
          <Select selectedKey={String(factionId)} onSelectionChange={handleFactionSelect} className="w-40" aria-label={t('players.actions.resources.faction')}>
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
          handleGiveFactionRep,
        )}
      </Panel>
    </div>
  )
}
