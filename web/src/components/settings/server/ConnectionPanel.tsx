import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { MASKED } from '../../../api/client'
import { Panel, SectionLabel } from '../../../dune-ui'
import { FieldRow } from '../fields/FieldRow'
import { TextInput } from '../fields/TextInput'
import { CheckboxField } from '../fields/CheckboxField'
import { TwoColumnGrid } from '../fields/TwoColumnGrid'
import type { ConnectionPanelProps } from './interfaces'

// ConnectionPanel renders the per-server Database and RabbitMQ broker settings.
// The combined 'server' tab shows both; the standalone wizard 'db' / 'broker'
// steps show one each.
export const ConnectionPanel: React.FC<ConnectionPanelProps> = ({ cfg, set, setBool, showDb, showBroker }) => {
  const { t } = useTranslation()
  return (
    <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
      {showDb && (
        <Panel>
          <SectionLabel>{t('settings.sections.database')}</SectionLabel>
          <TwoColumnGrid>
            <FieldRow label={t('settings.db.host')} hint={t('settings.db.hostHint')}>
              <TextInput value={cfg.db_host} onChange={set('db_host')} placeholder="127.0.0.1" />
            </FieldRow>
            <FieldRow label={t('settings.db.port')}>
              <TextInput
                value={cfg.db_port ? String(cfg.db_port) : ''}
                onChange={set('db_port')}
                placeholder="15432"
                type="number"
              />
            </FieldRow>
            <FieldRow label={t('settings.db.user')}>
              <TextInput value={cfg.db_user} onChange={set('db_user')} placeholder="dune" />
            </FieldRow>
            <FieldRow label={t('settings.db.password')} hint={t('settings.db.passwordHint')}>
              <TextInput value={cfg.db_pass} onChange={set('db_pass')} type="password" placeholder={MASKED} />
            </FieldRow>
            <FieldRow label={t('settings.db.name')}>
              <TextInput value={cfg.db_name} onChange={set('db_name')} placeholder="dune" />
            </FieldRow>
            <FieldRow label={t('settings.db.schema')}>
              <TextInput value={cfg.db_schema} onChange={set('db_schema')} placeholder="dune" />
            </FieldRow>
          </TwoColumnGrid>
        </Panel>
      )}

      {showBroker && (
        <Panel>
          <SectionLabel>{t('settings.sections.rabbitmq')}</SectionLabel>
          <p className="text-xs text-muted -mt-1">{t('settings.broker.optionalHint')}</p>
          <TwoColumnGrid>
            <FieldRow label={t('settings.broker.gameAddr')}><TextInput value={cfg.broker_game_addr} onChange={set('broker_game_addr')} placeholder="10.x.x.x:5672" /></FieldRow>
            <FieldRow label={t('settings.broker.adminAddr')}><TextInput value={cfg.broker_admin_addr} onChange={set('broker_admin_addr')} placeholder="10.x.x.x:5672" /></FieldRow>
            <FieldRow label={t('settings.broker.user')}><TextInput value={cfg.broker_user} onChange={set('broker_user')} placeholder="dune_cap" /></FieldRow>
            <FieldRow label={t('settings.broker.password')}><TextInput value={cfg.broker_pass} onChange={set('broker_pass')} type="password" placeholder={MASKED} /></FieldRow>
            <FieldRow label={t('settings.broker.jwtSecret')} hint={t('settings.broker.jwtSecretHint')}>
              <TextInput value={cfg.broker_jwt_secret} onChange={set('broker_jwt_secret')} type="password" placeholder={MASKED} />
            </FieldRow>
            <FieldRow label={t('settings.broker.execPrefix')} hint={t('settings.broker.execPrefixHint')}>
              <TextInput value={cfg.broker_exec_prefix} onChange={set('broker_exec_prefix')} placeholder="podman exec <container>" />
            </FieldRow>
            <div className="sm:col-span-2">
              <CheckboxField label={t('settings.broker.useTls')} checked={cfg.broker_tls} onChange={setBool('broker_tls')} />
            </div>
          </TwoColumnGrid>
        </Panel>
      )}
    </div>
  )
}
