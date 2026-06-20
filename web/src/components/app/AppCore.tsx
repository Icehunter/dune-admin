import * as React from 'react'
import { Toast, toast } from '@heroui/react'
import { AppLayout } from '@heroui-pro/react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAtom, useSetAtom } from 'jotai'
import { useStatus } from '../../hooks/useStatus'
import { BackendUnreachable } from '../BackendUnreachable'
import { ManageServerModal } from '../ManageServerModal'
import { useActiveServer } from '../../context/useActiveServer'
import { usePermissions } from '../../hooks/usePermissions'
import { api } from '../../api/client'
import type { TabId, AppCoreProps } from '../../types'
import {
  addServerOpenAtom,
  dashboardRefreshAtom,
  manageServerIdAtom,
  settingsOpenAtom,
  settingsTabAtom,
} from '../../atoms/app'
import {
  DEFAULT_TAB,
  TAB_CAPABILITIES,
  TAB_IDS,
  currentTabFromPath,
} from './nav'
import { useNavGroups } from './useNavGroups'
import { AppNavbar } from './AppNavbar'
import { AppSidebar } from './AppSidebar'
import { AppRoutes } from './AppRoutes'
import { SettingsModal } from './SettingsModal'
import { AddServerModal } from './AddServerModal'
import { UpdatePromptModal } from './UpdatePromptModal'
import { UpdateProgressOverlay } from './UpdateProgressOverlay'

export const AppCore: React.FC<AppCoreProps> = ({ isSignedIn }): React.ReactElement => {
  const { status, state: connState, refresh: refreshStatus } = useStatus()
  const location = useLocation()
  const navigate = useNavigate()
  const { t, i18n } = useTranslation()
  const [reconnecting, setReconnecting] = React.useState(false)
  const { can, isOwner, enabled: authEnabled } = usePermissions()
  const { servers, activeID, refresh: refreshServers, loading: serversLoading } = useActiveServer()

  const setAddServerOpen = useSetAtom(addServerOpenAtom)
  const [manageServerID, setManageServerID] = useAtom(manageServerIdAtom)
  const setSettingsOpen = useSetAtom(settingsOpenAtom)
  const setSettingsTab = useSetAtom(settingsTabAtom)
  const setDashboardRefresh = useSetAtom(dashboardRefreshAtom)

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
  //
  // Control-plane support is NOT a visibility gate: a tab the session is allowed
  // to see (e.g. Director) stays enabled regardless of the active control plane.
  // When the plane doesn't support it, the tab itself renders a friendly
  // "not supported" notice rather than vanishing from the nav.
  const canSeeTab = React.useCallback((key: TabId) => {
    if (key === 'dashboard') return true
    // Diagnostics is about dune-admin itself, not a specific game server, so it
    // stays visible even when no server is configured.
    if (servers.length === 0 && key !== 'diagnostics') return false
    const cap = TAB_CAPABILITIES[key]
    if (cap === 'owner') return authEnabled && (isOwner || can('auth:manage'))
    return can(cap)
  }, [authEnabled, isOwner, can, servers.length])

  // Re-establish backend connections (DB + control plane) without a service
  // restart — used by the navbar Reconnect button when the DB shows disconnected
  // (e.g. dune-admin came up before the database was ready).
  const handleReconnect = async (): Promise<void> => {
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

  const NAV_GROUPS = useNavGroups()

  const openSettings = (tab?: string): void => {
    setSettingsTab(tab)
    setSettingsOpen(true)
  }
  const closeSettings = (): void => {
    setSettingsOpen(false)
    setDashboardRefresh((n) => n + 1)
  }

  // Nav groups filtered to what the session may see; empty groups disappear.
  const visibleNavGroups = NAV_GROUPS
    .map((g) => ({ ...g, items: g.items.filter((i) => canSeeTab(i.key)) }))
    .filter((g) => g.items.length > 0)
  const firstVisibleTab = visibleNavGroups[0]?.items[0]?.key ?? DEFAULT_TAB

  React.useEffect(() => {
    const seg = location.pathname.replace(/^\//, '').split('/')[0]
    // Unknown or empty path → resolve to a valid tab immediately (no data needed).
    if (!seg || !(TAB_IDS as readonly string[]).includes(seg)) {
      navigate(`/${firstVisibleTab}`, { replace: true })
      return
    }
    // Known path: don't bounce to the Dashboard before we know whether the path
    // is actually allowed. On a hard refresh the server list and status load
    // asynchronously; redirecting too early discards the user's deep link.
    // Best-effort — keep the requested path until we can prove it's disallowed.
    if (serversLoading || status === null) return
    if (!canSeeTab(seg as TabId)) {
      navigate(`/${firstVisibleTab}`, { replace: true })
    }
  }, [location.pathname, navigate, canSeeTab, firstVisibleTab, serversLoading, status])

  const currentTab = currentTabFromPath(location.pathname)
  const pathname = location.pathname

  // #165: when the SPA never reached the backend, show an informative setup
  // screen instead of an empty, non-working dashboard.
  if (connState === 'error') {
    return <BackendUnreachable onRetry={() => window.location.reload()} />
  }

  // No forced first-run wizard. A fresh install (no servers) lands on the
  // Dashboard, which surfaces optional onboarding (add server / set up auth /
  // Discord). The "Add server" wizard is launched on demand as a modal overlay
  // (rendered below, alongside the Settings modal).

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
        navbar={(
          <AppNavbar
            status={status}
            reconnecting={reconnecting}
            onReconnect={handleReconnect}
            can={can}
            onOpenSettings={openSettings}
          />
        )}
        sidebar={(
          <AppSidebar
            visibleNavGroups={visibleNavGroups}
            pathname={pathname}
            navigate={navigate}
          />
        )}
      >
        {/* Keyed by activeID so switching servers remounts every tab — each
            re-fetches its data with the new X-Dune-Server header (no reload). */}
        <div key={activeID} className="h-full flex flex-col overflow-hidden min-h-0">
          <AppRoutes
            currentTab={currentTab}
            status={status}
            isSignedIn={isSignedIn}
            canSeeTab={canSeeTab}
            onOpenSettings={openSettings}
          />
        </div>
      </AppLayout>

      {/* Settings modal — structure mirrors BotControlPanel */}
      <SettingsModal status={status} can={can} onClose={closeSettings} />

      {/* Add-server wizard — a modal overlay (same size as Settings) over the
          app, not a full-screen takeover. */}
      <AddServerModal
        onDone={() => {
          setAddServerOpen(false)
          void refreshServers()
          void refreshStatus()
        }}
      />

      {/* Manage server — per-server settings as a modal (keyed by id in state). */}
      <ManageServerModal
        open={manageServerID !== 0}
        serverId={manageServerID}
        canControl={can('server:control')}
        onClose={() => setManageServerID(0)}
        onDeleted={() => void refreshStatus()}
      />

      {/* Update-available prompt — opened from the navbar release widget (#129).
          Reuses the backend update check for the release-notes link + Continue/Cancel. */}
      <UpdatePromptModal can={can} />

      {/* Update progress overlay — shown while downloading, restarting, and waiting for the server */}
      <UpdateProgressOverlay />

    </div>
  )
}
