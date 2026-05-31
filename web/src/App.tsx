import { useState, useEffect, useRef } from 'react'
import { Show, SignInButton, UserButton, useAuth } from '@clerk/react'
import { Button, Chip, Modal, Spinner, Toast, Tabs, toast } from '@heroui/react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useStatus } from './hooks/useStatus'
import SettingsConfigForm from './components/SettingsConfigForm'
import BattlegroupTab from './tabs/BattlegroupTab'
import PlayersTab from './tabs/PlayersTab'
import DatabaseTab from './tabs/DatabaseTab'
import LogsTab from './tabs/LogsTab'
import BlueprintsTab from './tabs/BlueprintsTab'
import BasesTab from './tabs/BasesTab'
import StorageTab from './tabs/StorageTab'
import ServerSettingsTab from './tabs/ServerSettingsTab'
import MarketTab from './tabs/MarketTab'
import WelcomePackageTab from './tabs/WelcomePackageTab'
import { Icon } from './dune-ui'
import { api } from './api/client'
import type { UpdateCheckResult } from './api/client'

const TAB_IDS = [
  'battlegroup',
  'players',
  'database',
  'logs',
  'blueprints',
  'bases',
  'storage',
  'server',
  'market',
  'welcome',
] as const
type TabId = (typeof TAB_IDS)[number]
const DEFAULT_TAB: TabId = 'battlegroup'

function currentTabFromPath(pathname: string): TabId {
  const seg = pathname.replace(/^\//, '').split('/')[0]
  return (TAB_IDS as readonly string[]).includes(seg) ? (seg as TabId) : DEFAULT_TAB
}

const hasClerk = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY

function AppWithAuth() {
  const { isSignedIn } = useAuth()
  return <AppCore isSignedIn={!!isSignedIn} />
}

export default function App() {
  return hasClerk ? <AppWithAuth /> : <AppCore isSignedIn={true} />
}

function parseVer(v: string): [number, number, number] {
  // Strip leading "v" and any pre-release suffix (-dev, -rc1, etc.) before parsing.
  const [a, b, c] = v.replace(/^v/, '').replace(/-.*$/, '').split('.').map(Number)
  return [a || 0, b || 0, c || 0]
}

function isNewer(latest: string, current: string): boolean {
  const [la, lb, lc] = parseVer(latest)
  const [ca, cb, cc] = parseVer(current)
  if (la !== ca) return la > ca
  if (lb !== cb) return lb > cb
  return lc > cc
}

function AppCore({ isSignedIn }: { isSignedIn: boolean }) {
  const status = useStatus()
  const location = useLocation()
  const navigate = useNavigate()
  const [showBackendConfig, setShowBackendConfig] = useState(false)
  const [latestVersion, setLatestVersion] = useState<string | null>(null)
  const [updateInfo, setUpdateInfo] = useState<UpdateCheckResult | null>(null)
  const [updateChecking, setUpdateChecking] = useState(false)
  const [updateApplying, setUpdateApplying] = useState(false)
  const [formSaving, setFormSaving] = useState(false)
  const formSaveRef = useRef<(() => Promise<void>) | null>(null)

  useEffect(() => {
    const seg = location.pathname.replace(/^\//, '').split('/')[0]
    if (!seg || !(TAB_IDS as readonly string[]).includes(seg)) {
      navigate(`/${DEFAULT_TAB}`, { replace: true })
    }
  }, [location.pathname, navigate])

  const currentTab = currentTabFromPath(location.pathname)

  useEffect(() => {
    fetch('https://api.github.com/repos/Icehunter/dune-admin/releases/latest')
      .then((r) => r.json())
      .then((d) => setLatestVersion(d.tag_name || null))
      .catch(() => {})
  }, [])

  const checkUpdate = async () => {
    setUpdateChecking(true)
    try {
      setUpdateInfo(await api.update.check())
    }
    catch {
      // silently ignore — user can retry
    }
    finally {
      setUpdateChecking(false)
    }
  }

  const applyUpdate = async (force = false) => {
    setUpdateApplying(true)
    try {
      const result = await api.update.apply(force)
      if (result.updated) {
        toast.success(`${force ? 'Reinstalled' : 'Updated to'} ${result.version ?? 'latest'}. Server is restarting…`)
        setUpdateInfo(null)
      }
      else {
        toast.info(result.message)
      }
    }
    catch (e) {
      toast.danger(`Update failed: ${e instanceof Error ? e.message : String(e)}`)
    }
    finally {
      setUpdateApplying(false)
    }
  }

  return (
    <div className="h-screen flex flex-col overflow-hidden bg-background">
      <Toast.Provider />

      {/* Header */}
      <header
        className="flex items-center justify-between px-6 py-3 border-b border-[#4e3411] bg-surface shrink-0"
        style={{ background: 'linear-gradient(180deg, #241a0e 0%, #1a1610 100%)' }}
      >
        <div className="flex items-center gap-3">
          <span className="text-xl font-bold uppercase tracking-[0.2em] text-accent">DUNE ADMIN</span>
          {status?.control && status.control !== 'none' && <span className="text-xs text-muted">{status.control}</span>}
          {status?.ssh_host && <span className="text-xs text-muted">{status.ssh_host}</span>}
          {status?.db_host && status.control !== 'kubectl' && (
            <span className="text-xs text-muted">{status.db_host}</span>
          )}
          {status?.version && (
            <button
              className="text-xs text-muted hover:text-foreground cursor-pointer bg-transparent border-0 p-0"
              onClick={() => setShowBackendConfig(true)}
              title="Open Settings"
            >
              v
              {status.version}
            </button>
          )}
          {latestVersion && status?.version && isNewer(latestVersion, status.version) && (
            <a
              href="https://github.com/Icehunter/dune-admin/releases/latest"
              target="_blank"
              rel="noreferrer"
              className="no-underline"
            >
              <Chip size="sm" color="warning" variant="soft">
                ↑
                {' '}
                {latestVersion}
              </Chip>
            </a>
          )}
        </div>

        <div className="flex items-center gap-3">
          {status?.executor === 'ssh' && <ConnectionBadge label="SSH" connected={status.ssh_connected} />}
          <ConnectionBadge label="DB" connected={status?.db_connected ?? false} />
          {status?.pod_ns && (
            <span className="text-xs text-muted">
              ns:
              {status.pod_ns}
            </span>
          )}

          <Button
            size="sm"
            variant="ghost"
            isIconOnly
            aria-label="Configure backend"
            onPress={() => setShowBackendConfig((v) => !v)}
            className={showBackendConfig ? 'text-accent' : ''}
          >
            <Icon name="settings" />
          </Button>

          {hasClerk && (
            <>
              <Show when="signed-out">
                <SignInButton>
                  <Button size="sm" variant="outline">
                    Sign In
                  </Button>
                </SignInButton>
              </Show>
              <Show when="signed-in">
                <UserButton />
              </Show>
            </>
          )}
        </div>
      </header>

      {/* Settings modal — structure mirrors BotControlPanel */}
      <Modal>
        <Modal.Backdrop isOpen={showBackendConfig} onOpenChange={(v) => !v && setShowBackendConfig(false)}>
          <Modal.Container size="cover" scroll="outside">
            <Modal.Dialog className="h-[92vh] flex flex-col">
              <Modal.CloseTrigger />
              <Modal.Header>
                <div className="flex items-baseline gap-6 flex-wrap">
                  <Modal.Heading className="text-accent">Settings</Modal.Heading>
                  {status && (
                    <div className="flex items-center gap-4 text-xs text-muted">
                      {status.version && (
                        <span className="font-mono">
                          v
                          {status.version}
                        </span>
                      )}
                      {status.control && status.control !== 'none' && <span>{status.control}</span>}
                      {status.commit && status.commit !== 'unknown' && (
                        <span className="font-mono opacity-60">{status.commit}</span>
                      )}
                    </div>
                  )}
                </div>
              </Modal.Header>

              {/* Body scrolls; form fills it with its own internal tab scroll */}
              <Modal.Body className="flex flex-col overflow-y-auto flex-1 min-h-0 pr-1">
                {showBackendConfig && (
                  <SettingsConfigForm saveRef={formSaveRef} onSavingChange={setFormSaving} />
                )}
              </Modal.Body>

              <Modal.Footer className="flex items-center gap-2">
                {/* Left: update controls — fixed positions so buttons don't shift */}
                <Button
                  size="sm"
                  variant="ghost"
                  onPress={checkUpdate}
                  isDisabled={updateChecking || updateApplying}
                >
                  {updateChecking
                    ? (
                        <>
                          <Spinner size="sm" color="current" />
                          {' '}
                          Checking…
                        </>
                      )
                    : 'Check for Updates'}
                </Button>
                {updateInfo && !updateInfo.needs_update && (
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={() => applyUpdate(true)}
                    isDisabled={updateApplying}
                  >
                    {updateApplying ? <Spinner size="sm" color="current" /> : 'Reinstall'}
                  </Button>
                )}
                {updateInfo?.needs_update && (
                  <Button size="sm" onPress={() => applyUpdate()} isDisabled={updateApplying}>
                    {updateApplying
                      ? <Spinner size="sm" color="current" />
                      : (
                          <span className="font-mono text-xs">
                            v
                            {updateInfo.current}
                            {' → '}
                            v
                            {updateInfo.latest.replace(/^v/, '')}
                          </span>
                        )}
                  </Button>
                )}

                {/* Spacer */}
                <span className="flex-1" />

                {/* Right: save + close */}
                <span className="text-xs text-muted">Changes on all tabs are saved together.</span>
                <Button
                  size="sm"
                  onPress={() => formSaveRef.current?.()}
                  isDisabled={formSaving}
                >
                  {formSaving
                    ? (
                        <>
                          <Spinner size="sm" color="current" />
                          {' '}
                          Saving…
                        </>
                      )
                    : (
                        <>
                          <Icon name="save" />
                          {' '}
                          Save & Apply
                        </>
                      )}
                </Button>
                <Button
                  size="sm"
                  variant="tertiary"
                  onPress={() => setShowBackendConfig(false)}
                >
                  Close
                </Button>
              </Modal.Footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>

      {/* Tabs */}
      <div className="flex-1 flex flex-col overflow-hidden min-h-0">
        <Tabs
          selectedKey={currentTab}
          onSelectionChange={(k) => navigate(`/${k}`)}
          className="flex-1 flex flex-col overflow-hidden min-h-0"
        >
          <Tabs.ListContainer className="px-4 pt-2 shrink-0">
            <Tabs.List aria-label="Admin sections" className="gap-1">
              <Tabs.Tab id="battlegroup">
                Battlegroup
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="players">
                Players
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="database">
                Database
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="logs">
                Logs
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="blueprints">
                Blueprints
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="bases">
                Bases
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="storage">
                Storage
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="server">
                Server
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="market">
                Market
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="welcome">
                Welcome
                <Tabs.Indicator />
              </Tabs.Tab>
            </Tabs.List>
          </Tabs.ListContainer>
          <Tabs.Panel id="battlegroup" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <BattlegroupTab isActive={currentTab === 'battlegroup'} />
          </Tabs.Panel>
          <Tabs.Panel id="players" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <PlayersTab isActive={currentTab === 'players'} />
          </Tabs.Panel>
          <Tabs.Panel id="database" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <DatabaseTab />
          </Tabs.Panel>
          <Tabs.Panel id="logs" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <LogsTab />
          </Tabs.Panel>
          <Tabs.Panel id="blueprints" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <BlueprintsTab isSignedIn={isSignedIn} />
          </Tabs.Panel>
          <Tabs.Panel id="bases" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <BasesTab isSignedIn={isSignedIn} />
          </Tabs.Panel>
          <Tabs.Panel id="storage" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <StorageTab />
          </Tabs.Panel>
          <Tabs.Panel id="server" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <ServerSettingsTab />
          </Tabs.Panel>
          <Tabs.Panel id="market" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <MarketTab />
          </Tabs.Panel>
          <Tabs.Panel id="welcome" className="flex-1 overflow-hidden flex flex-col p-4 min-h-0">
            <WelcomePackageTab />
          </Tabs.Panel>
        </Tabs>
      </div>
    </div>
  )
}

function ConnectionBadge({ label, connected }: { label: string, connected: boolean }) {
  return (
    <div className="flex items-center gap-1.5 text-xs">
      <div className={`w-2 h-2 rounded-full ${connected ? 'bg-success' : 'bg-muted/40'}`} />
      <span className={connected ? 'text-foreground' : 'text-muted'}>{label}</span>
    </div>
  )
}
