import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@heroui/react'
import type { AppConfig } from '../../../api/client'
import { Panel, SectionLabel } from '../../../dune-ui'
import { FieldRow } from '../fields/FieldRow'
import { TextInput } from '../fields/TextInput'
import { TwoColumnGrid } from '../fields/TwoColumnGrid'

export interface AdminAdvancedPanelProps {
  cfg: AppConfig
  set: (key: keyof AppConfig) => (v: string) => void
  backendUrl: string
  setBackendUrl: (v: string) => void
}

export const AdminAdvancedPanel: React.FC<AdminAdvancedPanelProps> = ({ cfg, set, backendUrl, setBackendUrl }) => {
  const { t } = useTranslation()
  return (
    <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
      <Panel>
        <SectionLabel>{t('settings.sections.server')}</SectionLabel>
        <TwoColumnGrid>
          <FieldRow label={t('settings.adv.listenAddr')} hint={t('settings.adv.listenAddrHint')}>
            <TextInput value={cfg.listen_addr} onChange={set('listen_addr')} placeholder=":8080" />
          </FieldRow>
        </TwoColumnGrid>
      </Panel>

      <Panel>
        <SectionLabel>{t('settings.sections.backendUrlOverride')}</SectionLabel>
        <p className="text-xs text-muted -mt-1">{t('settings.adv.backendUrlHint')}</p>
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
        <div className="flex gap-2 mt-1">
          <Button size="sm" onPress={() => window.location.reload()}>{t('settings.adv.applyReload')}</Button>
          <Button
            size="sm"
            variant="outline"
            onPress={() => {
              setBackendUrl('')
              localStorage.removeItem('dune_admin_backend')
              window.location.reload()
            }}
          >
            {t('settings.adv.reset')}
          </Button>
        </div>
      </Panel>
    </div>
  )
}
