import * as React from 'react'
import { useTranslation } from 'react-i18next'
import type { AppConfig } from '../../../api/client'
import { Panel, SectionLabel } from '../../../dune-ui'
import { FieldRow } from '../fields/FieldRow'
import { TextInput } from '../fields/TextInput'
import { TwoColumnGrid } from '../fields/TwoColumnGrid'

export interface SshPanelProps {
  cfg: AppConfig
  set: (key: keyof AppConfig) => (v: string) => void
}

export const SshPanel: React.FC<SshPanelProps> = ({ cfg, set }) => {
  const { t } = useTranslation()
  return (
    <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
      <Panel>
        <SectionLabel>{t('settings.sections.ssh')}</SectionLabel>
        <p className="text-xs text-muted -mt-1">{t('settings.ssh.hint', 'Leave blank if dune-admin runs directly on the game server host.')}</p>
        <TwoColumnGrid>
          <FieldRow label={t('settings.ssh.hostPort')} hint={t('settings.ssh.hostPortHint')}>
            <TextInput value={cfg.ssh_host} onChange={set('ssh_host')} placeholder="192.168.0.72:22" />
          </FieldRow>
          <FieldRow label={t('settings.ssh.user')}>
            <TextInput value={cfg.ssh_user} onChange={set('ssh_user')} placeholder="dune" />
          </FieldRow>
          <FieldRow label={t('settings.ssh.privateKey')} hint={t('settings.ssh.privateKeyHint')}>
            <TextInput value={cfg.ssh_key} onChange={set('ssh_key')} placeholder="~/.ssh/id_ed25519" />
          </FieldRow>
        </TwoColumnGrid>
      </Panel>
    </div>
  )
}
