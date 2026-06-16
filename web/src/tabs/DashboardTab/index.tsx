import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { Button, Chip, Skeleton, Spinner, toast } from '@heroui/react'
import { Icon, Panel, PageHeader, LoadingState } from '../../dune-ui'
import { api } from '../../api/client'
import type { ServerHealth } from '../../api/client'
import { useActiveServer } from '../../context/useActiveServer'
import { usePermissions } from '../../hooks/usePermissions'
import { formatUptime } from '../BattlegroupTab/uptime'
import { phaseChipColor } from '../BattlegroupTab/helpers'
import { OnboardingCards } from './OnboardingCards'

export interface DashboardTabProps {
  onAddServer: () => void
  onOpenSettings: (tab?: string) => void
  onManageServer: (id: number) => void
  /** Bumped when the settings modal closes, so onboarding state re-syncs. */
  refreshKey?: number
}

export const DashboardTab: React.FC<DashboardTabProps> = ({
  onAddServer, onOpenSettings, onManageServer, refreshKey,
}) => {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { servers, setActive, loading: serversLoading } = useActiveServer()
  const { can, enabled: authEnabled } = usePermissions()
  const canControl = can('server:control')
  const canManageAuth = can('auth:manage')
  const [health, setHealth] = React.useState<ServerHealth[]>([])
  const [loading, setLoading] = React.useState(false)
  const [discordEnabled, setDiscordEnabled] = React.useState(false)

  const load = React.useCallback(() => {
    if (servers.length === 0) {
      void Promise.resolve().then(() => setHealth([]))
      return
    }
    void Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.servers.health())
      .then(setHealth)
      .catch((e: unknown) => toast.danger(t('dashboard.loadFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t, servers.length])

  React.useEffect(() => {
    load()
  }, [load])

  // Read whether the Discord bot is configured so its onboarding card hides once
  // set up. Best-effort — guests (no config:read) just won't see the card anyway.
  React.useEffect(() => {
    if (!canControl) return
    void api.config.get()
      .then((c) => setDiscordEnabled(!!c.discord_bot_enabled))
      .catch(() => { /* ignore */ })
  }, [canControl, refreshKey])

  const view = (id: number) => {
    void setActive(id).then(() => navigate('/battlegroup'))
  }

  const hasServers = servers.length > 0
  // First-load gate for the per-server health metrics: skeletons only while the
  // initial health fetch is in flight (no data yet). Background refreshes keep
  // the real values on screen.
  const healthFirstLoad = loading && health.length === 0

  return (
    <div className="flex flex-col h-full gap-4 min-h-0 overflow-y-auto">
      <PageHeader title={t('dashboard.title', 'Dashboard')} subtitle={t('dashboard.subtitle', 'Servers and setup at a glance')}>
        {hasServers && (
          <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
            {loading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
            {' '}
            {t('common.refresh', 'Refresh')}
          </Button>
        )}
      </PageHeader>

      {/* Onboarding / help cards (admin-only, dismissible) + empty state. */}
      <OnboardingCards
        hasServers={hasServers}
        serversLoading={serversLoading}
        canControl={canControl}
        canManageAuth={canManageAuth}
        authEnabled={authEnabled}
        discordEnabled={discordEnabled}
        onAddServer={onAddServer}
        onOpenSettings={onOpenSettings}
      />

      {/* Per-server health cards (gated behind the first-load spinner so a fresh
          load never flashes the empty state). */}
      {serversLoading
        ? <LoadingState />
        : hasServers && (
          <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-3">
            {servers.map((s) => {
              const h = health.find((x) => x.id === s.id)
              return (
                <Panel key={s.id} className="flex flex-col gap-3">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-2 min-w-0">
                      <span className="text-sm font-semibold text-foreground truncate">{s.name}</span>
                      {h?.control && (
                        <span className="text-[10px] font-semibold uppercase tracking-wide px-1.5 py-0.5 rounded bg-accent/15 text-accent border border-accent/30">
                          {h.control}
                        </span>
                      )}
                    </div>
                    {healthFirstLoad
                      ? <Skeleton className="h-5 w-20 rounded-lg" />
                      : (
                          <Chip size="sm" variant="soft" color={phaseChipColor(h?.running ? 'running' : (h?.phase || 'stopped'))}>
                            {h ? (h.running ? t('dashboard.running', 'Running') : (h.phase || t('dashboard.offline', 'Offline'))) : '…'}
                          </Chip>
                        )}
                  </div>

                  <div className="flex items-center gap-4 text-xs text-muted">
                    {healthFirstLoad
                      ? (
                          <>
                            {/* h-4 matches the text-xs metric line-box (16px). */}
                            <Skeleton className="h-4 w-16 rounded-lg" />
                            <Skeleton className="h-4 w-20 rounded-lg" />
                          </>
                        )
                      : (
                          <>
                            <span className="flex items-center gap-1">
                              <Icon name="clock" className="size-3" />
                              {formatUptime(h?.uptime_seconds)}
                            </span>
                            <span className="flex items-center gap-1">
                              <Icon name="users" className="size-3" />
                              {t('dashboard.playersOnline', '{{count}} online', { count: h?.players_online ?? 0 })}
                            </span>
                          </>
                        )}
                  </div>

                  {h?.error && <p className="text-xs text-warning">{h.error}</p>}

                  <div className="flex items-center gap-2 mt-auto">
                    <Button size="sm" onPress={() => view(s.id)}>
                      <Icon name="eye" />
                      {' '}
                      {t('dashboard.view', 'View')}
                    </Button>
                    {canControl && (
                      <Button size="sm" variant="outline" onPress={() => onManageServer(s.id)}>
                        <Icon name="settings" />
                        {' '}
                        {t('dashboard.manage', 'Manage')}
                      </Button>
                    )}
                  </div>
                </Panel>
              )
            })}
          </div>
        )}
    </div>
  )
}
