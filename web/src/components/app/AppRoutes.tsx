import * as React from 'react'
import { Spinner } from '@heroui/react'
import { useAtomValue, useSetAtom } from 'jotai'
import type { TabId } from '../../types'
import type { Status } from '../../api/client'
import {
  addServerOpenAtom,
  dashboardRefreshAtom,
  manageServerIdAtom,
} from '../../atoms/app'

const DashboardTab = React.lazy(() => import('../../tabs/DashboardTab').then((m) => ({ default: m.DashboardTab })))
const BattlegroupTab = React.lazy(() => import('../../tabs/BattlegroupTab').then((m) => ({ default: m.BattlegroupTab })))
const LiveMapTab = React.lazy(() => import('../../tabs/LiveMapTab').then((m) => ({ default: m.LiveMapTab })))
const PlayersTab = React.lazy(() => import('../../tabs/PlayersTab').then((m) => ({ default: m.PlayersTab })))
const DatabaseTab = React.lazy(() => import('../../tabs/DatabaseTab').then((m) => ({ default: m.DatabaseTab })))
const LogsTab = React.lazy(() => import('../../tabs/LogsTab').then((m) => ({ default: m.LogsTab })))
const BlueprintsTab = React.lazy(() => import('../../tabs/BlueprintsTab').then((m) => ({ default: m.BlueprintsTab })))
const BasesTab = React.lazy(() => import('../../tabs/BasesTab').then((m) => ({ default: m.BasesTab })))
const GuildsTab = React.lazy(() => import('../../tabs/GuildsTab').then((m) => ({ default: m.GuildsTab })))
const LandsraadTab = React.lazy(() => import('../../tabs/LandsraadTab').then((m) => ({ default: m.LandsraadTab })))
const StorageTab = React.lazy(() => import('../../tabs/StorageTab').then((m) => ({ default: m.StorageTab })))
const ServerSettingsTab = React.lazy(() => import('../../tabs/ServerSettingsTab').then((m) => ({ default: m.ServerSettingsTab })))
const DirectorTab = React.lazy(() => import('../../tabs/DirectorTab').then((m) => ({ default: m.DirectorTab })))
const MarketTab = React.lazy(() => import('../../tabs/MarketTab').then((m) => ({ default: m.MarketTab })))
const WelcomePackageTab = React.lazy(() => import('../../tabs/WelcomePackageTab').then((m) => ({ default: m.WelcomePackageTab })))
const EventsTab = React.lazy(() => import('../../tabs/EventsTab').then((m) => ({ default: m.EventsTab })))
const BattlepassTab = React.lazy(() => import('../../tabs/BattlepassTab').then((m) => ({ default: m.BattlepassTab })))
const PermissionsTab = React.lazy(() => import('../../tabs/PermissionsTab').then((m) => ({ default: m.PermissionsTab })))
const DiagnosticsTab = React.lazy(() => import('../../tabs/DiagnosticsTab').then((m) => ({ default: m.DiagnosticsTab })))

interface AppRoutesProps {
  currentTab: TabId
  status: Status | null
  isSignedIn: boolean
  canSeeTab: (key: TabId) => boolean
  onOpenSettings: (tab?: string) => void
}

export const AppRoutes: React.FC<AppRoutesProps> = ({ currentTab, status, isSignedIn, canSeeTab, onOpenSettings }) => {
  const dashboardRefreshKey = useAtomValue(dashboardRefreshAtom)
  const setAddServerOpen = useSetAtom(addServerOpenAtom)
  const setManageServerId = useSetAtom(manageServerIdAtom)

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

  return (
    <main className="flex-1 flex flex-col overflow-hidden min-h-0">
      {renderTab('dashboard', (
        <DashboardTab
          onAddServer={() => setAddServerOpen(true)}
          onOpenSettings={onOpenSettings}
          onManageServer={(id) => setManageServerId(id)}
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
      {canSeeTab('diagnostics') && renderTab('diagnostics', <DiagnosticsTab />)}
    </main>
  )
}
