import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Panel, SectionLabel } from '../../../dune-ui'
import { FieldRow } from '../fields/FieldRow'
import { TextInput } from '../fields/TextInput'
import { TwoColumnGrid } from '../fields/TwoColumnGrid'
import type { PathsPanelProps } from './interfaces'

export const PathsPanel: React.FC<PathsPanelProps> = ({ cfg, set }) => {
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
