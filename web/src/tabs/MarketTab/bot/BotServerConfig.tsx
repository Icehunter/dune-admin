import type React from 'react'
import { useState, useEffect } from 'react'
import { Button, Input, Spinner, Switch, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api, MASKED } from '../../../api/client'
import type { AppConfig } from '../../../api/client'
import { Panel, SectionLabel } from '../../../dune-ui'

// Restrict the set() helper to string-typed fields so it can't accidentally coerce
// numeric/boolean AppConfig keys to strings (which the backend would reject or misparse).
type StringAppConfigKey = { [K in keyof AppConfig]: AppConfig[K] extends string ? K : never }[keyof AppConfig]

export const BotServerConfig: React.FC = () => {
  const { t } = useTranslation()
  const [cfg, setCfg] = useState<AppConfig | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.config.get())
      .then(setCfg)
      .catch(() => toast.danger(t('market.bot.serverConfig.loadFailed')))
      .finally(() => setLoading(false))
  }, [t])

  const set = (key: StringAppConfigKey) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setCfg((prev) => prev ? { ...prev, [key]: e.target.value } : prev)

  const setBool = (key: keyof AppConfig) => (checked: boolean) =>
    setCfg((prev) => prev ? { ...prev, [key]: checked } : prev)

  const save = async () => {
    if (!cfg) return
    setSaving(true)
    try {
      // Sends the full AppConfig. The backend treats MASKED sentinel values as
      // "unchanged" for credential fields so they are never overwritten on save.
      await api.config.save(cfg)
      toast.success(t('market.bot.serverConfig.savedConfig'))
    }
    catch (e: unknown) {
      toast.danger(t('market.bot.serverConfig.saveFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setSaving(false)
    }
  }

  if (loading) {
    return <div className="flex justify-center py-8"><Spinner size="sm" /></div>
  }
  if (!cfg) {
    return <p className="text-xs text-muted">{t('market.bot.configUnavailable')}</p>
  }

  return (
    <div className="flex flex-col gap-4">
      <Panel>
        <SectionLabel>{t('market.bot.serverConfig.embeddedBot')}</SectionLabel>
        <div className="mt-2 flex items-center gap-2">
          <Switch
            isSelected={cfg.market_bot_enabled}
            onChange={setBool('market_bot_enabled')}
            size="sm"
          >
            <Switch.Control><Switch.Thumb /></Switch.Control>
            <Switch.Content>{t('market.bot.serverConfig.enableEmbedded')}</Switch.Content>
          </Switch>
          <span className="text-xs text-muted">{t('market.bot.serverConfig.restartRequired')}</span>
        </div>
        <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-muted">{t('market.bot.serverConfig.cacheDb')}</span>
            <Input className="font-mono" value={cfg.market_bot_cache_db} onChange={set('market_bot_cache_db')} placeholder="~/.dune-admin/market-bot-cache.db" aria-label={t('market.bot.serverConfig.cacheDb')} />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-muted">{t('market.bot.serverConfig.itemData')}</span>
            <Input className="font-mono" value={cfg.market_bot_item_data} onChange={set('market_bot_item_data')} placeholder="item-data.json" aria-label={t('market.bot.serverConfig.itemData')} />
          </label>
          <label className="flex flex-col gap-1 sm:col-span-2">
            <span className="text-xs font-medium text-muted">{t('market.bot.serverConfig.statePath')}</span>
            <Input className="font-mono" value={cfg.market_bot_state} onChange={set('market_bot_state')} placeholder="~/.dune-admin/market-bot-state.json" aria-label={t('market.bot.serverConfig.statePath')} />
          </label>
        </div>
      </Panel>

      <Panel>
        <SectionLabel>{t('market.bot.serverConfig.remoteBot')}</SectionLabel>
        <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-muted">{t('market.bot.serverConfig.remoteUrl')}</span>
            <Input className="font-mono" value={cfg.market_bot_remote_url} onChange={set('market_bot_remote_url')} placeholder="http://host:9191" aria-label={t('market.bot.serverConfig.remoteUrl')} />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-muted">{t('market.bot.serverConfig.remoteToken')}</span>
            <Input className="font-mono" type="password" value={cfg.market_bot_remote_token} onChange={set('market_bot_remote_token')} placeholder={MASKED} aria-label={t('market.bot.serverConfig.remoteToken')} />
          </label>
        </div>
      </Panel>

      <div className="flex items-center justify-between gap-4">
        <p className="text-xs text-muted">{t('market.bot.serverConfig.changesNote')}</p>
        <Button size="sm" onPress={save} isDisabled={saving}>
          {saving ? <Spinner size="sm" color="current" /> : null}
          {t('common.save')}
        </Button>
      </div>
    </div>
  )
}
