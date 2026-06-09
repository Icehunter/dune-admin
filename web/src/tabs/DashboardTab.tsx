import type React from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { DetailedStatus, ServerRow } from './BattlegroupTab/types'
import { DashboardCard } from '@/components/ui/dashboard-card'
import { StatTile } from '@/components/ui/stat-tile'
import { Badge } from '@/components/ui/badge'
import { Donut, type DonutSegment } from '@/components/viz/Donut'
import { RadialGauge } from '@/components/viz/RadialGauge'
import { Sparkline } from '@/components/viz/Sparkline'
import { Icon } from '../dune-ui'

interface DashboardTabProps {
  isActive?: boolean
}

// Faction → ring/legend colour. Stable dune.factions enum; extras fall back to
// the rarity palette so every slice is still distinct.
const FACTION_COLOR: Record<string, string> = {
  Atreides: 'var(--accent)',
  Harkonnen: 'var(--danger)',
  Smuggler: 'var(--warning)',
}
const FALLBACK_COLORS = ['#4a90e2', '#a855f7', '#f97316', '#b0c90a']

const fmtNum = (n: number) => n.toLocaleString()
const fmtHours = (secs: number) => {
  const h = Math.round(secs / 3600)
  return h >= 1000 ? `${(h / 1000).toFixed(1)}k` : String(h)
}
const factionColor = (faction: string, i: number) =>
  FACTION_COLOR[faction] ?? FALLBACK_COLORS[i % FALLBACK_COLORS.length]

export const DashboardTab: React.FC<DashboardTabProps> = ({ isActive = true }) => {
  // Poll while the dashboard is the active tab; pause when hidden.
  const poll = (ms: number): number | false => (isActive ? ms : false)

  const statusQ = useQuery({ queryKey: ['status'], queryFn: api.status, refetchInterval: poll(5000) })
  const summaryQ = useQuery({ queryKey: ['players', 'summary'], queryFn: api.players.summary, refetchInterval: poll(15000) })
  const fleetQ = useQuery({
    queryKey: ['battlegroup', 'status'],
    queryFn: () => api.battlegroup.status() as Promise<DetailedStatus>,
    refetchInterval: poll(15000),
  })
  const marketQ = useQuery({ queryKey: ['market', 'stats'], queryFn: api.market.stats, refetchInterval: poll(30000) })
  const guildsQ = useQuery({ queryKey: ['guilds', 'list'], queryFn: api.guilds.list })
  const updateQ = useQuery({ queryKey: ['update', 'check'], queryFn: api.update.check })

  const status = statusQ.data
  const summary = summaryQ.data
  const servers = fleetQ.data?.servers ?? []
  const bg = fleetQ.data?.battlegroup

  const onlinePlayers = summary?.online_players ?? 0
  const totalPlayers = summary?.total_players ?? 0
  const serversReady = servers.filter((s) => s.ready).length
  const fleetPlayers = servers.reduce((a, s) => a + s.players, 0)
  const fleetCap = servers.reduce((a, s) => a + s.playerHardCap, 0)
  const fleetQueue = servers.reduce((a, s) => a + s.queue, 0)

  const healthy = status?.db_connected ?? false

  const factions = summary?.by_faction ?? []
  const factionTotal = factions.reduce((a, f) => a + f.players, 0) || 1
  const segments: DonutSegment[] = factions.map((f, i) => ({
    pct: (f.players / factionTotal) * 100,
    color: factionColor(f.faction, i),
    label: `${f.faction} ${f.players}`,
  }))

  const trend = summary?.activity_trend ?? []
  const trendCounts = trend.map((p) => p.count)

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto flex w-full max-w-[1320px] flex-col gap-5 px-4 py-6 sm:px-6">
        <header className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <div className="text-[11px] font-bold uppercase tracking-[0.14em] text-accent-brand">Overview</div>
            <h2 className="text-2xl font-extrabold tracking-tight text-foreground">Server Dashboard</h2>
            <p className="mt-0.5 text-sm text-muted">
              {bg ? `${bg.title} · ` : ''}
              {status?.control ? `${status.control} control plane` : 'control plane'}
              {status?.version ? ` · v${status.version}` : ''}
            </p>
          </div>
          {updateQ.data?.needs_update && (
            <Badge tone="warning">
              <Icon name="download" className="size-3.5" />
              {` Update available: v${updateQ.data.latest}`}
            </Badge>
          )}
        </header>

        <div className="grid grid-cols-1 gap-5 lg:grid-cols-12">
          <DashboardCard
            className="lg:col-span-8"
            title="System status"
            icon="activity"
            subtitle={status ? `DB ${status.db_host || '—'} · SSH ${status.ssh_host || 'local'}` : 'Connecting…'}
          >
            <div className="flex flex-wrap items-center gap-x-8 gap-y-4">
              <div className="flex items-center gap-3">
                <span className={`grid size-12 place-items-center rounded-full ${healthy ? 'bg-success/15 text-success' : 'bg-danger/15 text-danger'}`}>
                  <Icon name={healthy ? 'circle-check' : 'triangle-alert'} className="size-6" />
                </span>
                <div>
                  <div className="text-lg font-extrabold tracking-tight text-foreground">{healthy ? 'Operational' : 'Degraded'}</div>
                  <div className="text-xs text-muted">{healthy ? 'Database connected' : 'Database unreachable'}</div>
                </div>
              </div>
              <div className="flex flex-wrap gap-x-8 gap-y-3">
                <Vital icon="users" value={fmtNum(onlinePlayers)} label="players online" />
                <Vital icon="server" value={`${serversReady}/${servers.length}`} label="servers ready" />
                <Vital icon="hourglass" value={fmtNum(fleetQueue)} label="in queue" />
                <Vital icon="clock" value={fmtHours(summary?.total_playtime_secs ?? 0)} label="total hours" />
              </div>
            </div>
          </DashboardCard>

          <DashboardCard className="items-center justify-center lg:col-span-4" title="Fleet capacity" icon="gauge">
            <RadialGauge
              value={fleetPlayers}
              max={fleetCap || 100}
              unit=""
              title="Fleet capacity"
              label="players"
              sub={fleetCap ? `of ${fmtNum(fleetCap)} cap` : 'no servers'}
            />
          </DashboardCard>
        </div>

        <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
          {([
            { label: 'Online', value: fmtNum(onlinePlayers), icon: 'user-check' },
            { label: 'Total players', value: fmtNum(totalPlayers), icon: 'users' },
            { label: 'Guilds', value: fmtNum(guildsQ.data?.length ?? 0), icon: 'shield' },
            { label: 'Market listings', value: fmtNum(marketQ.data?.total_listings ?? 0), icon: 'store' },
            { label: 'Avg level', value: summary ? Math.round(summary.avg_char_level) : 0, icon: 'medal' },
            { label: 'Solaris', value: fmtNum(summary?.total_solaris ?? 0), icon: 'coins' },
          ] as const).map((s) => (
            <DashboardCard key={s.label} className="p-4">
              <StatTile label={s.label} value={s.value} icon={s.icon} />
            </DashboardCard>
          ))}
        </div>

        <div className="grid grid-cols-1 gap-5 lg:grid-cols-12">
          <DashboardCard className="lg:col-span-4" title="Factions" icon="swords">
            {segments.length === 0
              ? <Empty>No faction data</Empty>
              : (
                  <div className="flex items-center gap-4">
                    <Donut segments={segments} title="Players by faction" centerTop="players" centerMain={fmtNum(factionTotal)} size={140} />
                    <ul className="flex flex-1 flex-col gap-1.5 text-sm">
                      {factions.map((f, i) => (
                        <li key={f.faction} className="flex items-center gap-2">
                          <span className="size-2.5 shrink-0 rounded-full" style={{ background: factionColor(f.faction, i) }} />
                          <span className="flex-1 truncate text-foreground">{f.faction}</span>
                          <span className="tabular-nums text-muted">{fmtNum(f.players)}</span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
          </DashboardCard>

          <DashboardCard
            className="lg:col-span-8"
            title="Activity"
            icon="trending-up"
            subtitle={summary ? `New characters · last ${summary.trend_days} days` : undefined}
          >
            {trendCounts.length < 2
              ? <Empty>Not enough history yet</Empty>
              : (
                  <div className="flex flex-col gap-2">
                    <Sparkline data={trendCounts} title="Activity trend" height={92} />
                    <div className="flex justify-between text-[11px] text-muted">
                      <span>{trend[0]?.day}</span>
                      <span>{trend[trend.length - 1]?.day}</span>
                    </div>
                  </div>
                )}
          </DashboardCard>
        </div>

        <DashboardCard
          title="Game-server fleet"
          icon="server"
          subtitle={`${servers.length} server${servers.length === 1 ? '' : 's'} · ${fmtNum(fleetPlayers)} players`}
        >
          {servers.length === 0
            ? <Empty>No game servers discovered</Empty>
            : (
                <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-3">
                  {servers.map((s) => <FleetRow key={`${s.map}-${s.dimension}-${s.partition}`} s={s} />)}
                </div>
              )}
        </DashboardCard>
      </div>
    </div>
  )
}

const Vital: React.FC<{ icon: string, value: React.ReactNode, label: string }> = ({ icon, value, label }) => (
  <div className="flex items-center gap-2">
    <Icon name={icon} className="size-4 text-muted" />
    <div>
      <div className="tabular-nums text-base font-extrabold leading-none text-foreground">{value}</div>
      <div className="text-[11px] text-muted">{label}</div>
    </div>
  </div>
)

const Empty: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <div className="py-6 text-center text-xs text-muted">{children}</div>
)

const FleetRow: React.FC<{ s: ServerRow }> = ({ s }) => {
  const frac = s.playerHardCap > 0 ? Math.min(1, s.players / s.playerHardCap) : 0
  return (
    <div className="rounded-[10px] border border-border/60 bg-secondary/40 p-3">
      <div className="flex items-center justify-between gap-2">
        <span className="truncate text-sm font-semibold text-foreground">{s.map}</span>
        <Badge tone={s.ready ? 'success' : 'warning'}>{s.ready ? 'ready' : s.phase || 'loading'}</Badge>
      </div>
      <div className="mt-1 text-[11px] text-muted">
        {`dim ${s.dimension} · part ${s.partition}${s.queue > 0 ? ` · ${s.queue} queued` : ''}`}
      </div>
      <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-background">
        <div className="h-full rounded-full bg-accent-brand" style={{ width: `${frac * 100}%` }} />
      </div>
      <div className="mt-1 flex justify-between text-[11px] tabular-nums text-muted">
        <span>{`${s.players} players`}</span>
        <span>{`${s.playerHardCap} cap`}</span>
      </div>
    </div>
  )
}
