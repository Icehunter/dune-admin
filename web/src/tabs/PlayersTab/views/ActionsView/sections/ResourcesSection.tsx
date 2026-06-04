import { useTranslation } from 'react-i18next'
import { Button, ListBox, ListLayout, Select, Virtualizer } from '@heroui/react'
import { NumberInput, Panel, SectionLabel } from '../../../../../dune-ui'
import allSkillModules from '../../../../../data/skillModules.json'
import { FACTIONS } from '../../../types'
import type { Player } from '../../../../../api/client'

interface ResourcesSectionProps {
  player: Player
  busy: boolean
  currency: number
  setCurrency: (v: number) => void
  scrip: number
  setScrip: (v: number) => void
  intel: number
  setIntel: (v: number) => void
  charXP: number
  setCharXP: (v: number) => void
  charXPCurrent: { xp: number, level: number } | null
  factionId: number
  setFactionId: (v: number) => void
  repDelta: number
  setRepDelta: (v: number) => void
  skillPointsAmount: number
  setSkillPointsAmount: (v: number) => void
  skillModule: string
  setSkillModule: (v: string) => void
  skillModuleLevel: number
  setSkillModuleLevel: (v: number) => void
  run: (fn: () => Promise<unknown>, label: string) => Promise<void>
}

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

const actionRow = (
  label: string,
  inputs: React.ReactNode,
  btnLabel: string,
  onAction: () => void,
  busy: boolean,
) => (
  <div className="flex items-end gap-3 py-3 border-b border-border/40 last:border-b-0">
    <div className="w-36 shrink-0 text-sm text-muted">{label}</div>
    <div className="flex items-end gap-2 flex-1 flex-wrap">{inputs}</div>
    <Button
      size="sm"
      variant="ghost"
      isDisabled={busy}
      onPress={onAction}
    >
      {btnLabel}
    </Button>
  </div>
)

export function ResourcesSection({
  player,
  busy,
  currency,
  setCurrency,
  scrip,
  setScrip,
  intel,
  setIntel,
  charXP,
  setCharXP,
  charXPCurrent,
  factionId,
  setFactionId,
  repDelta,
  setRepDelta,
  skillPointsAmount,
  setSkillPointsAmount,
  skillModule,
  setSkillModule,
  skillModuleLevel,
  setSkillModuleLevel,
  run,
}: ResourcesSectionProps) {
  const { t } = useTranslation()

  return (
    <div className="flex-1 overflow-y-auto flex flex-col gap-3 pr-2">
      <Panel>
        <SectionLabel>{t('players.actions.resources.currencyResources')}</SectionLabel>
        {actionRow(t('players.actions.resources.giveCurrency'), numInput(currency, setCurrency, 1, 9999999), t('players.actions.resources.give'), () =>
          run(
            () => import('../../../../../api/client').then((m) => m.api.players.giveCurrency(player.controller_id, currency)),
            `Gave ${currency} Solari to ${player.name}`,
          ),
        busy,
        )}
        {actionRow(t('players.actions.resources.giveScrip'), numInput(scrip, setScrip, 1, 9999999), t('players.actions.resources.give'), () =>
          run(
            () => import('../../../../../api/client').then((m) => m.api.players.giveScrip(player.controller_id, scrip)),
            `Gave ${scrip} scrip to ${player.name}`,
          ),
        busy,
        )}
        {actionRow(t('players.actions.resources.awardIntel'), numInput(intel, setIntel, 1, 9999999), t('players.actions.resources.award'), () =>
          run(
            () => import('../../../../../api/client').then((m) => m.api.players.awardIntel(player.id, intel)),
            `Awarded ${intel} intel to ${player.name}`,
          ),
        busy,
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
              () => import('../../../../../api/client').then((m) => m.api.players.awardCharXP(player.id, charXP, player.fls_id)),
              `Awarded ${charXP} char XP to ${player.name}`,
            ).then(() =>
              import('../../../../../api/client').then((m) =>
                m.api.players
                  .charXPCurrent(player.id)
                  .catch(() => {}),
              ),
            ),
          busy,
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
              () => import('../../../../../api/client').then((m) => m.api.players.setSkillPoints(player.fls_id, skillPointsAmount)),
              `Set skill points for ${player.name}`,
            ),
          busy,
        )}
        {actionRow(
          t('players.actions.resources.fillWater'),
          <span className="text-xs text-muted">{t('players.actions.resources.fillWaterNote')}</span>,
          t('players.actions.resources.fill'),
          () =>
            run(
              () => import('../../../../../api/client').then((m) => m.api.players.fillWater(player.fls_id)),
              `Fill water command sent for ${player.name}`,
            ),
          busy,
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
              () => import('../../../../../api/client').then((m) => m.api.players.setSkillModule(player.fls_id, skillModule, skillModuleLevel)),
              `Set ${skillModule} level ${skillModuleLevel} for ${player.name}`,
            ),
          busy,
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
                {FACTIONS.map((f: typeof FACTIONS[number]) => (
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
              () => import('../../../../../api/client').then((m) => m.api.players.giveFactionRep(player.controller_id, factionId, repDelta)),
              `Gave ${repDelta} rep (faction ${factionId}) to ${player.name}`,
            ),
          busy,
        )}
      </Panel>
    </div>
  )
}
