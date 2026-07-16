import * as React from 'react'
import { Button, Modal, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import { NumberInput } from '../../../dune-ui'
import type { EditItemModalProps } from './interfaces'

export const EditItemModal: React.FC<EditItemModalProps> = ({ item, onClose, onSaved }) => {
  const { t } = useTranslation()
  const [stackSize, setStackSize] = React.useState(1)
  const [quality, setQuality] = React.useState(0)
  const [saving, setSaving] = React.useState(false)

  React.useEffect(() => {
    if (!item) return
    Promise.resolve().then(() => {
      setStackSize(item.stack_size)
      setQuality(item.quality)
    })
  }, [item])

  const handleSave = async (): Promise<void> => {
    if (!item) return
    setSaving(true)
    try {
      await api.players.updateItem(item.id, stackSize, quality)
      onSaved({ ...item, stack_size: stackSize, quality })
      toast.success(t('players.inventory.itemUpdated'))
      onClose()
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
    finally {
      setSaving(false)
    }
  }

  return (
    <Modal.Backdrop
      variant="blur"
      className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent"
      isOpen={item !== null}
      onOpenChange={(v) => !v && onClose()}
    >
      <Modal.Container size="sm">
        <Modal.Dialog className="p-10">
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading className="text-accent">
              {item ? t('players.inventory.editItem', { name: item.name || item.template_id }) : ''}
            </Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-4">
            <NumberInput
              label={t('players.inventory.columns.stack')}
              value={stackSize}
              onChange={setStackSize}
              min={1}
              isDisabled={saving}
            />
            <NumberInput
              label={t('players.inventory.columns.quality')}
              value={quality}
              onChange={setQuality}
              min={0}
              isDisabled={saving}
            />
          </Modal.Body>
          <Modal.Footer className="flex justify-end gap-2">
            <Button size="sm" variant="ghost" onPress={onClose} isDisabled={saving}>
              {t('common.cancel')}
            </Button>
            <Button size="sm" variant="primary" onPress={handleSave} isDisabled={saving}>
              {t('common.save')}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
