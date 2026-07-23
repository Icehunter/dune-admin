import * as React from 'react'
import { Button, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../api/client'
import { ConfirmDialog, Icon } from '../../dune-ui'
import type { DisableItemActionProps } from './types'

export const DisableItemAction: React.FC<DisableItemActionProps> = (
  { item, botConfig, canManage, onDisabled, variant }: DisableItemActionProps,
) => {
  const { t } = useTranslation()
  const [confirmOpen, setConfirmOpen] = React.useState(false)
  const [saving, setSaving] = React.useState(false)

  if (!canManage || !botConfig) return null

  const name = item.display_name || item.template_id
  const alreadyDisabled = botConfig.disabled_items.includes(item.template_id)

  if (alreadyDisabled) {
    return (
      <span className="text-xs text-muted whitespace-nowrap">
        {t('market.disable.alreadyDisabled')}
      </span>
    )
  }

  const handleConfirm = async (): Promise<void> => {
    setSaving(true)
    try {
      const saved = await api.marketBot.saveConfig({
        ...botConfig,
        disabled_items: [...botConfig.disabled_items, item.template_id],
      })
      onDisabled(saved)
      toast.success(t('market.disable.success', { name }))
    }
    catch (e: unknown) {
      toast.danger(t('common.failed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setSaving(false)
      setConfirmOpen(false)
    }
  }

  return (
    <React.Fragment>
      {variant === 'icon'
        ? (
            <Button
              size="sm"
              variant="danger-soft"
              isIconOnly
              aria-label={t('market.disable.ariaLabel', { name })}
              isDisabled={saving}
              onPress={() => setConfirmOpen(true)}
            >
              <Icon name="ban" />
            </Button>
          )
        : (
            <Button
              size="sm"
              variant="danger-soft"
              isDisabled={saving}
              onPress={() => setConfirmOpen(true)}
            >
              {t('market.disable.button')}
            </Button>
          )}
      <ConfirmDialog
        open={confirmOpen}
        title={t('market.disable.confirmTitle')}
        description={t('market.disable.confirmBody', { name })}
        confirmLabel={t('market.disable.confirmLabel')}
        onConfirm={() => { void handleConfirm() }}
        onCancel={() => setConfirmOpen(false)}
      />
    </React.Fragment>
  )
}
