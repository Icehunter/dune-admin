import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Modal, Spinner, toast } from '@heroui/react'
import { Icon } from '../dune-ui'
import { ServerSettingsForm } from './settings/server/ServerSettingsForm'
import { DeleteServerModal } from './DeleteServerModal'
import { useActiveServer } from '../context/useActiveServer'

export interface ManageServerModalProps {
  open: boolean
  serverId: number
  /** Whether the session may delete/control servers. */
  canControl: boolean
  onClose: () => void
  /** Called after the server is deleted (so the app can refresh status). */
  onDeleted?: () => void
}

// Per-server settings as a modal overlay (rename + control/SSH/DB/broker/advanced
// + delete). Keyed by serverId in state — not a route — so the URL never carries
// a server id that would look stale after a rename.
export const ManageServerModal: React.FC<ManageServerModalProps> = ({
  open, serverId, canControl, onClose, onDeleted,
}) => {
  const { t } = useTranslation()
  const { servers, removeServer } = useActiveServer()
  const saveRef = React.useRef<(() => Promise<void>) | null>(null)
  const [saving, setSaving] = React.useState(false)
  const [deleteOpen, setDeleteOpen] = React.useState(false)
  const [deleting, setDeleting] = React.useState(false)

  const serverName = servers.find((s) => s.id === serverId)?.name ?? String(serverId)

  const handleSave = () => {
    void saveRef.current?.().then(() => onClose())
  }

  return (
    <>
      <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={open} onOpenChange={(v) => !v && onClose()}>
        <Modal.Container size="cover" scroll="outside">
          <Modal.Dialog className="p-10 dialog-surface-alt">
            <Modal.CloseTrigger />
            <Modal.Header>
              <div className="flex items-baseline gap-3 flex-wrap">
                <Modal.Heading className="text-accent">{t('manage.title', 'Manage server')}</Modal.Heading>
                <span className="text-sm font-semibold text-foreground">{serverName}</span>
              </div>
            </Modal.Header>
            <Modal.Body className="flex flex-col overflow-y-auto h-[80vh] min-h-0 pr-1">
              {open && (
                <ServerSettingsForm
                  key={serverId}
                  serverId={serverId}
                  saveRef={saveRef}
                  onSavingChange={setSaving}
                  onRequestDeleteServer={canControl ? () => setDeleteOpen(true) : undefined}
                />
              )}
            </Modal.Body>
            <Modal.Footer className="flex items-center gap-2">
              <span className="flex-1" />
              <Button size="sm" onPress={handleSave} isDisabled={saving}>
                {saving
                  ? (
                      <>
                        <Spinner size="sm" color="current" />
                        {' '}
                        {t('common.saving')}
                      </>
                    )
                  : (
                      <>
                        <Icon name="save" />
                        {' '}
                        {t('app.saveApply')}
                      </>
                    )}
              </Button>
              <Button size="sm" variant="tertiary" onPress={onClose}>
                {t('common.close')}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>

      <DeleteServerModal
        open={deleteOpen}
        serverName={serverName}
        busy={deleting}
        onConfirm={() => {
          setDeleting(true)
          removeServer(serverId)
            .then(() => {
              setDeleteOpen(false)
              onClose()
              onDeleted?.()
            })
            .catch((e: unknown) => {
              toast.danger(`${t('servers.removeFailed', 'Remove failed')}: ${e instanceof Error ? e.message : String(e)}`)
            })
            .finally(() => setDeleting(false))
        }}
        onCancel={() => setDeleteOpen(false)}
      />
    </>
  )
}
