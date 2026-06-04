import { useTranslation } from 'react-i18next'
import { Button, Input } from '@heroui/react'
import { Panel, SectionLabel } from '../../../../../dune-ui'
import type { Player } from '../../../../../api/client'

interface ExperimentalSectionProps {
  player: Player
  busy: boolean
  customScriptName: string
  setCustomScriptName: (v: string) => void
  run: (fn: () => Promise<unknown>, label: string) => Promise<void>
  gate: (title: string, description: string, confirmLabel: string, action: () => void) => void
}

export function ExperimentalSection({
  player,
  busy,
  customScriptName,
  setCustomScriptName,
  run,
  gate,
}: ExperimentalSectionProps) {
  const { t } = useTranslation()

  return (
    <div className="flex-1 overflow-y-auto flex flex-col gap-3 pr-2">
      <div className="text-xs px-3 py-2 rounded bg-danger/10 border border-danger/40 text-danger">
        {t('players.actions.experimental.warning')}
      </div>
      <Panel>
        <SectionLabel>{t('players.actions.experimental.knownScripts')}</SectionLabel>
        <div className="text-xs text-muted mb-2">{t('players.actions.experimental.knownScriptsDesc')}</div>
        {(
          [
            { name: 'LeaveMeAlone', label: t('players.actions.experimental.scripts.LeaveMeAlone'), desc: t('players.actions.experimental.scripts.LeaveMeAloneDesc'), danger: false },
            { name: 'AwardPlayerXP', label: t('players.actions.experimental.scripts.AwardPlayerXP'), desc: t('players.actions.experimental.scripts.AwardPlayerXPDesc'), danger: false },
            { name: 'UnlockAllSkills', label: t('players.actions.experimental.scripts.UnlockAllSkills'), desc: t('players.actions.experimental.scripts.UnlockAllSkillsDesc'), danger: false },
            { name: 'UnlockAllAbilities', label: t('players.actions.experimental.scripts.UnlockAllAbilities'), desc: t('players.actions.experimental.scripts.UnlockAllAbilitiesDesc'), danger: false },
            { name: 'PlaytestSetup', label: t('players.actions.experimental.scripts.PlaytestSetup'), desc: t('players.actions.experimental.scripts.PlaytestSetupDesc'), danger: true },
            { name: 'PlaytestSetupAdmin', label: t('players.actions.experimental.scripts.PlaytestSetupAdmin'), desc: t('players.actions.experimental.scripts.PlaytestSetupAdminDesc'), danger: true },
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
                      gate(t('players.actions.experimental.runTitle', { label }), desc.replace(/^⚠ {2}DESTRUCTIVE — /, ''), t('players.actions.experimental.confirmRun'), () =>
                        run(
                          () => import('../../../../../api/client').then((m) => m.api.players.cheatScript(player.fls_id, name)),
                          `CheatScript ${name} sent for ${player.name}`,
                        ),
                      )
                  : () =>
                      run(
                        () => import('../../../../../api/client').then((m) => m.api.players.cheatScript(player.fls_id, name)),
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
                () => import('../../../../../api/client').then((m) => m.api.players.cheatScript(player.fls_id, customScriptName)),
                `CheatScript "${customScriptName}" sent for ${player.name}`,
              )}
          >
            {t('players.actions.experimental.try')}
          </Button>
        </div>
      </Panel>
    </div>
  )
}
