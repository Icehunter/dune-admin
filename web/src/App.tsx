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
import { UserMenu } from './components/UserMenu'
const BattlegroupTab = React.lazy(() => import('./tabs/BattlegroupTab').then((m) => ({ default: m.BattlegroupTab })))
const LiveMapTab = React.lazy(() => import('./tabs/LiveMapTab').then((m) => ({ default: m.LiveMapTab })))
const PlayersTab = React.lazy(() => import('./tabs/PlayersTab').then((m) => ({ default: m.PlayersTab })))
const DatabaseTab = React.lazy(() => import('./tabs/DatabaseTab').then((m) => ({ default: m.DatabaseTab })))
const LogsTab = React.lazy(() => import('./tabs/LogsTab').then((m) => ({ default: m.LogsTab })))
const BlueprintsTab = React.lazy(() => import('./tabs/BlueprintsTab').then((m) => ({ default: m.BlueprintsTab })))
const BasesTab = React.lazy(() => import('./tabs/BasesTab').then((m) => ({ default: m.BasesTab })))
const GuildsTab = React.lazy(() => import('./tabs/GuildsTab').then((m) => ({ default: m.GuildsTab })))
const LandsraadTab = React.lazy(() => import('./tabs/LandsraadTab').then((m) => ({ default: m.LandsraadTab })))
const StorageTab = React.lazy(() => import('./tabs/StorageTab').then((m) => ({ default: m.StorageTab })))
const ServerSettingsTab = React.lazy(() => import('./tabs/ServerSettingsTab').then((m) => ({ default: m.ServerSettingsTab })))
const DirectorTab = React.lazy(() => import('./tabs/DirectorTab').then((m) => ({ default: m.DirectorTab })))
const MarketTab = React.lazy(() => import('./tabs/MarketTab').then((m) => ({ default: m.MarketTab })))
const WelcomePackageTab = React.lazy(() => import('./tabs/WelcomePackageTab').then((m) => ({ default: m.WelcomePackageTab })))
const EventsTab = React.lazy(() => import('./tabs/EventsTab').then((m) => ({ default: m.EventsTab })))
const BattlepassTab = React.lazy(() => import('./tabs/BattlepassTab').then((m) => ({ default: m.BattlepassTab })))
const PermissionsTab = React.lazy(() => import('./tabs/PermissionsTab').then((m) => ({ default: m.PermissionsTab })))
import { Icon } from './dune-ui'
import { AuthContext } from './auth/context'
import { LoginPage } from './auth/LoginPage'
import { usePermissions } from './hooks/usePermissions'
import { api } from './api/client'
import type { UpdateCheckResult } from './api/client'
import type { TabId, DbSection, WelcomeSection, AppCoreProps, ConnectionBadgeProps } from './types'
import { canSeeTabByControlPlane } from './tabNav'

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
  'battlepass',
  'permissions',
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
  battlepass: 'medal',
  permissions: 'lock',
}

// Read-level capability required to see each tab when backend auth is on.
// 'owner' is special: only owners (guild owner, configured owners, local
// account) see the Permissions tab.
const TAB_CAPABILITIES: Record<TabId, string> = {
  battlegroup: 'server:read',
  logs: 'logs:read',
  database: 'database:read',
  server: 'config:read',
  director: 'config:read',
  players: 'players:read',
  livemap: 'world:read',
  storage: 'world:read',
  bases: 'world:read',
  guilds: 'players:read',
  landsraad: 'players:read',
  blueprints: 'world:read',
  market: 'market:read',
  welcome: 'welcome:read',
  events: 'events:read',
  battlepass: 'battlepass:track',
  permissions: 'owner',
}

const BETA_TABS = new Set<TabId>(['events', 'battlepass'])

const hasClerk = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY

const AppWithAuth: React.FC = () => {
  const { isSignedIn } = useAuth()
  return <AppCore isSignedIn={!!isSignedIn} />
}

export const App: React.FC = () => {
  const auth = React.useContext(AuthContext)

  // Backend auth gate (self-host login). Independent of Clerk, which only
  // exists on the hosted CDN deploy. When auth is disabled (default) this
  // renders exactly the pre-auth app.
  if (auth.loading) {
    return (
      <div className="h-screen flex items-center justify-center bg-background">
        <Spinner />
      </div>
    )
  }
  if (auth.enabled && !auth.session) {
    return (
      <>
        <Toast.Provider />
        <LoginPage />
      </>
    )
  }

  return hasClerk ? <AppWithAuth /> : <AppCore isSignedIn={true} />
}

const AppCore: React.FC<AppCoreProps> = ({ isSignedIn }) => {
  const { status, state: connState } = useStatus()
  const location = useLocation()
  const navigate = useNavigate()
  const { t, i18n } = useTranslation()
  const [reconnecting, setReconnecting] = React.useState(false)
  const { can, isOwner, enabled: authEnabled } = usePermissions()

  // Whether a tab is visible for the current session. All-true when backend
  // auth is disabled.
  const canSeeTab = React.useCallback((key: TabId) => {
    if (!canSeeTabByControlPlane(key, status?.control)) return false
    const cap = TAB_CAPABILITIES[key]
    if (cap === 'owner') return authEnabled && (isOwner || can('auth:manage'))
    return can(cap)
  }, [authEnabled, isOwner, can, status?.control])

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
        { key: 'permissions' as TabId, label: t('nav.permissions') },
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
        { key: 'battlepass' as TabId, label: t('nav.battlepass') },
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

  // Nav groups filtered to what the session may see; empty groups disappear.
  const visibleNavGroups = NAV_GROUPS
    .map((g) => ({ ...g, items: g.items.filter((i) => canSeeTab(i.key)) }))
    .filter((g) => g.items.length > 0)
  const firstVisibleTab = visibleNavGroups[0]?.items[0]?.key ?? DEFAULT_TAB

  React.useEffect(() => {
    const seg = location.pathname.replace(/^\//, '').split('/')[0]
    if (!seg || !(TAB_IDS as readonly string[]).includes(seg) || !canSeeTab(seg as TabId)) {
      navigate(`/${firstVisibleTab}`, { replace: true })
    }
  }, [location.pathname, navigate, canSeeTab, firstVisibleTab])

  const currentTab = currentTabFromPath(location.pathname)
  const pathname = location.pathname

  const UPDATE_CACHE_KEY = 'dune_update_cache'
  const UPDATE_CACHE_TTL_MS = 60 * 60 * 1000

  // Check for a newer release via the backend — cached in localStorage for 1 hour
  // to avoid hammering GitHub's unauthenticated API rate limit during dev HMR cycles.
  React.useEffect(() => {
    try {
      const cached = localStorage.getItem(UPDATE_CACHE_KEY)
      if (cached) {
        const { ts, data } = JSON.parse(cached)
        if (Date.now() - ts < UPDATE_CACHE_TTL_MS) {
          Promise.resolve().then(() => setUpdateInfo(data))
          return
        }
      }
    }
    catch { /* ignore corrupt cache */ }
    api.update.check().then((data) => {
      setUpdateInfo(data)
      try {
        localStorage.setItem(UPDATE_CACHE_KEY, JSON.stringify({ ts: Date.now(), data }))
      }
      catch { /* ignore */ }
    }).catch(() => {})
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const checkUpdate = async () => {
    setUpdateChecking(true)
    try {
      const data = await api.update.check()
      setUpdateInfo(data)
      try {
        localStorage.setItem(UPDATE_CACHE_KEY, JSON.stringify({ ts: Date.now(), data }))
      }
      catch { /* ignore */ }
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
        localStorage.removeItem(UPDATE_CACHE_KEY)
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

  const renderTab = (id: TabId, node: React.ReactNode) => {
    if (currentTab !== id) return null
    return (
      <React.Suspense fallback={<div className="flex-1 flex items-center justify-center"><Spinner /></div>}>
        <div className="flex-1 flex flex-col min-h-0">
          {node}
        </div>
      </React.Suspense>
    )
  }

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
        <Sidebar.MenuLabel className="flex items-center">
          {label}
          {BETA_TABS.has(key) && (
            <Chip size="sm" color="accent" variant="soft" className="ml-1 text-[9px] h-4 px-1 min-w-0 shrink-0 self-center">{t('common.beta')}</Chip>
          )}
        </Sidebar.MenuLabel>
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
          {visibleNavGroups.map((group) => (
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
          {can('server:control') && status && !status.db_connected && (
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

          {can('config:read') && (
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
          )}

          <UserMenu />

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
      {renderTab('battlegroup', <BattlegroupTab />)}
      {renderTab('players', <PlayersTab />)}
      {renderTab('database', <DatabaseTab section={dbSection} />)}
      {renderTab('logs', <LogsTab control={status?.control} />)}
      {renderTab('blueprints', <BlueprintsTab isSignedIn={isSignedIn} />)}
      {renderTab('bases', <BasesTab isSignedIn={isSignedIn} />)}
      {renderTab('guilds', <GuildsTab isSignedIn={isSignedIn} />)}
      {renderTab('landsraad', <LandsraadTab />)}
      {renderTab('storage', <StorageTab />)}
      {renderTab('livemap', <LiveMapTab />)}
      {renderTab('server', <ServerSettingsTab />)}
      {renderTab('director', <DirectorTab />)}
      {renderTab('market', <MarketTab />)}
      {renderTab('welcome', <WelcomePackageTab section={welcomeSection} />)}
      {renderTab('events', <EventsTab />)}
      {renderTab('battlepass', <BattlepassTab />)}
      {canSeeTab('permissions') && renderTab('permissions', <PermissionsTab />)}
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
          <Modal.Dialog className="p-10 dialog-surface-alt">
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
            <Modal.Body className="flex flex-col overflow-y-auto h-[80vh] min-h-0 pr-1">
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
              {can('server:control') && updateInfo && !updateInfo.needs_update && (
                <Button
                  size="sm"
                  variant="ghost"
                  onPress={() => applyUpdate(true)}
                  isDisabled={updateApplying}
                >
                  {updateApplying ? <Spinner size="sm" color="current" /> : t('app.reinstall')}
                </Button>
              )}
              {can('server:control') && updateInfo?.needs_update && (
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
              {can('server:control') && (
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
              )}
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
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
