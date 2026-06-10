import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useAtom } from 'jotai'
import { Button, Input } from '@heroui/react'
import { Panel, SectionLabel } from '../../../../../dune-ui'
import { api } from '../../../../../api/client'
import type { Player } from '../../../../../api/client'
import { busyAtom } from '../store'
import { cheatScriptsSyncAtom } from '../../../../../data/store'
import { useRun, useGate } from '../hooks/useActions'

interface ExperimentalSectionProps { player: Player }

export function ExperimentalSection({ player }: ExperimentalSectionProps) {
  const { t } = useTranslation()
  const [busy] = useAtom(busyAtom(player.id))
  const run = useRun(player.id)
  const gate = useGate(player.id)
  const [customScriptName, setCustomScriptName] = useState('')
  const [scripts] = useAtom(cheatScriptsSyncAtom)

  const handleRunScript = (name: string, danger: boolean) => {
    const label = t(`players.actions.experimental.scripts.${name}` as never)
    const desc = t(`players.actions.experimental.scripts.${name}Desc` as never)
    const successMsg = `CheatScript ${name} sent for ${player.name}`

    if (danger) {
      gate(
        t('players.actions.experimental.runTitle', { label }),
        String(desc).replace(/^⚠ {2}DESTRUCTIVE — /, ''),
        t('players.actions.experimental.confirmRun'),
        () => run(() => api.players.cheatScript(player.fls_id, name), successMsg),
      )
    }
    else {
      run(() => api.players.cheatScript(player.fls_id, name), successMsg)
    }
  }

  const handleRunCustomScript = () => {
    const successMsg = `CheatScript "${customScriptName}" sent for ${player.name}`
    run(() => api.players.cheatScript(player.fls_id, customScriptName), successMsg)
  }

  return (
    <div className="flex-1 overflow-y-auto flex flex-col gap-3 pr-2">
      <div className="text-xs px-3 py-2 rounded bg-danger/10 border border-danger/40 text-danger">
        {t('players.actions.experimental.warning')}
      </div>
      <Panel>
        <SectionLabel>{t('players.actions.experimental.knownScripts')}</SectionLabel>
        <div className="text-xs text-muted mb-2">{t('players.actions.experimental.knownScriptsDesc')}</div>
        {scripts.map(({ name, danger }) => {
          const label = t(`players.actions.experimental.scripts.${name}` as never)
          const desc = t(`players.actions.experimental.scripts.${name}Desc` as never)
          return (
            <div key={name} className="flex items-center gap-3 py-3 border-b border-border/40 last:border-b-0">
              <div className="flex-1">
                <div className="text-sm">{label}</div>
                <div className="text-xs text-muted">{desc}</div>
              </div>
              <Button
                size="sm"
                variant={danger ? 'danger-soft' : 'ghost'}
                isDisabled={busy}
                onPress={() => handleRunScript(name, danger)}
              >
                {t('players.actions.experimental.run')}
              </Button>
            </div>
          )
        })}
      </Panel>
      <Panel>
        <SectionLabel>{t('players.actions.experimental.customScript')}</SectionLabel>
        <div className="text-xs text-muted mb-2">{t('players.actions.experimental.customScriptDesc')}</div>
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
            onPress={handleRunCustomScript}
          >
            {t('players.actions.experimental.try')}
          </Button>
        </div>
      </Panel>
    </div>
  )
}
