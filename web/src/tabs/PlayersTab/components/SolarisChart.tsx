import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { AreaChart } from '@heroui-pro/react'
import { SectionLabel } from '../../../dune-ui'
import type { SolarisChartProps, SolarisPoint } from './interfaces'

const fmtSolaris = (n: number): string => {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

const fmtTime = (iso: string): string => {
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export const SolarisChart: React.FC<SolarisChartProps> = ({ data }) => {
  const { t } = useTranslation()
  const [hidden, setHidden] = React.useState<Set<string>>(new Set())

  const LINES = [
    { key: 'balance', label: t('players.detail.solarisBalance'), color: 'var(--accent)' },
    { key: 'cum_earned', label: t('players.detail.earned'), color: '#52c080' },
    { key: 'cum_spent', label: t('players.detail.spent'), color: '#e05252' },
  ]

  const toggle = (key: string) => {
    setHidden((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  const _snaps = data.filter((s) => s.solaris_balance != null)
  let _cumEarned = 0
  let _cumSpent = 0
  const points: SolarisPoint[] = _snaps.length === 0
    ? []
    : _snaps.map((s, i) => {
        const balance = s.solaris_balance as number
        if (i > 0) {
          const prev = _snaps[i - 1].solaris_balance as number
          const delta = balance - prev
          if (delta > 0) _cumEarned += delta
          else if (delta < 0) _cumSpent += -delta
        }
        return { time: s.snapped_at, balance, cum_earned: _cumEarned, cum_spent: _cumSpent }
      })

  if (points.length === 0) {
    return (
      <div>
        <SectionLabel>{t('players.detail.solarisHistory')}</SectionLabel>
        <p className="text-muted text-sm mt-2">
          {t('players.detail.solarisHistoryEmpty')}
        </p>
      </div>
    )
  }

  const visibleLines = LINES.filter((l) => !hidden.has(l.key))

  return (
    <div>
      <SectionLabel>{t('players.detail.solarisHistory')}</SectionLabel>
      <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1">
        {LINES.map((l) => (
          <button
            key={l.key}
            type="button"
            className="flex cursor-pointer select-none items-center gap-1.5 text-xs"
            style={{ opacity: hidden.has(l.key) ? 0.4 : 1 }}
            onClick={() => toggle(l.key)}
          >
            <span className="size-2 shrink-0 rounded-full" style={{ background: l.color }} />
            <span className="text-muted">{l.label}</span>
          </button>
        ))}
      </div>
      <AreaChart data={points} height={200} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
        <defs>
          {LINES.map((l) => (
            <linearGradient key={l.key} id={`solaris-${l.key}`} x1="0" x2="0" y1="0" y2="1">
              <stop offset="0%" stopColor={l.color} stopOpacity={0.2} />
              <stop offset="100%" stopColor={l.color} stopOpacity={0.02} />
            </linearGradient>
          ))}
        </defs>
        <AreaChart.Grid vertical={false} />
        <AreaChart.XAxis dataKey="time" tickFormatter={fmtTime} tickMargin={8} />
        <AreaChart.YAxis tickFormatter={fmtSolaris} width={52} />
        {visibleLines.map((l) => (
          <AreaChart.Area
            key={l.key}
            type="monotone"
            dataKey={l.key}
            name={l.label}
            stroke={l.color}
            strokeWidth={2}
            dot={false}
            connectNulls
            fill={`url(#solaris-${l.key})`}
          />
        ))}
        <AreaChart.Tooltip
          content={(
            <AreaChart.TooltipContent
              indicator="line"
              labelFormatter={(d) => fmtTime(String(d))}
              valueFormatter={(v) => fmtSolaris(Number(v))}
            />
          )}
        />
      </AreaChart>
    </div>
  )
}
