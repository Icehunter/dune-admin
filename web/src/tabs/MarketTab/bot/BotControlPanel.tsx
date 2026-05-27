import { useState, useEffect, useCallback } from 'react'
import { Button, Modal, Spinner, Tabs } from '@heroui/react'
import { api } from '../../../api/client'
import type { BotStatus, BotConfig } from '../../../api/client'
import { Icon } from '../../../dune-ui'
import BotStatusCard from './BotStatusCard'
import BotActions from './BotActions'
import BotLogViewer from './BotLogViewer'
import BotConfigEditor from './BotConfigEditor'
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

  const loadStatus = useCallback(async () => {
    setStatusLoading(true)
    try {
      setStatus(await api.marketBot.status())
      setError(null)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setStatusLoading(false)
    }
  }, [])

  const loadConfig = useCallback(async () => {
    setConfigLoading(true)
    try {
      setConfig(await api.marketBot.config())
    } catch {
      // config load failure is non-fatal
    } finally {
      setConfigLoading(false)
    }
  }, [])

  useEffect(() => {
    if (open) {
      loadStatus()
      loadConfig()
    }
  }, [open, loadStatus, loadConfig])

  return (
    <Modal>
      <Modal.Backdrop isOpen={open} onOpenChange={v => !v && onClose()}>
        <Modal.Container size="cover">
          <Modal.Dialog className="h-[92vh] flex flex-col dialog-surface-alt">
            <Modal.CloseTrigger />
            <Modal.Header>
              <div className="flex items-center justify-between w-full pr-8">
                <Modal.Heading>Bot Control — Revy</Modal.Heading>
                <Button size="sm" variant="ghost" onPress={() => { loadStatus(); loadConfig() }} isDisabled={statusLoading}>
                  {statusLoading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
                </Button>
              </div>
            </Modal.Header>

            <Modal.Body className="flex flex-col gap-4 overflow-y-auto flex-1">
              {/* Status + actions */}
              {error ? (
                <p className="text-xs text-danger">{error}</p>
              ) : status ? (
                <div className="flex flex-wrap items-start gap-4 justify-between pb-2 border-b border-border">
                  <BotStatusCard status={status} />
                  <BotActions status={status} onRefresh={loadStatus} />
                </div>
              ) : statusLoading ? (
                <div className="flex justify-center py-4"><Spinner size="sm" /></div>
              ) : null}

              {/* Tabs */}
              <Tabs selectedKey={activeTab} onSelectionChange={k => setActiveTab(String(k))}>
                <Tabs.ListContainer>
                  <Tabs.List aria-label="Bot sections">
                    <Tabs.Tab id="config">Config<Tabs.Indicator /></Tabs.Tab>
                    <Tabs.Tab id="disabled">Disabled Items<Tabs.Indicator /></Tabs.Tab>
                    <Tabs.Tab id="logs">Logs<Tabs.Indicator /></Tabs.Tab>
                  </Tabs.List>
                </Tabs.ListContainer>

                <Tabs.Panel id="config" className="pt-4">
                  {configLoading ? (
                    <div className="flex justify-center py-6"><Spinner size="sm" /></div>
                  ) : config ? (
                    <BotConfigEditor config={config} onSaved={setConfig} />
                  ) : (
                    <p className="text-xs text-muted">Config unavailable.</p>
                  )}
                </Tabs.Panel>

                <Tabs.Panel id="disabled" className="pt-4">
                  {configLoading ? (
                    <div className="flex justify-center py-6"><Spinner size="sm" /></div>
                  ) : config ? (
                    <DisabledItemsManager config={config} onSaved={setConfig} />
                  ) : (
                    <p className="text-xs text-muted">Config unavailable.</p>
                  )}
                </Tabs.Panel>

                <Tabs.Panel id="logs" className="pt-4 flex-1 min-h-0 flex flex-col">
                  <BotLogViewer active={activeTab === 'logs'} />
                </Tabs.Panel>
              </Tabs>
            </Modal.Body>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
