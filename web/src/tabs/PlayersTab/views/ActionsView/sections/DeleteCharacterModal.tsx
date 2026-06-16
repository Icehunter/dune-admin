import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { AlertDialog, Button, Input, TextArea } from '@heroui/react'

export interface DeleteCharacterModalProps {
  open: boolean
  playerName: string
  online: boolean
  busy: boolean
  onCancel: () => void
  onConfirm: (reason: string) => void
}

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

  const phrase = `${t('players.actions.admin.deleteCharacterPhrase')} ${playerName}`

  // Reset countdown and inputs whenever the modal opens, then tick down.
  React.useEffect(() => {
    if (!open) return
    void Promise.resolve().then(() => {
      setRemaining(COUNTDOWN_SECONDS)
      setTyped('')
      setReason('')
    })
    const id = setInterval(() => {
      setRemaining((r) => (r <= 1 ? 0 : r - 1))
    }, 1000)
    return () => clearInterval(id)
  }, [open])

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
            </div>
          </AlertDialog.Body>
          <AlertDialog.Footer>
            <Button slot="close" variant="ghost" onPress={onCancel}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger-soft"
              isDisabled={!canDelete}
              onPress={() => onConfirm(reason.trim())}
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
