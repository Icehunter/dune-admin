import type React from 'react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BarChart, Bar, LineChart, Line, Legend, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'
import { Button, Spinner, toast } from '@heroui/react'
import { api } from '../../../api/client'
import type { FactionTrends, ServerSummary } from '../../../api/client'
import { InfoCard, PageHeader, Panel, SectionLabel } from '../../../dune-ui'

// Faction line colors keyed by name (recharts can't read CSS tokens at render).
// Atreides green, Harkonnen red, Smuggler spice-amber; unaffiliated (None /
// Unaligned) grey. Unknown factions fall back to a distinct-hue palette.
const FACTION_COLOR_MAP: Record<string, string> = {
  Atreides: '#52c080',
  Harkonnen: '#e05252',
  Smuggler: '#e6a338',
  None: '#8a8a8a',
  Unaligned: '#8a8a8a',
}
const FACTION_FALLBACK = ['var(--accent)', '#5b8def', '#9b59b6', '#d98c5f']
const factionColor = (faction: string, i: number) =>
  FACTION_COLOR_MAP[faction] ?? FACTION_FALLBACK[i % FACTION_FALLBACK.length]

// Compact axis labels so large Solaris totals (tens of millions) don't overflow
// the Y-axis gutter — e.g. 10,000,000 → "10M". Tooltip shows the full number.
const compactNum = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 })

function fmtDate(d: string): string {
  return new Date(d + 'T12:00:00Z').toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

function fmtPlaytime(secs: number): string {
  const h = Math.floor(secs / 3600)
  const m = Math.floor((secs % 3600) / 60)
  return h > 0 ? `${h}h ${m}m` : `${m}m`
}

// ServerDashboard is the Players-tab landing (#130): server-wide aggregates and
// trends across all players, shown when no individual player is selected. The
// 1:1 detail view is unchanged — picking a player replaces this.
export const ServerDashboard: React.FC = () => {
  const { t } = useTranslation()
  const [summary, setSummary] = useState<ServerSummary | null>(null)
  const [loading, setLoading] = useState(false)
  const [trends, setTrends] = useState<FactionTrends | null>(null)
  const [metric, setMetric] = useState<'solaris' | 'level'>('solaris')

  // Mirror PlayersTab.loadPlayers: defer setLoading into a microtask so it is
  // not a synchronous setState inside the effect (react-hooks/set-state-in-effect).
  const load = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.players.summary())
      .then(setSummary)
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  // Faction-growth trends; re-fetched when the metric toggles. Deferred setState
  // (same pattern as load) to satisfy react-hooks/set-state-in-effect.
  const loadTrends = useCallback(() => {
    Promise.resolve()
      .then(() => api.players.factionTrends(metric))
      .then(setTrends)
      .catch(() => {})
  }, [metric])

  useEffect(() => {
    loadTrends()
  }, [loadTrends])

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
            <>
              <InfoCard className="flex-wrap">
                <InfoCard.Item label={t('players.dashboard.totalPlayers')} value={summary.total_players} />
                <InfoCard.Item label={t('players.dashboard.online')} value={summary.online_players} />
                <InfoCard.Item label={t('players.dashboard.avgLevel')} value={summary.avg_char_level.toFixed(1)} />
                <InfoCard.Item label={t('players.dashboard.totalPlaytime')} value={fmtPlaytime(summary.total_playtime_secs)} />
                <InfoCard.Item label={t('players.dashboard.totalSolaris')} value={summary.total_solaris.toLocaleString()} />
                <InfoCard.Item label={t('players.dashboard.totalScrip')} value={summary.total_scrip.toLocaleString()} />
              </InfoCard>

              <Panel>
                <SectionLabel>{t('players.dashboard.activityTrend', { days: summary.trend_days })}</SectionLabel>
                <div className="mt-3 h-48">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={summary.activity_trend} margin={{ top: 4, right: 8, left: 8, bottom: 0 }}>
                      <XAxis
                        dataKey="day"
                        tickFormatter={fmtDate}
                        tick={{ fontSize: 11, fill: 'var(--muted)' }}
                        tickLine={false}
                        axisLine={false}
                      />
                      <YAxis
                        allowDecimals={false}
                        tick={{ fontSize: 11, fill: 'var(--muted)' }}
                        tickLine={false}
                        axisLine={false}
                        width={32}
                      />
                      <Tooltip
                        cursor={{ fill: 'var(--surface-hover)' }}
                        formatter={(val) => [String(val as number), t('players.dashboard.sessions')]}
                        labelFormatter={(d) => fmtDate(String(d))}
                        contentStyle={{
                          background: 'var(--surface)',
                          border: '1px solid var(--border)',
                          borderRadius: 'var(--radius)',
                          fontSize: 12,
                        }}
                        labelStyle={{ color: 'var(--foreground)' }}
                      />
                      <Bar dataKey="count" fill="var(--accent)" radius={[3, 3, 0, 0]} maxBarSize={28} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </Panel>

              <Panel>
                <SectionLabel>{t('players.dashboard.byMap')}</SectionLabel>
                <div className="mt-3 flex flex-col gap-1">
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
                  ? <p className="text-muted text-sm mt-3">{t('players.dashboard.noPlayers')}</p>
                  : (
                      <div className="mt-3 overflow-x-auto">
                        <table className="w-full text-sm">
                          <thead>
                            <tr className="text-left text-muted">
                              <th className="pb-1 font-normal">{t('players.dashboard.factionCol')}</th>
                              <th className="pb-1 text-right font-normal">{t('players.dashboard.playersCol')}</th>
                              <th className="pb-1 text-right font-normal">{t('players.dashboard.avgLevelCol')}</th>
                              <th className="pb-1 text-right font-normal">{t('players.dashboard.solarisCol')}</th>
                              <th className="pb-1 text-right font-normal">{t('players.dashboard.scripCol')}</th>
                              <th className="pb-1 text-right font-normal">{t('players.dashboard.econPctCol')}</th>
                            </tr>
                          </thead>
                          <tbody>
                            {summary.by_faction.map((f) => (
                              <tr key={f.faction} className="border-t border-border/40">
                                <td className="py-1 text-foreground">{f.faction}</td>
                                <td className="py-1 text-right tabular-nums">{f.players.toLocaleString()}</td>
                                <td className="py-1 text-right tabular-nums">{f.avg_level.toFixed(1)}</td>
                                <td className="py-1 text-right tabular-nums">{f.solaris.toLocaleString()}</td>
                                <td className="py-1 text-right tabular-nums">{f.scrip.toLocaleString()}</td>
                                <td className="py-1 text-right tabular-nums">
                                  {summary.total_solaris > 0 ? (f.solaris / summary.total_solaris * 100).toFixed(1) : '0.0'}
                                  %
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
              </Panel>

              <Panel>
                <div className="flex items-center justify-between gap-2">
                  <SectionLabel>{t('players.dashboard.growthTitle')}</SectionLabel>
                  <div className="flex gap-1">
                    <Button size="sm" variant={metric === 'solaris' ? 'secondary' : 'ghost'} onPress={() => setMetric('solaris')}>
                      {t('players.dashboard.metricSolaris')}
                    </Button>
                    <Button size="sm" variant={metric === 'level' ? 'secondary' : 'ghost'} onPress={() => setMetric('level')}>
                      {t('players.dashboard.metricLevel')}
                    </Button>
                  </div>
                </div>
                <p className="text-muted mt-1 text-xs">{t('players.dashboard.growthApprox')}</p>
                {!trends || trends.points.length === 0
                  ? <p className="text-muted text-sm mt-3">{t('players.dashboard.noPlayers')}</p>
                  : (
                      <div className="mt-3 h-56">
                        <ResponsiveContainer width="100%" height="100%">
                          <LineChart
                            data={trends.points.map((p) => ({ day: p.day, ...p.values }))}
                            margin={{ top: 4, right: 8, left: 8, bottom: 0 }}
                          >
                            <XAxis
                              dataKey="day"
                              tickFormatter={fmtDate}
                              tick={{ fontSize: 11, fill: 'var(--muted)' }}
                              tickLine={false}
                              axisLine={false}
                            />
                            <YAxis
                              tick={{ fontSize: 11, fill: 'var(--muted)' }}
                              tickLine={false}
                              axisLine={false}
                              width={48}
                              tickFormatter={(v) => compactNum.format(v as number)}
                            />
                            <Tooltip
                              labelFormatter={(d) => fmtDate(String(d))}
                              formatter={(val) => (val as number).toLocaleString()}
                              cursor={{ stroke: 'var(--border)' }}
                              contentStyle={{
                                background: 'var(--surface)',
                                border: '1px solid var(--border)',
                                borderRadius: 'var(--radius)',
                                fontSize: 12,
                              }}
                              labelStyle={{ color: 'var(--foreground)' }}
                            />
                            <Legend wrapperStyle={{ fontSize: 11 }} />
                            {trends.factions.map((fac, i) => (
                              <Line
                                key={fac}
                                type="monotone"
                                dataKey={fac}
                                stroke={factionColor(fac, i)}
                                strokeWidth={2}
                                dot={false}
                              />
                            ))}
                          </LineChart>
                        </ResponsiveContainer>
                      </div>
                    )}
              </Panel>
            </>
          )}
    </div>
  )
}
