import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Modal, Spinner, toast } from '@heroui/react'
import { api } from '../../../api/client'
import { Dropzone, Icon } from '../../../dune-ui'
import type { RestoreModalProps } from './types'

export const RestoreModal: React.FC<RestoreModalProps> = ({
  open, backupFiles, backupFilesLoading, setBackupFiles, onClose, onRestoreComplete,
}) => {
  const { t } = useTranslation()
  const [selectedFile, setSelectedFile] = React.useState('')
  const [restoreRunning, setRestoreRunning] = React.useState(false)
  const [uploading, setUploading] = React.useState(false)

  const uploadFile = async (file: File) => {
    setUploading(true)
    try {
      const res = await api.battlegroup.backupUpload(file)
      toast.success(t('battlegroup.restore.uploaded', { name: res.name }))
      const updated = await api.battlegroup.backupFiles()
      setBackupFiles(updated)
      setSelectedFile(res.name)
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
    finally {
      setUploading(false)
    }
  }

  return (
    <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={open} onOpenChange={(v) => { if (!v && !restoreRunning) onClose() }}>
      <Modal.Container>
        <Modal.Dialog className="p-10 w-[640px] max-w-[90vw]">
          <Modal.CloseTrigger />
          <Modal.Header><Modal.Heading>{t('battlegroup.restore.title')}</Modal.Heading></Modal.Header>
          <Modal.Body>
            <p className="text-sm mb-3 text-danger flex items-center gap-1.5">
              <Icon name="triangle-alert" />
              {' '}
              {t('battlegroup.restore.warning')}
            </p>

            <div className="mb-3">
              <Dropzone
                accept=".backup,.zip"
                uploading={uploading}
                onSelect={uploadFile}
                prompt={t('battlegroup.restore.dropzone')}
              />
            </div>

            {backupFilesLoading
              ? (
                  <div className="flex justify-center py-4"><Spinner /></div>
                )
              : backupFiles.length === 0
                ? (
                    <p className="text-sm text-muted">{t('battlegroup.restore.noBackups')}</p>
                  )
                : (
                    <div className="flex flex-col gap-1">
                      {backupFiles.map((f) => {
                        const isSelected = selectedFile === f.name
                        return (
                          <label
                            key={f.name}
                            className={
                              'flex items-center gap-3 rounded-md px-3 py-2 cursor-pointer border '
                              + (isSelected
                                ? 'bg-success/10 border-success/40'
                                : 'bg-background border-border hover:border-warning/60')
                            }
                          >
                            <input
                              type="radio"
                              name="restore-file"
                              value={f.name}
                              checked={isSelected}
                              onChange={() => setSelectedFile(f.name)}
                            />
                            <div className="flex-1 min-w-0">
                              <div className="text-xs font-mono">{f.name}</div>
                              <div className="text-xs flex items-center gap-2 text-muted">
                                <span>
                                  {(f.size_bytes / 1024 / 1024).toFixed(1)}
                                  {' '}
                                  MB ·
                                  {' '}
                                  {f.modified}
                                </span>
                                {f.has_yaml && (
                                  <span className="px-1 rounded bg-success/10 text-success text-[10px] border border-success/30">+yaml</span>
                                )}
                              </div>
                            </div>
                            <a
                              href={api.battlegroup.backupDownloadUrl(f.name)}
                              download={f.name.replace('.backup', '.zip')}
                              onClick={(e) => e.stopPropagation()}
                              className="text-xs px-2 py-1 rounded bg-accent/10 text-accent border border-accent/30 no-underline hover:bg-accent/20"
                              aria-label="Download"
                            >
                              <Icon name="download" />
                            </a>
                          </label>
                        )
                      })}
                    </div>
                  )}
          </Modal.Body>
          <Modal.Footer>
            <Button variant="tertiary" onPress={onClose} isDisabled={restoreRunning}>{t('common.cancel')}</Button>
            <Button
              variant="danger"
              isDisabled={!selectedFile || restoreRunning || backupFilesLoading}
              onPress={async () => {
                setRestoreRunning(true)
                try {
                  const res = await api.battlegroup.restore(selectedFile)
                  toast.success(t('battlegroup.restore.restoreCompleted'))
                  onRestoreComplete(res.output || '(done)')
                }
                catch (e: unknown) {
                  toast.danger(e instanceof Error ? e.message : String(e))
                }
                finally {
                  setRestoreRunning(false)
                }
              }}
            >
              {restoreRunning
                ? <Spinner size="sm" color="current" />
                : t('battlegroup.restore.restoreBtn', { file: selectedFile ? selectedFile.slice(-20) : '' })}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
