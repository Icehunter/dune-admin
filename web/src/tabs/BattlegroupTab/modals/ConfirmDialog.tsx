import { Button, Modal } from '@heroui/react'
import type { ActionDef } from '../types'

type Props = {
  action: ActionDef | null
  onConfirm: (a: ActionDef) => void
  onClose: () => void
}

export function ConfirmDialog({ action, onConfirm, onClose }: Props) {
  return (
    <Modal>
      <Modal.Backdrop isOpen={action !== null} onOpenChange={(v) => { if (!v) onClose() }}>
        <Modal.Container>
          <Modal.Dialog>
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading>
                {action?.label ?? ''}
                {' '}
                Server
              </Modal.Heading>
            </Modal.Header>
            <Modal.Body>
              <p className="text-foreground">{action?.msg ?? ''}</p>
            </Modal.Body>
            <Modal.Footer>
              <Button variant="tertiary" slot="close">Cancel</Button>
              <Button
                variant={action?.danger ? 'danger' : 'primary'}
                onPress={() => action && onConfirm(action)}
              >
                Confirm
                {' '}
                {action?.label ?? ''}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
