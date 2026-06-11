import type React from 'react'
import { useTranslation } from 'react-i18next'
import { AlertDialog, Button } from '@heroui/react'
import type { ActionDef } from '../types'

type ConfirmDialogProps = {
  action: ActionDef | null
  onConfirm: (a: ActionDef) => void
  onClose: () => void
}

export const ConfirmDialog: React.FC<ConfirmDialogProps> = ({ action, onConfirm, onClose }) => {
  const { t } = useTranslation()
  return (
    <AlertDialog.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={action !== null} onOpenChange={(v) => { if (!v) onClose() }}>
      <AlertDialog.Container size="sm">
        <AlertDialog.Dialog>
          <AlertDialog.Header>
            <AlertDialog.Icon status={action?.danger ? 'danger' : 'accent'} />
            <AlertDialog.Heading>
              {action ? t(`battlegroup.actions.${action.cmd}` as never) : ''}
              {' '}
              {t('battlegroup.confirm.serverSuffix')}
            </AlertDialog.Heading>
          </AlertDialog.Header>
          <AlertDialog.Body>
            <p className="text-sm text-muted">
              {action ? t(`battlegroup.actions.${action.cmd}Msg` as never) : ''}
            </p>
          </AlertDialog.Body>
          <AlertDialog.Footer>
            <Button slot="close" variant="ghost" onPress={onClose}>{t('common.cancel')}</Button>
            <Button
              slot="close"
              variant={action?.danger ? 'danger-soft' : 'primary'}
              onPress={() => action && onConfirm(action)}
            >
              {t('battlegroup.confirm.confirmPrefix')}
              {' '}
              {action ? t(`battlegroup.actions.${action.cmd}` as never) : ''}
            </Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  )
}
