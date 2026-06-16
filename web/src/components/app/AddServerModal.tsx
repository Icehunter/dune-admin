import * as React from 'react'
import { Modal } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { useAtom } from 'jotai'
import { SetupWizard } from '../SetupWizard'
import { addServerOpenAtom } from '../../atoms/app'

interface AddServerModalProps {
  onDone: () => void
}

export const AddServerModal: React.FC<AddServerModalProps> = ({ onDone }) => {
  const { t } = useTranslation()
  const [addServerOpen, setAddServerOpen] = useAtom(addServerOpenAtom)

  return (
    <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={addServerOpen} onOpenChange={(v) => !v && setAddServerOpen(false)}>
      <Modal.Container size="cover" scroll="outside">
        <Modal.Dialog className="p-10 dialog-surface-alt">
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading className="text-accent">{t('setup.addServerTitle', 'Add a server')}</Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col overflow-y-auto h-[80vh] min-h-0 pr-1">
            {addServerOpen && (
              <SetupWizard
                onDone={onDone}
              />
            )}
          </Modal.Body>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
