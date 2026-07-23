import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { AlertDialog, Button } from '@heroui/react'
import type { PartitionRestartConfirmProps } from './types'

export const PartitionRestartConfirm: React.FC<PartitionRestartConfirmProps> = ({ server, onConfirm, onClose }) => {
  const { t } = useTranslation()
  return (
    <AlertDialog.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={server !== null} onOpenChange={(v) => { if (!v) onClose() }}>
      <AlertDialog.Container size="sm">
        <AlertDialog.Dialog className="p-10">
          <AlertDialog.Header>
            <AlertDialog.Icon status="danger" />
            <AlertDialog.Heading>
              {t('battlegroup.restartPartition.title', { map: server?.map ?? '' })}
            </AlertDialog.Heading>
          </AlertDialog.Header>
          <AlertDialog.Body>
            <p className="text-sm text-muted">
              {t('battlegroup.restartPartition.body', { map: server?.map ?? '', partition: server?.partition ?? 0 })}
            </p>
          </AlertDialog.Body>
          <AlertDialog.Footer>
            <Button slot="close" variant="ghost" onPress={onClose}>{t('common.cancel')}</Button>
            <Button
              slot="close"
              variant="danger-soft"
              onPress={() => server && onConfirm(server)}
            >
              {t('battlegroup.restartPartition.confirm')}
            </Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  )
}
