import { useTranslation } from 'react-i18next'
import { Button, Modal } from '@heroui/react'
import type { ActionDef } from '../types'

type Props = {
  action: ActionDef | null
  onConfirm: (a: ActionDef) => void
  onClose: () => void
}

export function ConfirmDialog({ action, onConfirm, onClose }: Props) {
  const { t } = useTranslation()
  return (
    <Modal>
      <Modal.Backdrop isOpen={action !== null} onOpenChange={(v) => { if (!v) onClose() }}>
        <Modal.Container>
          <Modal.Dialog>
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading>
                {action ? t(`battlegroup.actions.${action.cmd}` as never) : ''}
                {' '}
                {t('battlegroup.confirm.serverSuffix')}
              </Modal.Heading>
            </Modal.Header>
            <Modal.Body>
              <p className="text-foreground">{action ? t(`battlegroup.actions.${action.cmd}Msg` as never) : ''}</p>
            </Modal.Body>
            <Modal.Footer>
              <Button variant="tertiary" slot="close">{t('common.cancel')}</Button>
              <Button
                variant={action?.danger ? 'danger' : 'primary'}
                onPress={() => action && onConfirm(action)}
              >
                {t('battlegroup.confirm.confirmPrefix')}
                {' '}
                {action ? t(`battlegroup.actions.${action.cmd}` as never) : ''}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
