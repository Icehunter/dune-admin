import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@heroui/react'
import type { AppConfig } from '../../../api/client'
import { Icon, Panel, SectionLabel } from '../../../dune-ui'
import { FieldRow } from '../fields/FieldRow'
import { TextInput } from '../fields/TextInput'
import { CheckboxField } from '../fields/CheckboxField'
import { TwoColumnGrid } from '../fields/TwoColumnGrid'

// Three rendering variants for the per-server "advanced" surface:
//  - 'add'        add-server wizard step: market bot only (per-server only)
//  - 'first-run'  first-run wizard step: listen+director, paths, market bot,
//                 backend-URL override (no apply/reset — wizard hides them)
//  - 'manage'     settings/manage tab: director, paths, market bot, danger zone
export type ServerAdvancedVariant = 'add' | 'first-run' | 'manage'

export interface ServerAdvancedPanelProps {
  variant: ServerAdvancedVariant
  cfg: AppConfig
  set: (key: keyof AppConfig) => (v: string) => void
  setBool: (key: keyof AppConfig) => (v: boolean) => void
  backendUrl: string
  setBackendUrl: (v: string) => void
  activeName: string
  onRequestDeleteServer?: () => void
}

interface MarketBotPanelProps {
  cfg: AppConfig
  setBool: (key: keyof AppConfig) => (v: boolean) => void
}

const MarketBotPanel: React.FC<MarketBotPanelProps> = ({ cfg, setBool }) => {
  const { t } = useTranslation()
  return (
    <Panel>
      <SectionLabel>{t('settings.sections.marketBot', 'Market Bot')}</SectionLabel>
      <CheckboxField
        label={t('settings.marketBot.enabled', 'Enable market bot for this server')}
        hint={t('settings.marketBot.enabledHint', 'Runs the embedded market bot against this server. Tuning is shared across servers and lives in the Market tab.')}
        checked={cfg.market_bot_enabled}
        onChange={setBool('market_bot_enabled')}
      />
    </Panel>
  )
}

const PathsPanel: React.FC<{ cfg: AppConfig, set: (key: keyof AppConfig) => (v: string) => void }> = ({ cfg, set }) => {
  const { t } = useTranslation()
  return (
    <Panel>
      <SectionLabel>{t('settings.sections.paths')}</SectionLabel>
      <TwoColumnGrid>
        <FieldRow label={t('settings.adv.backupDir')}>
          <TextInput value={cfg.backup_dir} onChange={set('backup_dir')} placeholder="/path/to/backups" />
        </FieldRow>
        <FieldRow label={t('settings.adv.serverIniDir')} hint={t('settings.adv.serverIniDirHint')}>
          <TextInput value={cfg.server_ini_dir} onChange={set('server_ini_dir')} placeholder="/path/to/server/state" />
        </FieldRow>
        <FieldRow label={t('settings.adv.defaultIniDir')} hint={t('settings.adv.defaultIniDirHint')}>
          <TextInput value={cfg.default_ini_dir} onChange={set('default_ini_dir')} placeholder="/path/to/game/Config" />
        </FieldRow>
      </TwoColumnGrid>
    </Panel>
  )
}

export const ServerAdvancedPanel: React.FC<ServerAdvancedPanelProps> = ({
  variant, cfg, set, setBool, backendUrl, setBackendUrl, activeName, onRequestDeleteServer,
}) => {
  const { t } = useTranslation()

  // Add-server wizard: per-server only → just the market bot toggle.
  if (variant === 'add') {
    return (
      <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
        <MarketBotPanel cfg={cfg} setBool={setBool} />
      </div>
    )
  }

  // First-run wizard: global + per-server combined.
  if (variant === 'first-run') {
    return (
      <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
        <Panel>
          <SectionLabel>{t('settings.sections.server')}</SectionLabel>
          <TwoColumnGrid>
            <FieldRow label={t('settings.adv.listenAddr')} hint={t('settings.adv.listenAddrHint')}>
              <TextInput value={cfg.listen_addr} onChange={set('listen_addr')} placeholder=":8080" />
            </FieldRow>
            <FieldRow label={t('settings.adv.directorUrl')} hint={t('settings.adv.directorUrlHint')}>
              <TextInput value={cfg.director_url} onChange={set('director_url')} placeholder="http://127.0.0.1:11717" />
            </FieldRow>
          </TwoColumnGrid>
        </Panel>

        <PathsPanel cfg={cfg} set={set} />
        <MarketBotPanel cfg={cfg} setBool={setBool} />

        <Panel>
          <SectionLabel>{t('settings.sections.backendUrlOverride')}</SectionLabel>
          <p className="text-xs text-muted -mt-1">
            {t('settings.adv.backendUrlHint')}
          </p>
          <TwoColumnGrid>
            <FieldRow label={t('settings.adv.url')} hint={t('settings.adv.urlHint')}>
              <TextInput
                value={backendUrl}
                onChange={(v) => {
                  setBackendUrl(v)
                  localStorage.setItem('dune_admin_backend', v)
                }}
                placeholder="http://host:port"
              />
            </FieldRow>
          </TwoColumnGrid>
        </Panel>
      </div>
    )
  }

  // Manage / settings per-server advanced tab.
  return (
    <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
      <Panel>
        <SectionLabel>{t('settings.sections.server')}</SectionLabel>
        <TwoColumnGrid>
          <FieldRow label={t('settings.adv.directorUrl')} hint={t('settings.adv.directorUrlHint')}>
            <TextInput value={cfg.director_url} onChange={set('director_url')} placeholder="http://127.0.0.1:11717" />
          </FieldRow>
        </TwoColumnGrid>
      </Panel>

      <PathsPanel cfg={cfg} set={set} />
      <MarketBotPanel cfg={cfg} setBool={setBool} />

      {onRequestDeleteServer && (
        <Panel>
          <SectionLabel>{t('settings.adv.dangerZone', 'Danger Zone')}</SectionLabel>
          <p className="text-xs text-muted -mt-1">
            {t('settings.adv.deleteServerHint', 'Permanently remove this server and all of its stored data. This cannot be undone.')}
          </p>
          <div className="mt-1">
            <Button size="sm" variant="danger-soft" onPress={() => onRequestDeleteServer()}>
              <Icon name="trash-2" />
              {' '}
              {activeName
                ? t('settings.adv.deleteServerNamed', 'Delete server "{{name}}"', { name: activeName })
                : t('settings.adv.deleteServer', 'Delete server')}
            </Button>
          </div>
        </Panel>
      )}
    </div>
  )
}
