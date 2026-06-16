import * as React from 'react'
import { Button, Modal } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { useAtom, useAtomValue } from 'jotai'
import { Icon } from '../../dune-ui'
import {
  updateApplyingAtom,
  updateInfoAtom,
  updatePromptOpenAtom,
} from '../../atoms/app'
import { useAppUpdate } from './useAppUpdate'

interface UpdatePromptModalProps {
  can: (cap: string) => boolean
}

export const UpdatePromptModal: React.FC<UpdatePromptModalProps> = ({ can }) => {
  const { t } = useTranslation()
  const [updatePromptOpen, setUpdatePromptOpen] = useAtom(updatePromptOpenAtom)
  const updateInfo = useAtomValue(updateInfoAtom)
  const updateApplying = useAtomValue(updateApplyingAtom)
  const { applyUpdate } = useAppUpdate()

  return (
    <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={updatePromptOpen} onOpenChange={(v) => !v && setUpdatePromptOpen(false)}>
      <Modal.Container size="sm">
        <Modal.Dialog className="p-10">
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading className="text-accent">{t('app.updateAvailable')}</Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-3">
            <p className="text-sm text-muted">
              {t('app.updateAvailableBody', {
                current: updateInfo?.current ?? '',
                latest: updateInfo?.latest?.replace(/^v/, '') ?? '',
              })}
            </p>
            {updateInfo?.release_url && (
              <a
                href={updateInfo.release_url}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 text-sm text-accent hover:opacity-80"
              >
                <Icon name="external-link" />
                {' '}
                {t('app.viewReleaseNotes')}
              </a>
            )}
          </Modal.Body>
          <Modal.Footer className="flex items-center justify-end gap-2">
            <Button
              size="sm"
              variant="tertiary"
              onPress={() => setUpdatePromptOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            {can('server:control') && (
              <Button
                size="sm"
                onPress={() => {
                  void applyUpdate()
                }}
                isDisabled={updateApplying}
              >
                {t('app.updateNow')}
              </Button>
            )}
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
