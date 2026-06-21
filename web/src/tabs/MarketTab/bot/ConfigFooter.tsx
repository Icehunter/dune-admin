import * as React from 'react'
import { Button, Spinner, Switch } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../../../dune-ui'
import type { ConfigFooterProps } from './interfaces'

export const ConfigFooter: React.FC<ConfigFooterProps> = ({
  editorRef, initialEnabled, onReload,
}: ConfigFooterProps) => {
  const { t } = useTranslation()
  const [saving, setSaving] = React.useState(false)
  const [reloading, setReloading] = React.useState(false)
  const [enabled, setEnabledLocal] = React.useState(initialEnabled)

  return (
    <div className="shrink-0 flex items-center gap-3 px-4 py-3">
      <Switch
        isSelected={enabled}
        onChange={(v) => {
          setEnabledLocal(v)
          editorRef.current?.setEnabled(v)
        }}
        size="sm"
        className="mr-auto"
      >
        <Switch.Control><Switch.Thumb /></Switch.Control>
        <Switch.Content>{t('market.bot.tickingEnabled')}</Switch.Content>
      </Switch>
      <Button size="sm" variant="ghost" onPress={() => editorRef.current?.reset()}>
        {t('market.bot.reset')}
      </Button>
      <Button
        size="sm"
        variant="ghost"
        isDisabled={reloading}
        onPress={() => {
          setReloading(true)
          Promise.resolve().then(onReload).finally(() => setReloading(false))
        }}
      >
        {reloading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
        {t('market.bot.reloadConfig')}
      </Button>
      <Button
        size="sm"
        isDisabled={saving}
        onPress={() => {
          setSaving(true)
          editorRef.current?.save()
            .catch(() => { /* toast shown inside save */ })
            .finally(() => setSaving(false))
        }}
      >
        {saving ? <Spinner size="sm" color="current" /> : null}
        {t('market.bot.saveConfig')}
      </Button>
    </div>
  )
}
