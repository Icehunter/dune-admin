import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { AlertDialog, Button, Checkbox, Input, TextArea } from '@heroui/react'
import { BackupScopeCard } from './BackupScopeCard'
import type { DeleteCharacterModalProps } from './interfaces'

const COUNTDOWN_SECONDS = 10

export const DeleteCharacterModal: React.FC<DeleteCharacterModalProps> = ({
  open,
  playerName,
  online,
  busy,
  onCancel,
  onConfirm,
}) => {
  const { t } = useTranslation()
  const [remaining, setRemaining] = React.useState(COUNTDOWN_SECONDS)
  const [typed, setTyped] = React.useState('')
  const [reason, setReason] = React.useState('')
  const [backup, setBackup] = React.useState(true)

  const phrase = `${t('players.actions.admin.deleteCharacterPhrase')} ${playerName}`

  // Reset countdown and inputs whenever the modal opens, then tick down.
  // The backup export requires the player OFFLINE, so it can't be requested
  // while they're online — default it off in that case.
  React.useEffect(() => {
    if (!open) return
    void Promise.resolve().then(() => {
      setRemaining(COUNTDOWN_SECONDS)
      setTyped('')
      setReason('')
      setBackup(!online)
    })
    const id = setInterval(() => {
      setRemaining((r) => (r <= 1 ? 0 : r - 1))
    }, 1000)
    return () => clearInterval(id)
  }, [open, online])

  const phraseMatches = typed.trim() === phrase
  const reasonValid = reason.trim().length > 0
  const canDelete = remaining === 0 && phraseMatches && reasonValid && !busy

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
            <AlertDialog.Heading>{t('players.actions.admin.deleteCharacterTitle')}</AlertDialog.Heading>
          </AlertDialog.Header>
          <AlertDialog.Body>
            <div className="flex flex-col gap-3">
              <div className="text-sm text-muted">
                {t('players.actions.admin.deleteCharacterWarning', { player: playerName })}
              </div>
              {online && (
                <div className="text-xs text-warning">
                  {t('players.actions.admin.deleteCharacterOnlineWarning')}
                </div>
              )}
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted">{t('players.actions.admin.deleteCharacterReasonLabel')}</span>
                <TextArea
                  aria-label={t('players.actions.admin.deleteCharacterReasonLabel')}
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  placeholder={t('players.actions.admin.deleteCharacterReasonPlaceholder')}
                  rows={2}
                  maxLength={500}
                  fullWidth
                  style={{ resize: 'vertical' }}
                />
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted">
                  {t('players.actions.admin.deleteCharacterConfirmLabel', { phrase })}
                </span>
                <Input
                  aria-label={t('players.actions.admin.deleteCharacterConfirmLabel', { phrase })}
                  value={typed}
                  onChange={(e) => setTyped(e.target.value)}
                  placeholder={phrase}
                  autoComplete="off"
                  className="font-mono"
                />
              </div>
              <Checkbox isSelected={backup && !online} isDisabled={online} onChange={setBackup}>
                <Checkbox.Content className="w-full max-w-none gap-2 py-1.5 px-1 rounded-[var(--radius)] cursor-pointer">
                  <Checkbox.Control><Checkbox.Indicator /></Checkbox.Control>
                  <span className="flex-1 text-xs text-foreground">
                    {t('players.actions.admin.deleteCharacterBackupLabel')}
                  </span>
                </Checkbox.Content>
              </Checkbox>
              {online && (
                <div className="text-xs text-muted -mt-2">
                  {t('players.actions.admin.deleteCharacterBackupOfflineNote')}
                </div>
              )}
              {backup && !online && (
                <div className="-mt-1"><BackupScopeCard /></div>
              )}
            </div>
          </AlertDialog.Body>
          <AlertDialog.Footer>
            <Button slot="close" variant="ghost" onPress={onCancel}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger-soft"
              isDisabled={!canDelete}
              onPress={() => onConfirm(reason.trim(), backup)}
            >
              {remaining > 0
                ? t('players.actions.admin.deleteCharacterCountdown', { seconds: remaining })
                : t('players.actions.admin.deleteCharacterConfirm')}
            </Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  )
}
