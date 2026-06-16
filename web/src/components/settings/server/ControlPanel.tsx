import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Select, ListBox } from '@heroui/react'
import { MASKED } from '../../../api/client'
import type { AppConfig } from '../../../api/client'
import { Panel, SectionLabel } from '../../../dune-ui'
import { FieldRow } from '../fields/FieldRow'
import { TextInput } from '../fields/TextInput'
import { CheckboxField } from '../fields/CheckboxField'
import { TwoColumnGrid } from '../fields/TwoColumnGrid'

export interface ControlPanelProps {
  cfg: AppConfig
  set: (key: keyof AppConfig) => (v: string) => void
  setBool: (key: keyof AppConfig) => (v: boolean) => void
  setControl: (v: string) => void
}

export const ControlPanel: React.FC<ControlPanelProps> = ({ cfg, set, setBool, setControl }) => {
  const { t } = useTranslation()
  const isKubectl = cfg.control === 'kubectl'
  const isDocker = cfg.control === 'docker'
  const isLocal = cfg.control === 'local'
  const isAmp = cfg.control === 'amp'

  return (
    <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
      <Panel>
        <SectionLabel>{t('settings.sections.controlPlane')}</SectionLabel>
        <div className="flex flex-col gap-1">
          <span className="text-xs text-muted font-medium">{t('settings.control.modeHint')}</span>
          <Select
            selectedKey={cfg.control || 'local'}
            onSelectionChange={(k) => setControl(String(k))}
            className="w-full"
            aria-label={t('settings.sections.controlPlane')}
          >
            <Select.Trigger>
              <Select.Value />
              <Select.Indicator />
            </Select.Trigger>
            <Select.Popover>
              <ListBox>
                <ListBox.Item id="kubectl" textValue="kubectl">
                  {t('settings.control.kubectl')}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
                <ListBox.Item id="docker" textValue="docker">
                  {t('settings.control.docker')}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
                <ListBox.Item id="local" textValue="local">
                  {t('settings.control.local')}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
                <ListBox.Item id="amp" textValue="amp">
                  {t('settings.control.amp')}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
              </ListBox>
            </Select.Popover>
          </Select>
        </div>
      </Panel>

      {isKubectl && (
        <Panel>
          <SectionLabel>{t('settings.sections.kubernetes')}</SectionLabel>
          <TwoColumnGrid>
            <FieldRow label={t('settings.k8s.namespace')} hint={t('settings.k8s.namespaceHint')}>
              <TextInput value={cfg.control_namespace} onChange={set('control_namespace')} placeholder="my-namespace" />
            </FieldRow>
          </TwoColumnGrid>
        </Panel>
      )}

      {isDocker && (
        <Panel>
          <SectionLabel>{t('settings.sections.dockerContainers')}</SectionLabel>
          <TwoColumnGrid>
            <FieldRow label={t('settings.docker.gameServer')}><TextInput value={cfg.docker_gameserver} onChange={set('docker_gameserver')} placeholder="dune-gameserver" /></FieldRow>
            <FieldRow label={t('settings.docker.brokerGame')}><TextInput value={cfg.docker_broker_game} onChange={set('docker_broker_game')} placeholder="dune-mq-game" /></FieldRow>
            <FieldRow label={t('settings.docker.brokerAdmin')}><TextInput value={cfg.docker_broker_admin} onChange={set('docker_broker_admin')} placeholder="dune-mq-admin" /></FieldRow>
            <FieldRow label={t('settings.docker.database')}><TextInput value={cfg.docker_db} onChange={set('docker_db')} placeholder="dune-postgres" /></FieldRow>
          </TwoColumnGrid>
        </Panel>
      )}

      {isLocal && (
        <Panel>
          <SectionLabel>{t('settings.sections.serverCommands')}</SectionLabel>
          <TwoColumnGrid>
            <FieldRow label={t('settings.cmd.start')}><TextInput value={cfg.cmd_start} onChange={set('cmd_start')} placeholder="service dune start" /></FieldRow>
            <FieldRow label={t('settings.cmd.stop')}><TextInput value={cfg.cmd_stop} onChange={set('cmd_stop')} placeholder="service dune stop" /></FieldRow>
            <FieldRow label={t('settings.cmd.restart')}><TextInput value={cfg.cmd_restart} onChange={set('cmd_restart')} placeholder="service dune restart" /></FieldRow>
            <FieldRow label={t('settings.cmd.status')}><TextInput value={cfg.cmd_status} onChange={set('cmd_status')} placeholder="service dune status" /></FieldRow>
          </TwoColumnGrid>
        </Panel>
      )}

      {isAmp && (
        <Panel>
          <SectionLabel>{t('settings.sections.amp')}</SectionLabel>
          <TwoColumnGrid>
            <FieldRow label={t('settings.amp.instanceName')}><TextInput value={cfg.amp_instance} onChange={set('amp_instance')} placeholder="DuneAwakening01" /></FieldRow>
            <FieldRow label={t('settings.amp.containerName')} hint={t('settings.amp.containerNameHint')}><TextInput value={cfg.amp_container} onChange={set('amp_container')} placeholder="AMP_DuneAwakening01" /></FieldRow>
            <FieldRow label={t('settings.amp.user')}><TextInput value={cfg.amp_user} onChange={set('amp_user')} placeholder="amp" /></FieldRow>
            <FieldRow label={t('settings.amp.logPath')}><TextInput value={cfg.amp_log_path} onChange={set('amp_log_path')} placeholder="/logs" /></FieldRow>
            <FieldRow label={t('settings.amp.dataRoot')}><TextInput value={cfg.amp_data_root} onChange={set('amp_data_root')} placeholder="/AMP/duneawakening" /></FieldRow>
            <CheckboxField
              label={t('settings.amp.useContainer')}
              checked={cfg.amp_use_container}
              onChange={setBool('amp_use_container')}
              hint={t('settings.amp.useContainerHint')}
            />
          </TwoColumnGrid>
          <p className="text-xs text-muted mt-3">{t('settings.amp.apiHint')}</p>
          <TwoColumnGrid>
            <FieldRow label={t('settings.amp.apiUser')}><TextInput value={cfg.amp_api_user} onChange={set('amp_api_user')} placeholder="admin" /></FieldRow>
            <FieldRow label={t('settings.amp.apiPassword')}><TextInput value={cfg.amp_api_pass} onChange={set('amp_api_pass')} type="password" placeholder={MASKED} /></FieldRow>
            <FieldRow label={t('settings.amp.apiPort')}>
              <TextInput
                value={cfg.amp_api_port ? String(cfg.amp_api_port) : ''}
                onChange={set('amp_api_port')}
                placeholder="8081"
                type="number"
              />
            </FieldRow>
          </TwoColumnGrid>
        </Panel>
      )}

      {!isKubectl && !isDocker && !isLocal && !isAmp && (
        <p className="text-xs text-muted pt-2">{t('settings.control.selectMode')}</p>
      )}
    </div>
  )
}
