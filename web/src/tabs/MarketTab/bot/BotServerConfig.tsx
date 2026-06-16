import * as React from 'react'
import { Input, Spinner, Switch, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api, MASKED } from '../../../api/client'
import type { AppConfig } from '../../../api/client'
import { Panel, SectionLabel } from '../../../dune-ui'
import { useActiveServer } from '../../../context/useActiveServer'
import type { BotServerConfigHandle, StringAppConfigKey } from './types'

export const BotServerConfig = React.forwardRef<BotServerConfigHandle>((_, ref) => {
  const { t } = useTranslation()
  const { activeID, servers } = useActiveServer()
  const serverID = activeID || servers[0]?.id || 0
  const activeName = servers.find((s) => s.id === serverID)?.name ?? ''
  const [cfg, setCfg] = React.useState<AppConfig | null>(null)
  const [loading, setLoading] = React.useState(false)

  React.useEffect(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => Promise.all([
        api.config.get(),
        api.servers.getConfig(serverID).catch(() => null),
      ]))
      .then(([global, server]) => {
        // The market_bot_enabled toggle is PER SERVER; the rest of the config is
        // global/shared. Overlay the active server's toggle onto the global cfg.
        setCfg({ ...global, market_bot_enabled: server?.market_bot_enabled ?? global.market_bot_enabled })
      })
      .catch(() => toast.danger(t('market.bot.serverConfig.loadFailed')))
      .finally(() => setLoading(false))
  }, [t, serverID])

  const set = (key: StringAppConfigKey) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setCfg((prev) => prev ? { ...prev, [key]: e.target.value } : prev)

  const setBool = (key: keyof AppConfig) => (checked: boolean) =>
    setCfg((prev) => prev ? { ...prev, [key]: checked } : prev)

  React.useImperativeHandle(ref, () => ({
    save: async () => {
      if (!cfg) return
      try {
        // Global/shared bot config (cache, item data, state, remote) is saved via
        // /config. For the default (single-server) install this also applies the
        // per-server toggle via the legacy mapping. For a non-default active
        // server, the toggle is saved through its per-server config endpoint.
        await api.config.save(cfg)
        if (serverID !== 0) {
          await api.servers.saveConfig(serverID, {
            id: serverID,
            name: activeName,
            market_bot_enabled: cfg.market_bot_enabled,
          })
        }
        toast.success(t('market.bot.serverConfig.savedConfig'))
      }
      catch (e: unknown) {
        toast.danger(t('market.bot.serverConfig.saveFailed', { message: e instanceof Error ? e.message : String(e) }))
        throw e
      }
    },
  }), [cfg, t, serverID, activeName])

  if (loading) {
    return <div className="flex justify-center py-8"><Spinner size="sm" /></div>
  }
  if (!cfg) {
    return <p className="text-xs text-muted">{t('market.bot.configUnavailable')}</p>
  }

  return (
    <form className="flex flex-col gap-4" onSubmit={(e) => e.preventDefault()} autoComplete="off">
      <input type="text" autoComplete="username" aria-hidden="true" tabIndex={-1} readOnly className="sr-only" />
      <Panel>
        <SectionLabel>{t('market.bot.serverConfig.embeddedBot')}</SectionLabel>
        <div className="mt-2 flex items-center gap-2">
          <Switch isSelected={cfg.market_bot_enabled} onChange={setBool('market_bot_enabled')} size="sm">
            <Switch.Control><Switch.Thumb /></Switch.Control>
            <Switch.Content>{t('market.bot.serverConfig.enableEmbedded')}</Switch.Content>
          </Switch>
          <span className="text-xs text-muted">{t('market.bot.serverConfig.restartRequired')}</span>
        </div>
        <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-muted">{t('market.bot.serverConfig.cacheDb')}</span>
            <Input className="font-mono" autoComplete="off" value={cfg.market_bot_cache_db} onChange={set('market_bot_cache_db')} placeholder="~/.dune-admin/market-bot-cache.db" aria-label={t('market.bot.serverConfig.cacheDb')} />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-muted">{t('market.bot.serverConfig.itemData')}</span>
            <Input className="font-mono" autoComplete="off" value={cfg.market_bot_item_data} onChange={set('market_bot_item_data')} placeholder="item-data.json" aria-label={t('market.bot.serverConfig.itemData')} />
          </label>
          <label className="flex flex-col gap-1 sm:col-span-2">
            <span className="text-xs font-medium text-muted">{t('market.bot.serverConfig.statePath')}</span>
            <Input className="font-mono" autoComplete="off" value={cfg.market_bot_state} onChange={set('market_bot_state')} placeholder="~/.dune-admin/market-bot-state.json" aria-label={t('market.bot.serverConfig.statePath')} />
          </label>
        </div>
      </Panel>

      <Panel>
        <SectionLabel>{t('market.bot.serverConfig.remoteBot')}</SectionLabel>
        <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-muted">{t('market.bot.serverConfig.remoteUrl')}</span>
            <Input className="font-mono" autoComplete="url" value={cfg.market_bot_remote_url} onChange={set('market_bot_remote_url')} placeholder="http://host:9191" aria-label={t('market.bot.serverConfig.remoteUrl')} />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-muted">{t('market.bot.serverConfig.remoteToken')}</span>
            <Input className="font-mono" type="password" autoComplete="new-password" value={cfg.market_bot_remote_token} onChange={set('market_bot_remote_token')} placeholder={MASKED} aria-label={t('market.bot.serverConfig.remoteToken')} />
          </label>
        </div>
      </Panel>
    </form>
  )
})
