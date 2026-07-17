import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../../../../../dune-ui'

// BackupScopeCard spells out exactly what a character backup does and does
// not contain, wherever a backup or restore decision is being made. The
// backup rides on the game's native transfer format, which only carries
// property packed with the in-game base/vehicle backup tools — live placed
// structures are disowned (not deleted) by a restore's internal account
// replacement, so this distinction is the difference between "my base came
// back" and "everything is gone".
export const BackupScopeCard: React.FC = (): React.ReactElement => {
  const { t } = useTranslation()

  const renderRow = (icon: 'check' | 'x', label: string): React.ReactNode => (
    <div className="flex items-start gap-2 text-xs text-muted">
      <span className="w-3.5 h-3.5 flex items-center justify-center shrink-0 mt-0.5">
        {icon === 'check'
          ? <Icon name="check" className="size-3.5 text-success" />
          : <Icon name="x" className="size-3.5 text-danger" />}
      </span>
      <span>{label}</span>
    </div>
  )

  return (
    <div className="rounded-md border border-border/60 bg-surface px-3 py-2.5 flex flex-col gap-1.5">
      <span className="text-xs font-medium text-foreground">
        {t('players.actions.admin.backupScope.includedTitle')}
      </span>
      {renderRow('check', t('players.actions.admin.backupScope.inclCharacter'))}
      {renderRow('check', t('players.actions.admin.backupScope.inclProgression'))}
      {renderRow('check', t('players.actions.admin.backupScope.inclBaseBackups'))}
      {renderRow('check', t('players.actions.admin.backupScope.inclVehicles'))}
      <span className="text-xs font-medium text-foreground mt-1.5">
        {t('players.actions.admin.backupScope.excludedTitle')}
      </span>
      {renderRow('x', t('players.actions.admin.backupScope.exclPlacedBases'))}
      {renderRow('x', t('players.actions.admin.backupScope.exclParkedVehicles'))}
      <p className="text-xs text-warning mt-1.5">
        {t('players.actions.admin.backupScope.hint')}
      </p>
    </div>
  )
}
