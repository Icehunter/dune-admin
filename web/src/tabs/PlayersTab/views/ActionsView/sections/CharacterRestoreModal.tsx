import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { AlertDialog, Button } from '@heroui/react'
import { BackupScopeCard } from './BackupScopeCard'
import type { CharacterRestoreModalProps } from './interfaces'

const fmtDate = (iso: string): string => new Date(iso).toLocaleString()

// CharacterRestoreModal is the confirm dialog for restoring a character from
// a transfer backup. It replaces a generic confirm so the operator sees the
// full consequences in one place: the restore is a complete replacement of
// the character's current data, requires the player offline and an unchanged
// game patch, and only brings back what the backup contains — live property
// currently in the world is disowned by the restore, not recovered.
export const CharacterRestoreModal: React.FC<CharacterRestoreModalProps> = ({
  backup,
  playerName,
  busy,
  onCancel,
  onConfirm,
}) => {
  const { t } = useTranslation()
  const open = backup !== null

  const renderBackupLine = (): React.ReactNode => {
    if (!backup) return null
    return (
      <div className="text-xs text-muted font-mono">
        {fmtDate(backup.created_at)}
        {' — '}
        {backup.action}
        {backup.reason ? ` — ${backup.reason}` : ''}
      </div>
    )
  }

  return (
    <AlertDialog.Backdrop
      variant="blur"
      className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent"
      isOpen={open}
      onOpenChange={(v) => !v && onCancel()}
    >
      <AlertDialog.Container size="md">
        <AlertDialog.Dialog className="p-10">
          <AlertDialog.Header>
            <AlertDialog.Icon status="danger" />
            <AlertDialog.Heading>{t('players.actions.admin.backupsRestoreTitle')}</AlertDialog.Heading>
          </AlertDialog.Header>
          <AlertDialog.Body>
            <div className="flex flex-col gap-3">
              <div className="text-sm text-muted">
                {t('players.actions.admin.backupsRestoreConfirmDesc', { player: backup?.character_name || playerName })}
              </div>
              {renderBackupLine()}
              <BackupScopeCard />
              <div className="text-xs text-danger">
                {t('players.actions.admin.backupsRestoreScopeWarning')}
              </div>
            </div>
          </AlertDialog.Body>
          <AlertDialog.Footer>
            <Button slot="close" variant="ghost" onPress={onCancel}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger-soft"
              isDisabled={busy || !backup}
              onPress={() => {
                if (backup) onConfirm(backup)
              }}
            >
              {t('players.actions.admin.backupsRestore')}
            </Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  )
}
