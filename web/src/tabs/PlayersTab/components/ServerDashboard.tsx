import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Spinner, toast } from '@heroui/react'
import { AreaChart, BarChart, Segment } from '@heroui-pro/react'
import { api } from '../../../api/client'
import type { FactionStat, FactionTrends, ServerSummary } from '../../../api/client'
import { DataTable, PageHeader, Panel, SectionLabel } from '../../../dune-ui'
import type { Column } from '../../../dune-ui'
import type { FactionCol } from './types'
import { IntelAuditPanel } from './IntelAuditPanel'
import { Sep } from './Sep'
import { Stat } from './Stat'

// Faction line colors keyed by name (recharts can't read CSS tokens at render).
// Atreides green, Harkonnen red, Smuggler spice-amber; unaffiliated (None /
// Unaligned) grey. Unknown factions fall back to a distinct-hue palette.
const FACTION_COLOR_MAP: Record<string, string> = {
  Atreides: '#52c080',
  Harkonnen: '#e05252',
  Smuggler: '#c9820a',
  None: '#8a8a8a',
  Unaligned: '#8a8a8a',
}
const FACTION_FALLBACK = ['var(--accent)', '#5b8def', '#9b59b6', '#d98c5f']
const factionColor = (faction: string, i: number) =>
  FACTION_COLOR_MAP[faction] ?? FACTION_FALLBACK[i % FACTION_FALLBACK.length]

// Compact axis labels so large Solaris totals (tens of millions) don't overflow
// the Y-axis gutter — e.g. 10,000,000 → "10M". Tooltip shows the full number.
const compactNum = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 })

const fmtDate = (d: string): string => {
  return new Date(d + 'T12:00:00Z').toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

const fmtPlaytime = (secs: number): string => {
  const h = Math.floor(secs / 3600)
  const m = Math.floor((secs % 3600) / 60)
  return h > 0 ? `${h}h ${m}m` : `${m}m`
}

const econPct = (f: FactionStat, totalSolaris: number): number =>
  totalSolaris > 0 ? f.solaris / totalSolaris * 100 : 0

const renderFactionCell = (f: FactionStat, key: FactionCol, totalSolaris: number): React.ReactNode => {
  switch (key) {
    case 'faction': return f.faction
    case 'players': return <span className="tabular-nums">{f.players.toLocaleString()}</span>
    case 'avg_level': return <span className="tabular-nums">{f.avg_level.toFixed(1)}</span>
    case 'solaris': return <span className="tabular-nums">{f.solaris.toLocaleString()}</span>
    case 'scrip': return <span className="tabular-nums">{f.scrip.toLocaleString()}</span>
    case 'econ_pct': return <span className="tabular-nums">{`${econPct(f, totalSolaris).toFixed(1)}%`}</span>
  }
}

const factionSortValue = (f: FactionStat, key: FactionCol, totalSolaris: number): string | number => {
  return key === 'faction' ? f.faction : key === 'econ_pct' ? econPct(f, totalSolaris) : f[key]
}

// ServerDashboard is the Players-tab landing (#130): server-wide aggregates and
// trends across all players, shown when no individual player is selected. The
// 1:1 detail view is unchanged — picking a player replaces this.
export const ServerDashboard: React.FC = () => {
  const { t } = useTranslation()
  const [summary, setSummary] = React.useState<ServerSummary | null>(null)
  const [loading, setLoading] = React.useState(false)
  const [trends, setTrends] = React.useState<FactionTrends | null>(null)
  const [metric, setMetric] = React.useState<'solaris' | 'level'>('solaris')

  // Mirror PlayersTab.loadPlayers: defer setLoading into a microtask so it is
  // not a synchronous setState inside the effect (react-hooks/set-state-in-effect).
  const load = (): void => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.players.summary())
      .then(setSummary)
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }

  React.useEffect(() => {
    load()
  }, [])

  // Faction-growth trends; re-fetched when the metric toggles. Deferred setState
  // (same pattern as load) to satisfy react-hooks/set-state-in-effect.
  const loadTrends = (): void => {
    Promise.resolve()
      .then(() => api.players.factionTrends(metric))
      .then(setTrends)
      .catch(() => {})
  }

  React.useEffect(() => {
    loadTrends()
  }, [metric]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="flex h-full flex-col gap-4 overflow-y-auto pr-3">
      <PageHeader
        title={t('players.dashboard.title')}
        subtitle={t('players.dashboard.subtitle')}
        onRefresh={load}
        loading={loading}
      />

      {!summary
        ? (
            <div className="flex flex-1 items-center justify-center">
              {loading ? <Spinner /> : <p className="text-muted text-sm">{t('common.noResults')}</p>}
            </div>
          )
        : (
            <React.Fragment>
              <Panel>
                <div className="flex items-center flex-wrap gap-0">
                  <Stat label={t('players.dashboard.totalPlayers')}>{summary.total_players.toLocaleString()}</Stat>
                  <Sep />
                  <Stat label={t('players.dashboard.online')}>{summary.online_players.toLocaleString()}</Stat>
                  <Sep />
                  <Stat label={t('players.dashboard.avgLevel')}>{summary.avg_char_level.toFixed(1)}</Stat>
                  <Sep />
                  <Stat label={t('players.dashboard.totalPlaytime')}>{fmtPlaytime(summary.total_playtime_secs)}</Stat>
                  <Sep />
                  <Stat label={t('players.dashboard.totalSolaris')}>{summary.total_solaris.toLocaleString()}</Stat>
                  <Sep />
                  <Stat label={t('players.dashboard.totalScrip')}>{summary.total_scrip.toLocaleString()}</Stat>
                </div>
              </Panel>

              <Panel>
                <SectionLabel>{t('players.dashboard.activityTrend', { days: summary.trend_days })}</SectionLabel>
                <BarChart
                  data={summary.activity_trend}
                  height={176}
                  margin={{ top: 4, right: 8, bottom: 0, left: 0 }}
                >
                  <BarChart.Grid vertical={false} />
                  <BarChart.XAxis dataKey="day" tickFormatter={fmtDate} tickMargin={8} />
                  <BarChart.YAxis allowDecimals={false} width={32} />
                  <BarChart.Bar
                    dataKey="count"
                    fill="var(--accent)"
                    radius={[3, 3, 0, 0]}
                    maxBarSize={28}
                    name={t('players.dashboard.sessions')}
                  />
                  <BarChart.Tooltip
                    content={(
                      <BarChart.TooltipContent
                        labelFormatter={(d) => fmtDate(String(d))}
                        valueFormatter={(v) => String(v)}
                      />
                    )}
                  />
                </BarChart>
              </Panel>

              <Panel>
                <SectionLabel>{t('players.dashboard.byMap')}</SectionLabel>
                <div className="flex flex-col gap-1">
                  {summary.by_map.length === 0
                    ? <p className="text-muted text-sm">{t('players.dashboard.noPlayers')}</p>
                    : summary.by_map.map((m) => (
                        <div key={m.label} className="flex items-center justify-between text-sm">
                          <span className="text-foreground">{m.label}</span>
                          <span className="text-muted tabular-nums">{m.count}</span>
                        </div>
                      ))}
                </div>
              </Panel>

              <Panel>
                <SectionLabel>{t('players.dashboard.byFaction')}</SectionLabel>
                {summary.by_faction.length === 0
                  ? <p className="text-muted text-sm">{t('players.dashboard.noPlayers')}</p>
                  : (
                      <DataTable<FactionStat, FactionCol>
                        aria-label={t('players.dashboard.byFaction')}
                        columns={[
                          { key: 'faction', label: t('players.dashboard.factionCol'), isRowHeader: true },
                          { key: 'players', label: t('players.dashboard.playersCol'), align: 'end' },
                          { key: 'avg_level', label: t('players.dashboard.avgLevelCol'), align: 'end' },
                          { key: 'solaris', label: t('players.dashboard.solarisCol'), align: 'end' },
                          { key: 'scrip', label: t('players.dashboard.scripCol'), align: 'end' },
                          { key: 'econ_pct', label: t('players.dashboard.econPctCol'), align: 'end' },
                        ] satisfies Column<FactionCol>[]}
                        rows={summary.by_faction}
                        rowId={(f) => f.faction}
                        renderCell={(f, key) => renderFactionCell(f, key, summary.total_solaris)}
                        sortValue={(f, key) => factionSortValue(f, key, summary.total_solaris)}
                      />
                    )}
              </Panel>

              <Panel>
                <div className="flex items-center justify-between gap-2">
                  <SectionLabel>{t('players.dashboard.growthTitle')}</SectionLabel>
                  <Segment
                    size="sm"
                    selectedKey={metric}
                    onSelectionChange={(key) => setMetric(key as 'solaris' | 'level')}
                  >
                    <Segment.Item id="solaris">
                      <Segment.Separator />
                      {t('players.dashboard.metricSolaris')}
                    </Segment.Item>
                    <Segment.Item id="level">
                      <Segment.Separator />
                      {t('players.dashboard.metricLevel')}
                    </Segment.Item>
                  </Segment>
                </div>
                <p className="text-muted text-xs">{t('players.dashboard.growthApprox')}</p>
                {!trends || trends.points.length === 0
                  ? <p className="text-muted text-sm">{t('players.dashboard.noPlayers')}</p>
                  : (
                      <React.Fragment>
                        <div className="flex flex-wrap gap-x-3 gap-y-1">
                          {trends.factions.map((fac, i) => (
                            <div key={fac} className="flex items-center gap-1.5">
                              <span className="size-2 shrink-0 rounded-full" style={{ background: factionColor(fac, i) }} />
                              <span className="text-muted text-xs">{fac}</span>
                            </div>
                          ))}
                        </div>
                        <AreaChart
                          data={trends.points.map((p) => ({ day: p.day, ...p.values }))}
                          height={200}
                          margin={{ top: 4, right: 8, bottom: 0, left: 0 }}
                        >
                          <defs>
                            {trends.factions.map((fac, i) => (
                              <linearGradient key={fac} id={`faction-${i}`} x1="0" x2="0" y1="0" y2="1">
                                <stop offset="0%" stopColor={factionColor(fac, i)} stopOpacity={0.2} />
                                <stop offset="100%" stopColor={factionColor(fac, i)} stopOpacity={0.02} />
                              </linearGradient>
                            ))}
                          </defs>
                          <AreaChart.Grid vertical={false} />
                          <AreaChart.XAxis dataKey="day" tickFormatter={fmtDate} tickMargin={8} />
                          <AreaChart.YAxis width={48} tickFormatter={(v: number) => compactNum.format(v)} />
                          {trends.factions.map((fac, i) => (
                            <AreaChart.Area
                              key={fac}
                              type="monotone"
                              dataKey={fac}
                              stroke={factionColor(fac, i)}
                              strokeWidth={2}
                              dot={false}
                              fill={`url(#faction-${i})`}
                            />
                          ))}
                          <AreaChart.Tooltip
                            content={(
                              <AreaChart.TooltipContent
                                indicator="line"
                                labelFormatter={(d) => fmtDate(String(d))}
                                valueFormatter={(v) => (v as number).toLocaleString()}
                              />
                            )}
                          />
                        </AreaChart>
                      </React.Fragment>
                    )}
              </Panel>

              <IntelAuditPanel />
            </React.Fragment>
          )}
    </div>
  )
}
