import * as React from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button,
  Label,
  Modal,
  Spinner,
  TextField,
  toast,
} from '@heroui/react'
import { api } from '../../api/client'
import type { Player } from '../../api/client'
import { Dropzone, Icon } from '../../dune-ui'
import { PlayerSearchField } from '../../components/PlayerSearchField'
import type { ImportModalProps } from '../interfaces'

export const ImportModal: React.FC<ImportModalProps> = ({ open, onClose, onSuccess }) => {
  const { t } = useTranslation()
  const [file, setFile] = React.useState<File | null>(null)
  const [selectedPlayer, setSelectedPlayer] = React.useState<Player | null>(null)
  const [submitting, setSubmitting] = React.useState(false)

  React.useEffect(() => {
    if (!open) return
    Promise.resolve().then(() => {
      setFile(null)
      setSelectedPlayer(null)
    })
  }, [open])

  const handleSubmit = async () => {
    if (!file) {
      toast.warning(t('blueprints.selectFile'))
      return
    }
    if (!selectedPlayer) {
      toast.warning(t('blueprints.selectPlayer'))
      return
    }
    setSubmitting(true)
    try {
      const res = await api.blueprints.import(file, selectedPlayer.id)
      if (res.ok) {
        toast.success(t('blueprints.importSuccess'))
        onSuccess()
      }
      else {
        toast.danger(t('blueprints.importFailed', { message: res.error ?? 'unknown error' }))
      }
    }
    catch (e: unknown) {
      toast.danger(t('blueprints.importFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={open} onOpenChange={(v) => !v && onClose()}>
      <Modal.Container>
        <Modal.Dialog className="p-10 !overflow-visible">
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading className="text-accent">{t('blueprints.importModal.title')}</Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-4">
            <TextField>
              <Label>{t('blueprints.importModal.blueprintFile')}</Label>
              <Dropzone
                accept=".json"
                file={file}
                onSelect={setFile}
                prompt={t('blueprints.importModal.dropzone')}
              />
            </TextField>

            <TextField>
              <Label>{t('blueprints.importModal.playerLabel')}</Label>
              <PlayerSearchField
                ariaLabel={t('blueprints.importModal.playerLabel')}
                placeholder={t('blueprints.importModal.playerPlaceholder')}
                onSelect={setSelectedPlayer}
                onClear={() => setSelectedPlayer(null)}
                className="w-full"
              />
            </TextField>
          </Modal.Body>
          <Modal.Footer>
            <Button variant="tertiary" slot="close">
              {t('common.cancel')}
            </Button>
            <Button onPress={handleSubmit} isDisabled={submitting || !file || !selectedPlayer}>
              {submitting ? <Spinner size="sm" color="current" /> : <Icon name="upload" />}
              {t('blueprints.importModal.import')}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
