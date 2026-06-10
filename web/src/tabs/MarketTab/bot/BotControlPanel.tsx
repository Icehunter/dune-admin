import type React from 'react'
import { useState, useRef, useEffect, useCallback } from 'react'
import { Button, Modal, Spinner, Switch } from '@heroui/react'
import { Segment } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { BotStatus, BotConfig } from '../../../api/client'
import { Icon } from '../../../dune-ui'
import { BotStatusCard } from './BotStatusCard'
import { BotActions } from './BotActions'
import { BotLogViewer } from './BotLogViewer'
import { BotConfigEditor, type ConfigEditorHandle } from './BotConfigEditor'
import { DisabledItemsManager } from './DisabledItemsManager'
import { BotServerConfig, type BotServerConfigHandle } from './BotServerConfig'

type BotControlPanelProps = {
  open: boolean
  onClose: () => void
}

export const BotControlPanel: React.FC<BotControlPanelProps> = ({ open, onClose }: BotControlPanelProps) => {
  const { t } = useTranslation()
  const [status, setStatus] = useState<BotStatus | null>(null)
  const [config, setConfig] = useState<BotConfig | null>(null)
  const [statusLoading, setStatusLoading] = useState(false)
  const [configLoading, setConfigLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState('config')
  const editorRef = useRef<ConfigEditorHandle>(null)
  const serverConfigRef = useRef<BotServerConfigHandle>(null)

  const loadStatus = useCallback(() => {
    Promise.resolve()
      .then(() => setStatusLoading(true))
      .then(() => api.marketBot.status())
      .then((s) => {
        setStatus(s)
        setError(null)
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setStatusLoading(false))
  }, [])

  const loadConfig = useCallback(() => {
    Promise.resolve()
      .then(() => setConfigLoading(true))
      .then(() => api.marketBot.config())
      .then(setConfig)
      .catch(() => { /* config load failure is non-fatal */ })
      .finally(() => setConfigLoading(false))
  }, [])

  useEffect(() => {
    if (open) {
      loadStatus()
      loadConfig()
    }
  }, [open, loadStatus, loadConfig])

  return (
    <Modal.Backdrop isOpen={open} onOpenChange={(v) => !v && onClose()}>
      <Modal.Container size="cover" scroll="outside">
        <Modal.Dialog className="h-[92vh] flex flex-col dialog-surface-alt">
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading>{t('market.bot.panelTitle')}</Modal.Heading>
          </Modal.Header>

          <Modal.Body className="flex flex-col gap-4 overflow-y-auto flex-1 pr-1 min-h-0">
            {/* Status + actions */}
            {error
              ? (
                  <p className="text-xs text-danger">{error}</p>
                )
              : status
                ? (
                    <div className="flex flex-wrap items-center gap-4 justify-between pb-2 border-b border-border shrink-0">
                      <BotStatusCard status={status} />
                      <BotActions status={status} onRefresh={loadStatus} />
                    </div>
                  )
                : statusLoading
                  ? (
                      <div className="flex justify-center py-4 shrink-0"><Spinner size="sm" /></div>
                    )
                  : null}

            {/* Segment switcher — right-aligned, flex-1 so panel fills remaining height */}
            <div className="flex flex-col flex-1 min-h-0 gap-4">
              <div className="flex justify-end shrink-0">
                <Segment
                  selectedKey={activeTab}
                  onSelectionChange={(k) => setActiveTab(String(k))}
                  size="sm"
                  aria-label={t('market.bot.botSectionsLabel')}
                >
                  <Segment.Item id="config">
                    <Segment.Separator />
                    {t('market.bot.config')}
                  </Segment.Item>
                  <Segment.Item id="disabled">
                    <Segment.Separator />
                    {t('market.bot.disabledItemsTab')}
                  </Segment.Item>
                  <Segment.Item id="server">
                    <Segment.Separator />
                    {t('market.bot.server')}
                  </Segment.Item>
                  <Segment.Item id="logs">
                    <Segment.Separator />
                    {t('market.bot.logs')}
                  </Segment.Item>
                </Segment>
              </div>

              {activeTab === 'config' && (
                <div className="overflow-y-auto flex-1 pr-1">
                  {configLoading
                    ? <div className="flex justify-center py-6"><Spinner size="sm" /></div>
                    : config
                      ? <BotConfigEditor ref={editorRef} config={config} onSaved={setConfig} />
                      : <p className="text-xs text-muted">{t('market.bot.configUnavailable')}</p>}
                </div>
              )}

              {activeTab === 'disabled' && (
                <div className="overflow-y-auto flex-1 pr-1">
                  {configLoading
                    ? <div className="flex justify-center py-6"><Spinner size="sm" /></div>
                    : config
                      ? <DisabledItemsManager config={config} onSaved={setConfig} />
                      : <p className="text-xs text-muted">{t('market.bot.configUnavailable')}</p>}
                </div>
              )}

              {activeTab === 'server' && (
                <div className="overflow-y-auto flex-1 pr-1">
                  <BotServerConfig ref={serverConfigRef} />
                </div>
              )}

              {activeTab === 'logs' && (
                <div className="flex-1 min-h-0 flex flex-col overflow-hidden">
                  <BotLogViewer active />
                </div>
              )}
            </div>
          </Modal.Body>

          {activeTab === 'config' && config && !configLoading && (
            <ConfigFooter editorRef={editorRef} initialEnabled={config.enabled} onReload={loadConfig} />
          )}
          {activeTab === 'server' && (
            <ServerConfigFooter configRef={serverConfigRef} />
          )}
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}

interface ConfigFooterProps {
  editorRef: React.RefObject<ConfigEditorHandle | null>
  initialEnabled: boolean
  onReload: () => void
}

function ConfigFooter({ editorRef, initialEnabled, onReload }: ConfigFooterProps) {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)
  const [reloading, setReloading] = useState(false)
  const [enabled, setEnabledLocal] = useState(initialEnabled)

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

function ServerConfigFooter({ configRef }: { configRef: React.RefObject<BotServerConfigHandle | null> }) {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)

  return (
    <div className="shrink-0 flex items-center justify-between gap-3 px-4 py-3">
      <p className="text-xs text-muted">{t('market.bot.serverConfig.changesNote')}</p>
      <Button
        size="sm"
        isDisabled={saving}
        onPress={() => {
          setSaving(true)
          configRef.current?.save()
            .catch(() => { /* toast shown inside save */ })
            .finally(() => setSaving(false))
        }}
      >
        {saving ? <Spinner size="sm" color="current" /> : null}
        {t('common.save')}
      </Button>
    </div>
  )
}
