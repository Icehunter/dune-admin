import * as React from 'react'
import { Show, SignInButton, UserButton, useAuth } from '@clerk/react'
import { Button, Chip, ListBox, Modal, Select, Spinner, Toast, toast } from '@heroui/react'
import { AppLayout, Navbar, Sidebar } from '@heroui-pro/react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useStatus } from './hooks/useStatus'
import { BackendUnreachable } from './components/BackendUnreachable'
import { SetupWizard } from './components/SetupWizard'
import { SettingsConfigForm } from './components/SettingsConfigForm'
import { LanguageSelector } from './components/LanguageSelector'
import { ThemeSelector } from './components/ThemeSelector'
import { HelpMenu } from './components/HelpMenu'
import { UserMenu } from './components/UserMenu'

const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms))
const DashboardTab = React.lazy(() => import('./tabs/DashboardTab').then((m) => ({ default: m.DashboardTab })))
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
import { ManageServerModal } from './components/ManageServerModal'
import { useActiveServer } from './context/useActiveServer'
import { AuthContext } from './auth/context'
import { LoginPage } from './auth/LoginPage'
import { usePermissions } from './hooks/usePermissions'
import { api } from './api/client'
import type { UpdateCheckResult } from './api/client'
import type { TabId, AppCoreProps, ConnectionBadgeProps } from './types'
import { canSeeTabByControlPlane } from './tabNav'
import { UpdateProgressModal } from './components/UpdateProgressModal'
import type { UpdatePhase } from './components/UpdateProgressModal'

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
  'director',
  'market',
  'welcome',
  'events',
  'battlepass',
  'permissions',
] as const
const DEFAULT_TAB: TabId = 'dashboard'

const currentTabFromPath = (pathname: string): TabId => {
  const seg = pathname.replace(/^\//, '').split('/')[0]
  return (TAB_IDS as readonly string[]).includes(seg) ? (seg as TabId) : DEFAULT_TAB
}

// Lucide icon per top-level tab, shown in the collapsible sidebar rail.
const TAB_ICONS: Record<TabId, string> = {
  dashboard: 'layout-grid',
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
  dashboard: 'server:read',
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
  const { status, state: connState, refresh: refreshStatus } = useStatus()
  const location = useLocation()
  const navigate = useNavigate()
  const { t, i18n } = useTranslation()
  const [reconnecting, setReconnecting] = React.useState(false)
  const { can, isOwner, enabled: authEnabled } = usePermissions()
  const { servers, activeID, setActive, refresh: refreshServers } = useActiveServer()
  const [addingServer, setAddingServer] = React.useState(false)
  const prevNeedsSetup = React.useRef<boolean | undefined>(undefined)
  React.useEffect(() => {
    if (prevNeedsSetup.current === true && status?.needs_setup === false) {
      void refreshServers()
    }
    prevNeedsSetup.current = status?.needs_setup
  }, [status?.needs_setup, refreshServers])

  // Switching the active server loads a whole new config/context: re-fetch
  // status so the header, control-plane gating and badges reflect the new
  // server. Tab content is remounted via a key on activeID (below) so each tab
  // re-fetches its data with the new X-Dune-Server header — no hard reload.
  const prevActiveID = React.useRef(activeID)
  React.useEffect(() => {
    if (prevActiveID.current !== activeID) {
      prevActiveID.current = activeID
      void refreshStatus()
    }
  }, [activeID, refreshStatus])

  // Whether a tab is visible for the current session. All-true when backend
  // auth is disabled. The Dashboard is always visible (it's the home and the
  // empty-state/onboarding surface); when no servers are configured every other
  // tab is hidden so a fresh install shows only the Dashboard.
  const canSeeTab = React.useCallback((key: TabId) => {
    if (key === 'dashboard') return true
    if (servers.length === 0) return false
    if (!canSeeTabByControlPlane(key, status?.control)) return false
    const cap = TAB_CAPABILITIES[key]
    if (cap === 'owner') return authEnabled && (isOwner || can('auth:manage'))
    return can(cap)
  }, [authEnabled, isOwner, can, status?.control, servers.length])

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
      title: t('nav.groups.dashboard', 'Home'),
      items: [
        { key: 'dashboard' as TabId, label: t('nav.dashboard', 'Dashboard') },
      ],
    },
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

  const [showBackendConfig, setShowBackendConfig] = React.useState(false)
  // Which settings tab to open on (e.g. dashboard onboarding deep-links to
  // 'discord' or 'auth'); undefined → the form's default tab.
  const [settingsTab, setSettingsTab] = React.useState<string | undefined>(undefined)
  // Bumped whenever the global Settings modal closes, so the Dashboard re-syncs
  // onboarding state (e.g. the Discord/auth card disappears once configured).
  const [dashboardRefreshKey, setDashboardRefreshKey] = React.useState(0)
  const openSettings = (tab?: string) => {
    setSettingsTab(tab)
    setShowBackendConfig(true)
  }
  const closeSettings = () => {
    setShowBackendConfig(false)
    setDashboardRefreshKey((n) => n + 1)
  }
  const [updateInfo, setUpdateInfo] = React.useState<UpdateCheckResult | null>(null)
  const [showUpdateModal, setShowUpdateModal] = React.useState(false)
  const [updateChecking, setUpdateChecking] = React.useState(false)
  const [updateApplying, setUpdateApplying] = React.useState(false)
  const [updatePhase, setUpdatePhase] = React.useState<UpdatePhase>('downloading')
  const [updateError, setUpdateError] = React.useState<string | undefined>(undefined)
  const [formSaving, setFormSaving] = React.useState(false)
  const formSaveRef = React.useRef<(() => Promise<void>) | null>(null)

  // Nav groups filtered to what the session may see; empty groups disappear.
  const visibleNavGroups = NAV_GROUPS
    .map((g) => ({ ...g, items: g.items.filter((i) => canSeeTab(i.key)) }))
    .filter((g) => g.items.length > 0)
  const firstVisibleTab = visibleNavGroups[0]?.items[0]?.key ?? DEFAULT_TAB

  // Manage server is a modal (keyed by id in state, not a route) so the URL
  // never carries a server id that would look stale after a rename.
  const [manageServerID, setManageServerID] = React.useState('')

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
    setUpdatePhase('downloading')
    setUpdateError(undefined)
    setShowUpdateModal(false)
    setShowBackendConfig(false)
    try {
      const result = await api.update.apply(force)
      if (!result.updated) {
        toast.info(result.message)
        setUpdateApplying(false)
        return
      }
      localStorage.removeItem(UPDATE_CACHE_KEY)
      setUpdateInfo(null)

      // Binary swapped; server is restarting in ~500ms.
      setUpdatePhase('verifying')
      await sleep(400)
      setUpdatePhase('extracting')
      await sleep(300)
      setUpdatePhase('restarting')

      // Poll /status until the server comes back up (max 90s).
      const started = Date.now()
      const TIMEOUT_MS = 90_000
      const LONG_WAIT_MS = 20_000
      await sleep(2000)
      setUpdatePhase('waiting')

      let back = false
      while (Date.now() - started < TIMEOUT_MS) {
        if (Date.now() - started > LONG_WAIT_MS) {
          setUpdatePhase('waitingLong')
        }
        try {
          await api.status()
          back = true
          break
        }
        catch {
          await sleep(2000)
        }
      }

      if (back) {
        setUpdatePhase('ready')
        await sleep(800)
        window.location.reload()
      }
      else {
        // Timed out but keep polling in background; page will reload on next success.
        setUpdatePhase('waitingLong')
        const keepPolling = async () => {
          while (true) {
            await sleep(3000)
            try {
              await api.status()
              window.location.reload()
              return
            }
            catch { /* keep trying */ }
          }
        }
        void keepPolling()
      }
    }
    catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      setUpdateError(t('app.updateFailed', { message: msg }))
      setUpdatePhase('error')
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

  // No forced first-run wizard. A fresh install (no servers) lands on the
  // Dashboard, which surfaces optional onboarding (add server / set up auth /
  // Discord). The "Add server" wizard is launched on demand as a modal overlay
  // (rendered below, alongside the Settings modal).

  // A single top-level menu item. Sub-sections (Database, Welcome Kits,
  // Battle Pass) live inside their tab via an in-header Segment, so every
  // sidebar item is a plain top-level entry.
  const menuItem = (key: TabId) => {
    const label = NAV_GROUPS.flatMap((g) => g.items).find((i) => i.key === key)?.label ?? key
    const icon = <Sidebar.MenuIcon><Icon name={TAB_ICONS[key]} /></Sidebar.MenuIcon>

    return (
      <Sidebar.MenuItem key={key} id={key} href={`/${key}`} isCurrent={pathname === `/${key}`} onAction={() => navigate(`/${key}`)}>
        {icon}
        <Sidebar.MenuLabel className="flex items-center">
          <Sidebar.MenuItemContent>
            {label}
            {BETA_TABS.has(key) && (
              <Chip size="sm" color="accent" variant="soft" className="ml-1 text-[9px] h-4 px-1 min-w-0 shrink-0 self-center">{t('common.beta')}</Chip>
            )}
          </Sidebar.MenuItemContent>
        </Sidebar.MenuLabel>
      </Sidebar.MenuItem>
    )
  }

  const sidebar = (
    <Sidebar>
      <Sidebar.Header>
        <Button
          variant="ghost"
          className="flex items-center gap-0 px-2 h-14 min-w-0 hover:opacity-80 w-full justify-start"
          onPress={() => navigate(`/${DEFAULT_TAB}`)}
          aria-label={t('app.goHome')}
        >
          <img src="/dune-admin-logo-primary.svg" alt="dune-admin" className="max-h-12 w-auto" />
          <span
            data-sidebar="label"
            className="text-xl font-bold uppercase text-accent overflow-hidden whitespace-nowrap"
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
          {/* Connection info is meaningless with no servers configured (fresh
              install / last server deleted) — hide it then. */}
          {servers.length > 0 && status?.control && status.control !== 'none' && <span className="text-xs text-muted">{status.control}</span>}
          {servers.length > 0 && status?.ssh_host && <span className="text-xs text-muted">{status.ssh_host}</span>}
          {servers.length > 0 && status?.db_host && status.control !== 'kubectl' && (
            <span className="text-xs text-muted">{status.db_host}</span>
          )}
          {status?.version && (
            <Button
              variant="ghost"
              className="text-xs text-muted hover:text-foreground px-0 h-auto min-w-0"
              onPress={() => openSettings()}
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

        {servers.length > 0 && (
          <div className="flex items-center gap-1">
            {/* Always render the dropdown when there is ≥1 server so the navbar
                layout doesn't jump when a second server is added. */}
            <Select
              aria-label="Active server"
              className="w-40"
              selectedKey={activeID || servers[0]?.id}
              onSelectionChange={(id) => {
                if (id && id !== activeID) void setActive(String(id))
              }}
            >
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  {servers.map((s) => (
                    <ListBox.Item key={s.id} id={s.id} textValue={s.name}>
                      {s.name}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
            {can('server:control') && (
              <Button
                size="sm"
                variant="ghost"
                isIconOnly
                aria-label={t('manage.title', 'Manage server')}
                onPress={() => setManageServerID(activeID || servers[0]?.id || 'default')}
              >
                <Icon name="settings" />
              </Button>
            )}
            {can('server:control') && (
              <Button
                size="sm"
                variant="ghost"
                isIconOnly
                aria-label="Add server"
                onPress={() => setAddingServer(true)}
              >
                <Icon name="plus" />
              </Button>
            )}
          </div>
        )}

        <Navbar.Spacer />

        <Navbar.Content>
          {/* Connection badges + reconnect only make sense with a server
              configured — hide them on a fresh/empty install. */}
          {servers.length > 0 && status?.executor === 'ssh' && <ConnectionBadge label="SSH" connected={status.ssh_connected} />}
          {servers.length > 0 && <ConnectionBadge label="DB" connected={status?.db_connected ?? false} />}
          {servers.length > 0 && can('server:control') && status && !status.db_connected && (
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
      {renderTab('dashboard', (
        <DashboardTab
          onAddServer={() => setAddingServer(true)}
          onOpenSettings={openSettings}
          onManageServer={(id) => setManageServerID(id)}
          refreshKey={dashboardRefreshKey}
        />
      ))}
      {renderTab('battlegroup', <BattlegroupTab />)}
      {renderTab('players', <PlayersTab />)}
      {renderTab('database', <DatabaseTab />)}
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
      {renderTab('welcome', <WelcomePackageTab />)}
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
        {/* Keyed by activeID so switching servers remounts every tab — each
            re-fetches its data with the new X-Dune-Server header (no reload). */}
        <div key={activeID} className="h-full flex flex-col p-3 overflow-hidden min-h-0">
          {tabContent}
        </div>
      </AppLayout>

      {/* Settings modal — structure mirrors BotControlPanel */}
      <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={showBackendConfig} onOpenChange={(v) => !v && closeSettings()}>
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
                    {/* Control plane is per-server — it belongs on Manage server,
                        not the global dune-admin Settings modal. */}
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
                <SettingsConfigForm
                  saveRef={formSaveRef}
                  onSavingChange={setFormSaving}
                  globalOnly
                  initialTab={settingsTab}
                />
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
                  {t('app.reinstall')}
                </Button>
              )}
              {can('server:control') && updateInfo?.needs_update && (
                <Button size="sm" onPress={() => applyUpdate()} isDisabled={updateApplying}>
                  <span className="font-mono text-xs">
                    v
                    {updateInfo.current}
                    {' → '}
                    v
                    {updateInfo.latest.replace(/^v/, '')}
                  </span>
                </Button>
              )}

              {/* Spacer */}
              <span className="flex-1" />

              {/* Right: save + close */}
              <span className="text-xs text-muted">{t('app.changesNote')}</span>
              <Button
                size="sm"
                onPress={() => {
                  // Save & apply, then close. Don't block on slow background work
                  // (e.g. the Discord bot can take ~10s to connect). An auth
                  // toggle reloads the page inside save(), so this resolves only
                  // for non-toggle saves — close the modal then.
                  void formSaveRef.current?.().then(() => closeSettings())
                }}
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

      {/* Add-server wizard — a modal overlay (same size as Settings) over the
          app, not a full-screen takeover. */}
      <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={addingServer} onOpenChange={(v) => !v && setAddingServer(false)}>
        <Modal.Container size="cover" scroll="outside">
          <Modal.Dialog className="p-10 dialog-surface-alt">
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading className="text-accent">{t('setup.addServerTitle', 'Add a server')}</Modal.Heading>
            </Modal.Header>
            <Modal.Body className="flex flex-col overflow-y-auto h-[80vh] min-h-0 pr-1">
              {addingServer && (
                <SetupWizard
                  onDone={() => {
                    setAddingServer(false)
                    void refreshServers()
                    void refreshStatus()
                  }}
                />
              )}
            </Modal.Body>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>

      {/* Manage server — per-server settings as a modal (keyed by id in state). */}
      <ManageServerModal
        open={!!manageServerID}
        serverId={manageServerID}
        canControl={can('server:control')}
        onClose={() => setManageServerID('')}
        onDeleted={() => void refreshStatus()}
      />

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
                    void applyUpdate()
                  }}
                  isDisabled={updateApplying}
                >
                  {t('app.updateNow')}
                </Button>
              )}
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>

      {/* Update progress overlay — shown while downloading, restarting, and waiting for the server */}
      <UpdateProgressModal
        isOpen={updateApplying}
        phase={updatePhase}
        errorMessage={updateError}
        onDismiss={() => setUpdateApplying(false)}
      />

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
