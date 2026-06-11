import * as React from 'react'
import { Show, SignInButton, UserButton, useAuth } from '@clerk/react'
import { Button, Chip, Modal, Spinner, Toast, toast } from '@heroui/react'
import { AppLayout, Navbar, Sidebar } from '@heroui-pro/react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useStatus } from './hooks/useStatus'
import { BackendUnreachable } from './components/BackendUnreachable'
import { SettingsConfigForm } from './components/SettingsConfigForm'
import { LanguageSelector } from './components/LanguageSelector'
import { ThemeSelector } from './components/ThemeSelector'
import { HelpMenu } from './components/HelpMenu'
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
import { DirectorTab } from './tabs/DirectorTab'
import { MarketTab } from './tabs/MarketTab'
import { WelcomePackageTab } from './tabs/WelcomePackageTab'
import { EventsTab } from './tabs/EventsTab'
import { Icon } from './dune-ui'
import { api } from './api/client'
import type { UpdateCheckResult } from './api/client'
import type { TabId, DbSection, WelcomeSection, AppCoreProps, TabPaneProps, ConnectionBadgeProps } from './types'

const TAB_IDS = [
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
  'director',
  'market',
  'welcome',
  'events',
] as const
const DEFAULT_TAB: TabId = 'battlegroup'

const currentTabFromPath = (pathname: string): TabId => {
  const seg = pathname.replace(/^\//, '').split('/')[0]
  return (TAB_IDS as readonly string[]).includes(seg) ? (seg as TabId) : DEFAULT_TAB
}

// Lucide icon per top-level tab, shown in the collapsible sidebar rail.
const TAB_ICONS: Record<TabId, string> = {
  battlegroup: 'activity',
  logs: 'scroll-text',
  database: 'database',
  server: 'settings-2',
  director: 'clapperboard',
  players: 'users',
  livemap: 'map',
  storage: 'package',
  bases: 'house',
  guilds: 'shield',
  landsraad: 'landmark',
  blueprints: 'scroll',
  market: 'store',
  welcome: 'gift',
  events: 'calendar-clock',
}

// Memoized at module level so identity is stable — prevents all inactive tabs from
// re-rendering whenever AppCore re-renders (e.g. router location change, useStatus poll).
const BattlegroupTabMemo = React.memo(BattlegroupTab)
const LiveMapTabMemo = React.memo(LiveMapTab)
const PlayersTabMemo = React.memo(PlayersTab)
const DatabaseTabMemo = React.memo(DatabaseTab)
const LogsTabMemo = React.memo(LogsTab)
const BlueprintsTabMemo = React.memo(BlueprintsTab)
const BasesTabMemo = React.memo(BasesTab)
const GuildsTabMemo = React.memo(GuildsTab)
const LandsraadTabMemo = React.memo(LandsraadTab)
const StorageTabMemo = React.memo(StorageTab)
const ServerSettingsTabMemo = React.memo(ServerSettingsTab)
const DirectorTabMemo = React.memo(DirectorTab)
const MarketTabMemo = React.memo(MarketTab)
const WelcomePackageTabMemo = React.memo(WelcomePackageTab)
const EventsTabMemo = React.memo(EventsTab)

const hasClerk = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY

const AppWithAuth: React.FC = () => {
  const { isSignedIn } = useAuth()
  return <AppCore isSignedIn={!!isSignedIn} />
}

export const App: React.FC = () => {
  return hasClerk ? <AppWithAuth /> : <AppCore isSignedIn={true} />
}

const AppCore: React.FC<AppCoreProps> = ({ isSignedIn }) => {
  const { status, state: connState } = useStatus()
  const location = useLocation()
  const navigate = useNavigate()
  const { t, i18n } = useTranslation()
  const [reconnecting, setReconnecting] = React.useState(false)

  const DB_SECTIONS: { key: DbSection, label: string, icon: string }[] = [
    { key: 'backups', label: t('database.sections.backups'), icon: 'archive' },
    { key: 'tables', label: t('database.sections.tables'), icon: 'table' },
    { key: 'describe', label: t('database.sections.describe'), icon: 'file-text' },
    { key: 'sample', label: t('database.sections.sample'), icon: 'flask-conical' },
    { key: 'search', label: t('database.sections.search'), icon: 'search' },
    { key: 'sql', label: t('database.sections.sql'), icon: 'terminal' },
  ]

  const WELCOME_SECTIONS: { key: WelcomeSection, label: string, icon: string }[] = [
    { key: 'config', label: t('welcome.sections.config'), icon: 'sliders-horizontal' },
    { key: 'packages', label: t('welcome.sections.packages'), icon: 'package' },
    { key: 'grants', label: t('welcome.sections.grants'), icon: 'gift' },
  ]

  // Re-establish backend connections (DB + control plane) without a service
  // restart — used by the navbar Reconnect button when the DB shows disconnected
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

  // Left-sidebar navigation, grouped to mirror the product's structure
  // (operator tooling today; a Player Portal group lands here later).
  const NAV_GROUPS: { title: string, items: { key: TabId, label: string }[] }[] = [
    {
      title: t('nav.groups.operations'),
      items: [
        { key: 'battlegroup' as TabId, label: t('nav.battlegroup') },
        { key: 'logs' as TabId, label: t('nav.logs') },
        { key: 'database' as TabId, label: t('nav.database') },
        { key: 'server' as TabId, label: t('nav.server') },
        { key: 'director' as TabId, label: t('nav.director') },
      ],
    },
    {
      title: t('nav.groups.playerWorld'),
      items: [
        { key: 'players' as TabId, label: t('nav.players') },
        { key: 'livemap' as TabId, label: t('nav.liveMap') },
        { key: 'storage' as TabId, label: t('nav.storage') },
        { key: 'bases' as TabId, label: t('nav.bases') },
        { key: 'guilds' as TabId, label: t('nav.guilds') },
        { key: 'landsraad' as TabId, label: t('nav.landsraad') },
        { key: 'blueprints' as TabId, label: t('nav.blueprints') },
      ],
    },
    {
      title: t('nav.groups.economy'),
      items: [
        { key: 'market' as TabId, label: t('nav.market') },
        { key: 'welcome' as TabId, label: t('nav.welcome') },
        { key: 'events' as TabId, label: t('nav.events') },
      ],
    },
  ]

  const [dbSection, setDbSection] = React.useState<DbSection>('backups')
  const [welcomeSection, setWelcomeSection] = React.useState<WelcomeSection>('config')
  const [showBackendConfig, setShowBackendConfig] = React.useState(false)
  const [updateInfo, setUpdateInfo] = React.useState<UpdateCheckResult | null>(null)
  const [showUpdateModal, setShowUpdateModal] = React.useState(false)
  const [updateChecking, setUpdateChecking] = React.useState(false)
  const [updateApplying, setUpdateApplying] = React.useState(false)
  const [formSaving, setFormSaving] = React.useState(false)
  const formSaveRef = React.useRef<(() => Promise<void>) | null>(null)

  React.useEffect(() => {
    const seg = location.pathname.replace(/^\//, '').split('/')[0]
    if (!seg || !(TAB_IDS as readonly string[]).includes(seg)) {
      navigate(`/${DEFAULT_TAB}`, { replace: true })
    }
  }, [location.pathname, navigate])

  const currentTab = currentTabFromPath(location.pathname)
  const pathname = location.pathname

  // Tracks which tabs have been visited at least once — they get mounted and stay
  // mounted (TabPane keeps them hidden), preserving in-tab state and the isActive
  // auto-refresh contract. Unvisited tabs never mount, avoiding the startup query storm.
  const [mounted, setMounted] = React.useState<Set<TabId>>(() => new Set<TabId>([currentTab]))
  React.useEffect(() => {
    setMounted((prev) => { // eslint-disable-line react-hooks/set-state-in-effect
      if (prev.has(currentTab)) return prev
      const next = new Set(prev)
      next.add(currentTab)
      return next
    })
  }, [currentTab])

  // Check for a newer release via the backend — cached in localStorage for 1 hour
  // to avoid hammering GitHub's unauthenticated API rate limit during dev HMR cycles.
  React.useEffect(() => {
    const CACHE_KEY = 'dune_update_cache'
    const TTL_MS = 60 * 60 * 1000
    try {
      const cached = localStorage.getItem(CACHE_KEY)
      if (cached) {
        const { ts, data } = JSON.parse(cached)
        if (Date.now() - ts < TTL_MS) {
          Promise.resolve().then(() => setUpdateInfo(data))
          return
        }
      }
    }
    catch { /* ignore corrupt cache */ }
    api.update.check().then((data) => {
      setUpdateInfo(data)
      try {
        localStorage.setItem(CACHE_KEY, JSON.stringify({ ts: Date.now(), data }))
      }
      catch { /* ignore */ }
    }).catch(() => {})
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

  const renderTab = (id: TabId, node: React.ReactNode) => (
    <TabPane active={currentTab === id}>
      {mounted.has(id) ? node : null}
    </TabPane>
  )

  // #165: when the SPA never reached the backend, show an informative setup
  // screen instead of an empty, non-working dashboard.
  if (connState === 'error') {
    return <BackendUnreachable onRetry={() => window.location.reload()} />
  }

  // A single top-level menu item. When the item carries sub-sections (Database,
  // Welcome Kits) we render a Submenu so the sections expand inline under it.
  const menuItem = (key: TabId) => {
    const label = NAV_GROUPS.flatMap((g) => g.items).find((i) => i.key === key)?.label ?? key
    const icon = <Sidebar.MenuIcon><Icon name={TAB_ICONS[key]} /></Sidebar.MenuIcon>

    if (key === 'database') {
      return (
        <Sidebar.MenuItem key={key} id={key} href={`/${key}`} isCurrent={currentTab === 'database'} onAction={() => navigate(`/${key}`)}>
          {icon}
          <Sidebar.MenuLabel>{label}</Sidebar.MenuLabel>
          <Sidebar.MenuTrigger><Sidebar.MenuIndicator /></Sidebar.MenuTrigger>
          <Sidebar.Submenu>
            {DB_SECTIONS.map((s) => (
              <Sidebar.MenuItem
                key={s.key}
                id={`db:${s.key}`}
                isCurrent={currentTab === 'database' && dbSection === s.key}
                onAction={() => {
                  setDbSection(s.key)
                  navigate('/database')
                }}
              >
                <Sidebar.MenuIcon><Icon name={s.icon} /></Sidebar.MenuIcon>
                <Sidebar.MenuLabel>{s.label}</Sidebar.MenuLabel>
              </Sidebar.MenuItem>
            ))}
          </Sidebar.Submenu>
        </Sidebar.MenuItem>
      )
    }

    if (key === 'welcome') {
      return (
        <Sidebar.MenuItem key={key} id={key} href={`/${key}`} isCurrent={currentTab === 'welcome'} onAction={() => navigate(`/${key}`)}>
          {icon}
          <Sidebar.MenuLabel>{label}</Sidebar.MenuLabel>
          <Sidebar.MenuTrigger><Sidebar.MenuIndicator /></Sidebar.MenuTrigger>
          <Sidebar.Submenu>
            {WELCOME_SECTIONS.map((s) => (
              <Sidebar.MenuItem
                key={s.key}
                id={`welcome:${s.key}`}
                isCurrent={currentTab === 'welcome' && welcomeSection === s.key}
                onAction={() => {
                  setWelcomeSection(s.key)
                  navigate('/welcome')
                }}
              >
                <Sidebar.MenuIcon><Icon name={s.icon} /></Sidebar.MenuIcon>
                <Sidebar.MenuLabel>{s.label}</Sidebar.MenuLabel>
              </Sidebar.MenuItem>
            ))}
          </Sidebar.Submenu>
        </Sidebar.MenuItem>
      )
    }

    return (
      <Sidebar.MenuItem key={key} id={key} href={`/${key}`} isCurrent={pathname === `/${key}`} onAction={() => navigate(`/${key}`)}>
        {icon}
        <Sidebar.MenuLabel>{label}</Sidebar.MenuLabel>
      </Sidebar.MenuItem>
    )
  }

  const sidebar = (
    <Sidebar>
      <Sidebar.Header>
        <Button
          variant="ghost"
          className="flex items-center gap-2 px-2 h-10 min-w-0 hover:opacity-80 w-full justify-start"
          onPress={() => navigate(`/${DEFAULT_TAB}`)}
          aria-label={t('app.goHome')}
        >
          <img
            src="/dune-admin-logo-small.svg"
            alt="Dune Admin"
            className="size-6 shrink-0"
          />
          <span
            data-sidebar="label"
            className="text-sm font-bold uppercase tracking-[0.2em] text-accent overflow-hidden whitespace-nowrap"
          >
            {t('app.title')}
          </span>
        </Button>
      </Sidebar.Header>
      <Sidebar.Content>
        <Sidebar.Menu aria-label={t('nav.menu')}>
          {NAV_GROUPS.map((group) => (
            <Sidebar.MenuSection key={group.title}>
              <Sidebar.MenuHeader>{group.title}</Sidebar.MenuHeader>
              {group.items.map((item) => menuItem(item.key))}
            </Sidebar.MenuSection>
          ))}
        </Sidebar.Menu>
      </Sidebar.Content>
    </Sidebar>
  )

  const navbar = (
    <Navbar position="sticky" maxWidth="full">
      <Navbar.Header>
        <Sidebar.Trigger />
        <div className="flex items-center gap-3">
          {status?.control && status.control !== 'none' && <span className="text-xs text-muted">{status.control}</span>}
          {status?.ssh_host && <span className="text-xs text-muted">{status.ssh_host}</span>}
          {status?.db_host && status.control !== 'kubectl' && (
            <span className="text-xs text-muted">{status.db_host}</span>
          )}
          {status?.version && (
            <Button
              variant="ghost"
              className="text-xs text-muted hover:text-foreground px-0 h-auto min-w-0"
              onPress={() => setShowBackendConfig(true)}
              aria-label={t('app.openSettings')}
            >
              v
              {status.version}
            </Button>
          )}
          {updateInfo?.needs_update && (
            <Button
              variant="ghost"
              onPress={() => setShowUpdateModal(true)}
              aria-label={t('app.updateAvailable')}
              className="cursor-pointer p-0 h-auto min-w-0"
            >
              <Chip size="sm" color="warning" variant="soft">
                ↑
                {' '}
                {updateInfo.latest.replace(/^v/, '')}
              </Chip>
            </Button>
          )}
        </div>

        <Navbar.Spacer />

        <Navbar.Content>
          {status?.executor === 'ssh' && <ConnectionBadge label="SSH" connected={status.ssh_connected} />}
          <ConnectionBadge label="DB" connected={status?.db_connected ?? false} />
          {status && !status.db_connected && (
            <Button
              size="sm"
              variant="outline"
              isDisabled={reconnecting}
              onPress={handleReconnect}
            >
              {reconnecting ? t('app.reconnecting') : t('app.reconnect')}
            </Button>
          )}
          {status?.pod_ns && (
            <span className="text-xs text-muted">
              ns:
              {status.pod_ns}
            </span>
          )}

          <HelpMenu status={status} />
          <ThemeSelector />
          <LanguageSelector />

          <Button
            size="sm"
            variant="outline"
            aria-label={t('app.configureBackend')}
            onPress={() => setShowBackendConfig((v) => !v)}
            className={showBackendConfig ? 'text-accent border-accent' : ''}
          >
            <Icon name="settings" />
            {' '}
            {t('app.settings')}
          </Button>

          {hasClerk && (
            <>
              <Show when="signed-out">
                <SignInButton>
                  <Button size="sm" variant="outline">
                    {t('app.signIn')}
                  </Button>
                </SignInButton>
              </Show>
              <Show when="signed-in">
                <UserButton />
              </Show>
            </>
          )}
        </Navbar.Content>
      </Navbar.Header>
    </Navbar>
  )

  const tabContent = (
    <main className="flex-1 flex flex-col overflow-hidden min-h-0">
      {renderTab('battlegroup', <BattlegroupTabMemo isActive={currentTab === 'battlegroup'} />)}
      {renderTab('players', <PlayersTabMemo isActive={currentTab === 'players'} />)}
      {renderTab('database', <DatabaseTabMemo section={dbSection} />)}
      {renderTab('logs', <LogsTabMemo control={status?.control} />)}
      {renderTab('blueprints', <BlueprintsTabMemo isSignedIn={isSignedIn} />)}
      {renderTab('bases', <BasesTabMemo isSignedIn={isSignedIn} />)}
      {renderTab('guilds', <GuildsTabMemo isSignedIn={isSignedIn} />)}
      {renderTab('landsraad', <LandsraadTabMemo />)}
      {renderTab('storage', <StorageTabMemo />)}
      {renderTab('livemap', <LiveMapTabMemo isActive={currentTab === 'livemap'} />)}
      {renderTab('server', <ServerSettingsTabMemo />)}
      {renderTab('director', <DirectorTabMemo />)}
      {renderTab('market', <MarketTabMemo />)}
      {renderTab('welcome', <WelcomePackageTabMemo section={welcomeSection} />)}
      {renderTab('events', <EventsTabMemo />)}
    </main>
  )

  return (
    // Keyed on the active language so switching language remounts the content
    // subtree once. The module-level memo() tabs stay mounted and otherwise keep
    // stale-language text on a language change (their props don't change), until
    // an unrelated local state update forces them to re-render (#123).
    <div key={i18n.language} className="h-screen overflow-hidden bg-background">
      <Toast.Provider />

      <AppLayout
        sidebarCollapsible="icon"
        sidebarVariant="inset"
        scrollMode="content"
        navigate={navigate}
        navbar={navbar}
        sidebar={sidebar}
      >
        <div className="h-full flex flex-col p-3 overflow-hidden min-h-0">
          {tabContent}
        </div>
      </AppLayout>

      {/* Settings modal — structure mirrors BotControlPanel */}
      <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={showBackendConfig} onOpenChange={(v) => !v && setShowBackendConfig(false)}>
        <Modal.Container size="cover" scroll="outside">
          <Modal.Dialog className="p-10 h-[92vh] flex flex-col dialog-surface-alt">
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

      {/* Update-available prompt — opened from the navbar release widget (#129).
          Reuses the backend update check for the release-notes link + Continue/Cancel. */}
      <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={showUpdateModal} onOpenChange={(v) => !v && setShowUpdateModal(false)}>
        <Modal.Container size="sm">
          <Modal.Dialog className="p-10">
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
    </div>
  )
}

const TabPane: React.FC<TabPaneProps> = ({ active, children }) => {
  return (
    <div className={`flex-1 min-h-0 ${active ? 'flex flex-col dune-tab-active' : 'hidden'}`}>
      {children}
    </div>
  )
}

const ConnectionBadge: React.FC<ConnectionBadgeProps> = ({ label, connected }) => {
  return (
    <div className="flex items-center gap-1.5 text-xs">
      <div className={`w-2 h-2 rounded-full ${connected ? 'bg-success' : 'bg-muted/40'}`} />
      <span className={connected ? 'text-foreground' : 'text-muted'}>{label}</span>
    </div>
  )
}
