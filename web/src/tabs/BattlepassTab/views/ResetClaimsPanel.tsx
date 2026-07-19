import * as React from 'react'
import { Button, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { BattlepassResetResult, Player } from '../../../api/client'
import { PlayerSearchField } from '../../../components/PlayerSearchField'
import { ConfirmDialog, FieldSelect, Panel, SectionLabel } from '../../../dune-ui'
import type { BattlepassResetMode } from '../types'

// ResetClaimsPanel is the #293 incident-cleanup control: "demote" turns every
// bogus earned claim into baseline (nothing left for auto-grant; granted rows
// stay as delivery history) so the battlepass can be re-enabled without a
// re-grant storm; "purge" wipes claims and seen-markers together so the next
// scan re-baselines from scratch.
//
// Targeting used to require typing a raw account_id, which is shown nowhere
// else an operator would be looking (player list, detail panel) — they had to
// dig it out of the DB or the API directly. It now reuses the shared
// PlayerSearchField picker (name → account_id) like every other player
// picker in the app; leaving the field empty keeps the "all accounts" scope.
export const ResetClaimsPanel: React.FC = (): React.ReactElement => {
  const { t } = useTranslation()
  const [mode, setMode] = React.useState<BattlepassResetMode>('demote')
  const [target, setTarget] = React.useState<Player | null>(null)
  const [confirming, setConfirming] = React.useState(false)
  const [busy, setBusy] = React.useState(false)

  const accountId = target?.account_id ?? 0

  const runReset = (): void => {
    setConfirming(false)
    setBusy(true)
    api.battlepass.resetClaims(mode, accountId)
      .then((res: BattlepassResetResult) => {
        toast.success(t('battlepass.reset.done', {
          claims: res.claims,
          ledger: res.ledger_rows,
        }))
      })
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setBusy(false))
  }

  const scopeDescription = accountId === 0
    ? t('battlepass.reset.scopeAll')
    : t('battlepass.reset.scopeAccount', { accountId })

  return (
    <Panel>
      <SectionLabel>{t('battlepass.reset.section')}</SectionLabel>
      <p className="text-xs text-muted mb-3">{t('battlepass.reset.hint')}</p>
      <div className="flex items-end gap-3 flex-wrap">
        <FieldSelect
          value={mode}
          onChange={(v) => setMode(v as BattlepassResetMode)}
          options={['demote', 'purge']}
          className="w-40"
        />
        <PlayerSearchField
          onSelect={setTarget}
          onClear={() => setTarget(null)}
          ariaLabel={t('battlepass.reset.playerSearch')}
          placeholder={t('battlepass.reset.playerSearchPlaceholder')}
          className="w-64"
        />
        <Button size="sm" variant="danger-soft" isDisabled={busy} onPress={() => setConfirming(true)}>
          {t('battlepass.reset.run')}
        </Button>
      </div>
      <p className="text-xs text-muted mt-2">{scopeDescription}</p>
      <p className="text-xs text-muted mt-1">
        {mode === 'demote' ? t('battlepass.reset.demoteHint') : t('battlepass.reset.purgeHint')}
      </p>
      <ConfirmDialog
        open={confirming}
        title={t('battlepass.reset.confirmTitle', { mode })}
        description={`${scopeDescription} ${mode === 'demote' ? t('battlepass.reset.demoteHint') : t('battlepass.reset.purgeHint')}`}
        confirmLabel={t('battlepass.reset.run')}
        onConfirm={runReset}
        onCancel={() => setConfirming(false)}
      />
    </Panel>
  )
}
