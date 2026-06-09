import type React from 'react'
import { memo, useState, useEffect, useRef, type ReactNode } from 'react'
import { useAuth } from '@clerk/react'
import { Button, Modal, Spinner, Toast, toast } from '@heroui/react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useStatus } from './hooks/useStatus'
import { SettingsConfigForm } from './components/SettingsConfigForm'
import { BattlegroupTab } from './tabs/BattlegroupTab'
import { LiveMapTab } from './tabs/LiveMapTab'
import { PlayersTab } from './tabs/PlayersTab'
import { DatabaseTab } from './tabs/DatabaseTab'
import { LogsTab } from './tabs/LogsTab'
import { BlueprintsTab } from './tabs/BlueprintsTab'
import { BasesTab } from './tabs/BasesTab'
import { GuildsTab } from './tabs/GuildsTab'
import { LandsraadTab } from './tabs/LandsraadTab'
import { StorageTab } from './tabs/StorageTab'
import { ServerSettingsTab } from './tabs/ServerSettingsTab'
import { MarketTab } from './tabs/MarketTab'
import { WelcomePackageTab } from './tabs/WelcomePackageTab'
import { DashboardTab } from './tabs/DashboardTab'
import { Icon } from './dune-ui'
import { Toaster } from './components/ui/toaster'
import { Sidebar } from './components/layout/Sidebar'
import { Topbar } from './components/layout/Topbar'
import { MobileNav } from './components/layout/MobileNav'
import { CommandPalette } from './components/layout/CommandPalette'
import { findNavItem } from './components/layout/nav.config'
import { api } from './api/client'
import type { UpdateCheckResult } from './api/client'

const TAB_IDS = [
  'dashboard',
  'battlegroup',
  'players',
  'database',
  'logs',
  'blueprints',
  'bases',
  'guilds',
  'landsraad',
  'storage',
  'livemap',
  'server',
  'market',
  'welcome',
] as const
type TabId = (typeof TAB_IDS)[number]
const DEFAULT_TAB: TabId = 'dashboard'

function currentTabFromPath(pathname: string): TabId {
  const seg = pathname.replace(/^\//, '').split('/')[0]
  return (TAB_IDS as readonly string[]).includes(seg) ? (seg as TabId) : DEFAULT_TAB
}

type DbSection = 'tables' | 'describe' | 'sample' | 'search' | 'sql'
type WelcomeSection = 'config' | 'packages' | 'grants'

// Memoized at module level so identity is stable — prevents all inactive tabs from
// re-rendering whenever AppCore re-renders (e.g. router location change, useStatus poll).
const MBattlegroupTab = memo(BattlegroupTab)
const MLiveMapTab = memo(LiveMapTab)
const MPlayersTab = memo(PlayersTab)
const MDatabaseTab = memo(DatabaseTab)
const MLogsTab = memo(LogsTab)
const MBlueprintsTab = memo(BlueprintsTab)
const MBasesTab = memo(BasesTab)
const MGuildsTab = memo(GuildsTab)
const MLandsraadTab = memo(LandsraadTab)
const MStorageTab = memo(StorageTab)
const MServerSettingsTab = memo(ServerSettingsTab)
const MMarketTab = memo(MarketTab)
const MWelcomePackageTab = memo(WelcomePackageTab)
const MDashboardTab = memo(DashboardTab)

const hasClerk = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY

interface AppCoreProps {
  isSignedIn: boolean
}

interface TabPaneProps {
  active: boolean
  children: ReactNode
}

function AppWithAuth() {
  const { isSignedIn } = useAuth()
  return <AppCore isSignedIn={!!isSignedIn} />
}

export const App: React.FC = () => {
  return hasClerk ? <AppWithAuth /> : <AppCore isSignedIn={true} />
}

const AppCore: React.FC<AppCoreProps> = ({ isSignedIn }) => {
  const status = useStatus()
  const location = useLocation()
  const navigate = useNavigate()
  const { t, i18n } = useTranslation()
  const [reconnecting, setReconnecting] = useState(false)

  const [dbSection, setDbSection] = useState<DbSection>('tables')
  const [welcomeSection, setWelcomeSection] = useState<WelcomeSection>('config')
  const [sidebarCollapsed, setSidebarCollapsed] = useState(
    () => localStorage.getItem('dune_admin_sidebar') === 'collapsed',
  )
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [showBackendConfig, setShowBackendConfig] = useState(false)
  const [updateInfo, setUpdateInfo] = useState<UpdateCheckResult | null>(null)
  const [showUpdateModal, setShowUpdateModal] = useState(false)
  const [updateChecking, setUpdateChecking] = useState(false)
  const [updateApplying, setUpdateApplying] = useState(false)
  const [formSaving, setFormSaving] = useState(false)
  const formSaveRef = useRef<(() => Promise<void>) | null>(null)

  // Re-establish backend connections (DB + control plane) without a service
  // restart — used by the topbar Reconnect button when the DB shows disconnected
  // (e.g. dune-admin came up before the database was ready).
  const handleReconnect = async () => {
    setReconnecting(true)
    try {
      const s = await api.reconnect()
      if (s.db_connected) toast.success(t('app.reconnected'))
      else toast.danger(t('app.reconnectFailed', { error: 'database still unreachable' }))
    }
    catch (e) {
      toast.danger(t('app.reconnectFailed', { error: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setReconnecting(false)
    }
  }

  useEffect(() => {
    const seg = location.pathname.replace(/^\//, '').split('/')[0]
    if (!seg || !(TAB_IDS as readonly string[]).includes(seg)) {
      navigate(`/${DEFAULT_TAB}`, { replace: true })
    }
  }, [location.pathname, navigate])

  const currentTab = currentTabFromPath(location.pathname)

  // Tracks which tabs have been visited at least once — they get mounted and stay
  // mounted (TabPane keeps them hidden), preserving in-tab state and the isActive
  // auto-refresh contract. Unvisited tabs never mount, avoiding the startup query storm.
  const [mounted, setMounted] = useState<Set<TabId>>(() => new Set<TabId>([currentTab]))
  useEffect(() => {
    setMounted((prev) => { // eslint-disable-line react-hooks/set-state-in-effect
      if (prev.has(currentTab)) return prev
      const next = new Set(prev)
      next.add(currentTab)
      return next
    })
  }, [currentTab])

  // Check for a newer release via the backend (it knows this build's version and
  // returns the release-notes URL) — drives the clickable topbar update widget (#129).
  useEffect(() => {
    api.update.check().then(setUpdateInfo).catch(() => {})
  }, [])

  // ⌘K / Ctrl+K toggles the command palette.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setPaletteOpen((v) => !v)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
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
        toast.success(force ? t('app.reinstalled', { version: result.version ?? 'latest' }) : t('app.updated', { version: result.version ?? 'latest' }))
        setUpdateInfo(null)
        setTimeout(() => {
          window.location.reload()
        }, 1500)
      }
      else {
        toast.info(result.message)
      }
    }
    catch (e) {
      toast.danger(t('app.updateFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setUpdateApplying(false)
    }
  }

  const renderTab = (id: TabId, node: ReactNode) => (
    <TabPane active={currentTab === id}>
      {mounted.has(id) ? node : null}
    </TabPane>
  )

  const onNavigate = (tab: string) => navigate(`/${tab}`)
  const onSubNavigate = (section: 'database' | 'welcome', key: string) => {
    if (section === 'database') {
      setDbSection(key as DbSection)
      if (currentTab !== 'database') navigate('/database')
    }
    else {
      setWelcomeSection(key as WelcomeSection)
      if (currentTab !== 'welcome') navigate('/welcome')
    }
  }
  const toggleCollapse = () => {
    const next = !sidebarCollapsed
    setSidebarCollapsed(next)
    localStorage.setItem('dune_admin_sidebar', next ? 'collapsed' : 'expanded')
  }

  const navItem = findNavItem(currentTab)
  // Widen the strictly-typed t() for the dynamic nav key from config.
  const tr = t as unknown as (key: string) => string
  const title = navItem ? (navItem.labelKey ? tr(navItem.labelKey) : navItem.literal ?? '') : ''

  return (
    // Keyed on the active language so switching language remounts the content
    // subtree once. The module-level memo() tabs stay mounted and otherwise keep
    // stale-language text on a language change (their props don't change), until
    // an unrelated local state update forces them to re-render (#123).
    <div key={i18n.language} className="h-screen flex overflow-hidden bg-background">
      {/* HeroUI toasts (un-migrated tabs) + sonner toasts (migrated tabs) coexist
          until the last @heroui/react toast import is gone. */}
      <Toast.Provider />
      <Toaster />

      {/* Persistent sidebar on md+, drawer on mobile */}
      <aside className="hidden shrink-0 border-r border-border md:flex">
        <Sidebar
          currentTab={currentTab}
          dbSection={dbSection}
          welcomeSection={welcomeSection}
          onNavigate={onNavigate}
          onSubNavigate={onSubNavigate}
          collapsed={sidebarCollapsed}
          onToggleCollapse={toggleCollapse}
        />
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar
          title={title}
          status={status}
          reconnecting={reconnecting}
          onReconnect={handleReconnect}
          updateInfo={updateInfo}
          onOpenUpdate={() => setShowUpdateModal(true)}
          onOpenSettings={() => setShowBackendConfig(true)}
          onOpenSearch={() => setPaletteOpen(true)}
          onOpenMobileNav={() => setMobileNavOpen(true)}
        />
        <main className="min-h-0 flex-1 overflow-hidden p-3">
          {renderTab('dashboard', <MDashboardTab isActive={currentTab === 'dashboard'} />)}
          {renderTab('battlegroup', <MBattlegroupTab isActive={currentTab === 'battlegroup'} />)}
          {renderTab('players', <MPlayersTab isActive={currentTab === 'players'} />)}
          {renderTab('database', <MDatabaseTab section={dbSection} />)}
          {renderTab('logs', <MLogsTab control={status?.control} />)}
          {renderTab('blueprints', <MBlueprintsTab isSignedIn={isSignedIn} />)}
          {renderTab('bases', <MBasesTab isSignedIn={isSignedIn} />)}
          {renderTab('guilds', <MGuildsTab isSignedIn={isSignedIn} />)}
          {renderTab('landsraad', <MLandsraadTab />)}
          {renderTab('storage', <MStorageTab />)}
          {renderTab('livemap', <MLiveMapTab isActive={currentTab === 'livemap'} />)}
          {renderTab('server', <MServerSettingsTab />)}
          {renderTab('market', <MMarketTab />)}
          {renderTab('welcome', <MWelcomePackageTab section={welcomeSection} />)}
        </main>
      </div>

      <MobileNav
        open={mobileNavOpen}
        onOpenChange={setMobileNavOpen}
        currentTab={currentTab}
        dbSection={dbSection}
        welcomeSection={welcomeSection}
        onNavigate={onNavigate}
        onSubNavigate={onSubNavigate}
      />
      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} onNavigate={onNavigate} />

      {/* Settings modal — structure mirrors BotControlPanel */}
      <Modal>
        <Modal.Backdrop isOpen={showBackendConfig} onOpenChange={(v) => !v && setShowBackendConfig(false)}>
          <Modal.Container size="cover" scroll="outside">
            <Modal.Dialog className="h-[92vh] flex flex-col">
              <Modal.CloseTrigger />
              <Modal.Header>
                <div className="flex items-baseline gap-6 flex-wrap">
                  <Modal.Heading className="text-accent">{t('app.settings')}</Modal.Heading>
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
                          {t('common.checking')}
                        </>
                      )
                    : t('app.checkUpdates')}
                </Button>
                {updateInfo && !updateInfo.needs_update && (
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={() => applyUpdate(true)}
                    isDisabled={updateApplying}
                  >
                    {updateApplying ? <Spinner size="sm" color="current" /> : t('app.reinstall')}
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
                <span className="text-xs text-muted">{t('app.changesNote')}</span>
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
                          {t('common.saving')}
                        </>
                      )
                    : (
                        <>
                          <Icon name="save" />
                          {' '}
                          {t('app.saveApply')}
                        </>
                      )}
                </Button>
                <Button
                  size="sm"
                  variant="tertiary"
                  onPress={() => setShowBackendConfig(false)}
                >
                  {t('common.close')}
                </Button>
              </Modal.Footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>

      {/* Update-available prompt — opened from the topbar release widget (#129).
          Reuses the backend update check for the release-notes link + Continue/Cancel. */}
      <Modal>
        <Modal.Backdrop isOpen={showUpdateModal} onOpenChange={(v) => !v && setShowUpdateModal(false)}>
          <Modal.Container size="sm">
            <Modal.Dialog>
              <Modal.CloseTrigger />
              <Modal.Header>
                <Modal.Heading className="text-accent">{t('app.updateAvailable')}</Modal.Heading>
              </Modal.Header>
              <Modal.Body className="flex flex-col gap-3">
                <p className="text-sm text-muted">
                  {t('app.updateAvailableBody', {
                    current: updateInfo?.current ?? '',
                    latest: updateInfo?.latest?.replace(/^v/, '') ?? '',
                  })}
                </p>
                {updateInfo?.release_url && (
                  <a
                    href={updateInfo.release_url}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-1 text-sm text-accent hover:opacity-80"
                  >
                    <Icon name="external-link" />
                    {' '}
                    {t('app.viewReleaseNotes')}
                  </a>
                )}
              </Modal.Body>
              <Modal.Footer className="flex items-center justify-end gap-2">
                <Button
                  size="sm"
                  variant="tertiary"
                  onPress={() => setShowUpdateModal(false)}
                >
                  {t('common.cancel')}
                </Button>
                <Button
                  size="sm"
                  onPress={() => {
                    setShowUpdateModal(false)
                    void applyUpdate()
                  }}
                  isDisabled={updateApplying}
                >
                  {updateApplying ? <Spinner size="sm" color="current" /> : t('app.updateNow')}
                </Button>
              </Modal.Footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
    </div>
  )
}

function TabPane({ active, children }: TabPaneProps) {
  return (
    <div className={`h-full min-h-0 ${active ? 'flex flex-col dune-tab-active' : 'hidden'}`}>
      {children}
    </div>
  )
}
