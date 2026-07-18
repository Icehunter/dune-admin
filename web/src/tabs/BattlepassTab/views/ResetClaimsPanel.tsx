import * as React from 'react'
import { Button, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { BattlepassResetResult } from '../../../api/client'
import { ConfirmDialog, FieldSelect, NumberInput, Panel, SectionLabel } from '../../../dune-ui'
import type { BattlepassResetMode } from '../types'

// ResetClaimsPanel is the #293 incident-cleanup control: "demote" turns every
// bogus earned claim into baseline (nothing left for auto-grant; granted rows
// stay as delivery history) so the battlepass can be re-enabled without a
// re-grant storm; "purge" wipes claims and seen-markers together so the next
// scan re-baselines from scratch.
export const ResetClaimsPanel: React.FC = (): React.ReactElement => {
  const { t } = useTranslation()
  const [mode, setMode] = React.useState<BattlepassResetMode>('demote')
  const [accountId, setAccountId] = React.useState(0)
  const [confirming, setConfirming] = React.useState(false)
  const [busy, setBusy] = React.useState(false)

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
        <NumberInput
          label={t('battlepass.reset.accountId')}
          min={0}
          max={999999999}
          value={accountId}
          onChange={setAccountId}
          className="w-56"
        />
        <Button size="sm" variant="danger-soft" isDisabled={busy} onPress={() => setConfirming(true)}>
          {t('battlepass.reset.run')}
        </Button>
      </div>
      <p className="text-xs text-muted mt-2">
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
