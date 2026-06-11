import type React from 'react'
import { useTranslation } from 'react-i18next'
import { AlertDialog, Button } from '@heroui/react'

type ConfirmDialogProps = {
  open: boolean
  title: string
  description: string
  confirmLabel?: string
  onConfirm: () => void
  onCancel: () => void
}

export const ConfirmDialog: React.FC<ConfirmDialogProps> = ({
  open,
  title,
  description,
  confirmLabel,
  onConfirm,
  onCancel,
}) => {
  const { t } = useTranslation()
  return (
    <AlertDialog.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={open} onOpenChange={(v) => !v && onCancel()}>
      <AlertDialog.Container size="sm">
        <AlertDialog.Dialog>
          <AlertDialog.Header>
            <AlertDialog.Icon status="danger" />
            <AlertDialog.Heading>{title}</AlertDialog.Heading>
          </AlertDialog.Header>
          <AlertDialog.Body>
            <p className="text-sm text-muted">{description}</p>
          </AlertDialog.Body>
          <AlertDialog.Footer>
            <Button slot="close" variant="ghost" onPress={onCancel}>{t('common.cancel')}</Button>
            <Button slot="close" variant="danger-soft" onPress={onConfirm}>{confirmLabel ?? t('common.confirm')}</Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  )
}
