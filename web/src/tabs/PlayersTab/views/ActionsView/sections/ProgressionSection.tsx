import { useTranslation } from 'react-i18next'
import { Button, Chip, ListBox, Select } from '@heroui/react'
import { Panel, SectionLabel } from '../../../../../dune-ui'
import type { Player, ProgressionPreset } from '../../../../../api/client'

const TRAINERS = ['BeneGesserit', 'Mentat', 'Planetologist', 'Swordmaster', 'Trooper'] as const
type TrainerKey = (typeof TRAINERS)[number]

const MAIN_QUESTS = [
  { id: 'DA_MQ_ANewBeginning', label: '1. A New Beginning', nodes: 132 },
  { id: 'DA_MQ_AssassinsHandbook', label: '2. Assassin\'s Handbook', nodes: 91 },
  { id: 'DA_MQ_FindTheFremen', label: '3. Find the Fremen', nodes: 46 },
  { id: 'DA_MQ_TheGreatConvention', label: '4. The Great Convention', nodes: 90 },
  { id: 'DA_MQ_TheGreatConventionPt2', label: '5. Great Convention Pt 2', nodes: 109 },
  { id: 'DA_MQ_TheBloodline', label: '6. The Bloodline (standalone)', nodes: 0 },
] as const

interface ProgressionSectionProps {
  player: Player
  busy: boolean
  presets: ProgressionPreset[]
  presetsLoaded: boolean
  contractCatalog: { id: string, alias: string }[]
  contractCatalogLoaded: boolean
  contractCatalogError: string
  selectedTrainer: TrainerKey
  setSelectedTrainer: (v: TrainerKey) => void
  selectedMQ: string
  setSelectedMQ: (v: string) => void
  unlockFaction: string
  setUnlockFaction: (v: string) => void
  unlockPreset: string
  setUnlockPreset: (v: string) => void
  run: (fn: () => Promise<unknown>, label: string) => Promise<void>
  gate: (title: string, description: string, confirmLabel: string, action: () => void) => void
  onNodesLoaded: () => void
}

export function ProgressionSection({
  player,
  busy,
  presets,
  presetsLoaded,
  contractCatalog,
  contractCatalogLoaded,
  contractCatalogError,
  selectedTrainer,
  setSelectedTrainer,
  selectedMQ,
  setSelectedMQ,
  unlockFaction,
  setUnlockFaction,
  unlockPreset,
  setUnlockPreset,
  run,
  gate,
  onNodesLoaded,
}: ProgressionSectionProps) {
  const { t } = useTranslation()

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
                        <div className="text-sm font-semibold">
                          {t(`players.actions.progression.presets.${p.id}.name`, { defaultValue: p.name })}
                        </div>
                        <div className="text-xs text-muted">
                          {t(`players.actions.progression.presets.${p.id}.desc`, { defaultValue: p.description })}
                        </div>
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
                            () => import('../../../../../api/client').then((m) => m.api.progression.applyPreset(player.account_id, p.id)),
                            `Applied preset '${p.name}' to ${player.name}`,
                          ).then(onNodesLoaded)}
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
                () => import('../../../../../api/client').then((m) => m.api.players.progressionUnlock(player.id, unlockFaction, unlockPreset)),
                `Applied ${unlockPreset} (${unlockFaction}) to ${player.name}`,
              ).then(onNodesLoaded)}
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
                    () => import('../../../../../api/client').then((m) => m.api.players.progressionReverse(player.id, unlockFaction, unlockPreset)),
                    `Reversed ${unlockPreset} (${unlockFaction}) for ${player.name}`,
                  ).then(onNodesLoaded),
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
                    const mapi = await import('../../../../../api/client')
                    const r = await mapi.api.players.completeContracts(player.account_id, trainerMatches)
                    await mapi.api.players.grantJobSkills(player.account_id, selectedTrainer)
                    return r
                  }, `Unlocked ${selectedTrainer} (${trainerMatches.length} contracts + skill tree) for ${player.name}`).then(
                    onNodesLoaded,
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
                        () => import('../../../../../api/client').then((m) => m.api.players.resetJobSkills(player.account_id, selectedTrainer)),
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
                  () => import('../../../../../api/client').then((m) => m.api.players.journeyComplete(player.account_id, selectedMQ)),
                  `Unlocked ${selectedMQDef?.label ?? selectedMQ} for ${player.name}`,
                ).then(onNodesLoaded)}
            >
              {t('players.actions.progression.apply')}
            </Button>
          </div>
        </Panel>
      </div>
    </div>
  )
}
