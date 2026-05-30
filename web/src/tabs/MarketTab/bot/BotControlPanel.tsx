import { useState, useRef, useEffect, useCallback } from 'react'
import { Button, Modal, Spinner, Tabs } from '@heroui/react'
import { api } from '../../../api/client'
import type { BotStatus, BotConfig } from '../../../api/client'
import { Icon } from '../../../dune-ui'
import BotStatusCard from './BotStatusCard'
import BotActions from './BotActions'
import BotLogViewer from './BotLogViewer'
import BotConfigEditor, { type ConfigEditorHandle } from './BotConfigEditor'
import DisabledItemsManager from './DisabledItemsManager'

type Props = {
  open: boolean
  onClose: () => void
}

export default function BotControlPanel({ open, onClose }: Props) {
  const [status, setStatus] = useState<BotStatus | null>(null)
  const [config, setConfig] = useState<BotConfig | null>(null)
  const [statusLoading, setStatusLoading] = useState(false)
  const [configLoading, setConfigLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState('config')
  const editorRef = useRef<ConfigEditorHandle>(null)

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
    <Modal>
      <Modal.Backdrop isOpen={open} onOpenChange={(v) => !v && onClose()}>
        <Modal.Container size="cover" scroll="outside">
          <Modal.Dialog className="h-[92vh] flex flex-col dialog-surface-alt">
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading>Bot Control — Revy</Modal.Heading>
            </Modal.Header>

            <Modal.Body className="flex flex-col gap-4 overflow-y-auto flex-1 pr-1 min-h-0">
              {/* Status + actions */}
              {error
                ? (
                    <p className="text-xs text-danger">{error}</p>
                  )
                : status
                  ? (
                      <div className="flex flex-wrap items-start gap-4 justify-between pb-2 border-b border-border shrink-0">
                        <BotStatusCard status={status} />
                        <BotActions status={status} onRefresh={loadStatus} />
                      </div>
                    )
                  : statusLoading
                    ? (
                        <div className="flex justify-center py-4 shrink-0"><Spinner size="sm" /></div>
                      )
                    : null}

              {/* Tabs — flex-1 so logs panel can fill the remaining height */}
              <Tabs selectedKey={activeTab} onSelectionChange={(k) => setActiveTab(String(k))} className="flex flex-col flex-1 min-h-0">
                <Tabs.ListContainer className="shrink-0">
                  <Tabs.List aria-label="Bot sections">
                    <Tabs.Tab id="config">
                      Config
                      <Tabs.Indicator />
                    </Tabs.Tab>
                    <Tabs.Tab id="disabled">
                      Disabled Items
                      <Tabs.Indicator />
                    </Tabs.Tab>
                    <Tabs.Tab id="logs">
                      Logs
                      <Tabs.Indicator />
                    </Tabs.Tab>
                  </Tabs.List>
                </Tabs.ListContainer>

                <Tabs.Panel id="config" className="pt-4 overflow-y-auto flex-1 pr-1">
                  {configLoading
                    ? (
                        <div className="flex justify-center py-6"><Spinner size="sm" /></div>
                      )
                    : config
                      ? (
                          <BotConfigEditor ref={editorRef} config={config} onSaved={setConfig} />
                        )
                      : (
                          <p className="text-xs text-muted">Config unavailable.</p>
                        )}
                </Tabs.Panel>

                <Tabs.Panel id="disabled" className="pt-4 overflow-y-auto flex-1 pr-1">
                  {configLoading
                    ? (
                        <div className="flex justify-center py-6"><Spinner size="sm" /></div>
                      )
                    : config
                      ? (
                          <DisabledItemsManager config={config} onSaved={setConfig} />
                        )
                      : (
                          <p className="text-xs text-muted">Config unavailable.</p>
                        )}
                </Tabs.Panel>

                <Tabs.Panel id="logs" className="pt-4 flex-1 min-h-0 flex flex-col overflow-hidden">
                  <BotLogViewer active={activeTab === 'logs'} />
                </Tabs.Panel>
              </Tabs>
            </Modal.Body>

            {/* Static config footer — only shown on the config tab */}
            {activeTab === 'config' && config && !configLoading && (
              <ConfigFooter editorRef={editorRef} initialEnabled={config.enabled} onReload={loadConfig} />
            )}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

interface ConfigFooterProps {
  editorRef: React.RefObject<ConfigEditorHandle | null>
  initialEnabled: boolean
  onReload: () => void
}

function ConfigFooter({ editorRef, initialEnabled, onReload }: ConfigFooterProps) {
  const [saving, setSaving] = useState(false)
  const [reloading, setReloading] = useState(false)
  const [enabled, setEnabledLocal] = useState(initialEnabled)

  return (
    <div className="shrink-0 flex items-center gap-3 px-4 py-3 border-t border-border">
      <label className="flex items-center gap-2 text-sm cursor-pointer select-none mr-auto">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => {
            setEnabledLocal(e.target.checked)
            editorRef.current?.setEnabled(e.target.checked)
          }}
          className="accent-[var(--color-accent)]"
        />
        Ticking enabled
      </label>
      <Button size="sm" variant="ghost" onPress={() => editorRef.current?.reset()}>
        Reset
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
        Reload Config
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
        Save Config
      </Button>
    </div>
  )
}
