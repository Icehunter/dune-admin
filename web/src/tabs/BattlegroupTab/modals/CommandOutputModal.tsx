import type React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Modal, Spinner } from '@heroui/react'
import { api } from '../../../api/client'
import { Icon } from '../../../dune-ui'

type CommandOutputModalProps = {
  runningCmd: string | null
  cmdOutput: string | null
  cmdDone: boolean
  lastBackupFile: string | null
  onClose: () => void
}

export const CommandOutputModal: React.FC<CommandOutputModalProps> = ({
  runningCmd, cmdOutput, cmdDone, lastBackupFile, onClose,
}) => {
  const { t } = useTranslation()
  return (
    <Modal.Backdrop isOpen={runningCmd !== null} onOpenChange={(v) => { if (!v && cmdDone) onClose() }}>
      <Modal.Container>
        <Modal.Dialog>
          <Modal.Header><Modal.Heading>{runningCmd ? t(`battlegroup.actions.${runningCmd}` as never) : ''}</Modal.Heading></Modal.Header>
          <Modal.Body>
            {!cmdDone
              ? (
                  <div className="flex flex-col items-center gap-4 py-6">
                    <Spinner size="lg" />
                    <p className="text-sm text-muted">
                      {t('battlegroup.runningCmd', { cmd: runningCmd?.toLowerCase() ?? '' })}
                    </p>
                  </div>
                )
              : (
                  <div className="rounded-[var(--radius)] p-3 font-mono text-xs overflow-auto max-h-60 bg-background border border-border text-success">
                    <pre className="m-0 whitespace-pre-wrap">{cmdOutput}</pre>
                  </div>
                )}
          </Modal.Body>
          {cmdDone && (
            <Modal.Footer>
              {lastBackupFile && runningCmd === 'backup' && (
                <a
                  href={api.battlegroup.backupDownloadUrl(lastBackupFile)}
                  download={lastBackupFile.replace('.backup', '.zip')}
                  className="text-sm px-3 py-1.5 rounded-[var(--radius)] inline-flex items-center gap-1.5 bg-success/10 text-success border border-success/40 no-underline hover:bg-success/20"
                >
                  <Icon name="download" />
                  {' '}
                  {t('battlegroup.modal.download')}
                </a>
              )}
              <Button onPress={onClose}>{t('common.close')}</Button>
            </Modal.Footer>
          )}
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
