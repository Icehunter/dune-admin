import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { AlertDialog, Button, Input } from '@heroui/react'
import type { DeleteServerModalProps } from './interfaces'

const COUNTDOWN_SECONDS = 10

// Destructive confirmation for removing a server — mirrors DeleteCharacterModal:
// a 10-second countdown plus a type-the-name confirmation, because removing a
// server purges all of its stored data.
export const DeleteServerModal: React.FC<DeleteServerModalProps> = ({
  open,
  serverName,
  busy,
  onCancel,
  onConfirm,
}) => {
  const { t } = useTranslation()
  const [remaining, setRemaining] = React.useState(COUNTDOWN_SECONDS)
  const [typed, setTyped] = React.useState('')

  React.useEffect(() => {
    if (!open) return
    void Promise.resolve().then(() => {
      setRemaining(COUNTDOWN_SECONDS)
      setTyped('')
    })
    const id = setInterval(() => {
      setRemaining((r) => (r <= 1 ? 0 : r - 1))
    }, 1000)
    return () => clearInterval(id)
  }, [open])

  const nameMatches = typed.trim() === serverName.trim()
  const canDelete = remaining === 0 && nameMatches && !busy

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
            <AlertDialog.Heading>{t('servers.removeTitle', 'Remove Server')}</AlertDialog.Heading>
          </AlertDialog.Header>
          <AlertDialog.Body>
            <div className="flex flex-col gap-3">
              <div className="text-sm text-muted">
                {t('servers.removeWarning', 'Permanently remove "{{name}}" and all of its stored data. This cannot be undone.', { name: serverName })}
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted">
                  {t('servers.removeConfirmLabel', 'Type {{name}} to confirm', { name: serverName })}
                </span>
                <Input
                  aria-label={t('servers.removeConfirmLabel', 'Type {{name}} to confirm', { name: serverName })}
                  value={typed}
                  onChange={(e) => setTyped(e.target.value)}
                  placeholder={serverName}
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
            <Button variant="danger-soft" isDisabled={!canDelete} onPress={onConfirm}>
              {remaining > 0
                ? t('servers.removeCountdown', 'Wait {{seconds}}s…', { seconds: remaining })
                : t('servers.removeConfirm', 'Remove')}
            </Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  )
}
