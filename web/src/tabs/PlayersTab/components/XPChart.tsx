import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { AreaChart } from '@heroui-pro/react'
import { SectionLabel } from '../../../dune-ui'
import type { XPChartProps } from './interfaces'

const fmtXP = (n: number): string => {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return String(n)
}

const fmtTime = (iso: string): string => {
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export const XPChart: React.FC<XPChartProps> = ({ data }) => {
  const { t } = useTranslation()
  const [hidden, setHidden] = React.useState<Set<string>>(new Set())

  const LINES = [
    { key: 'char_xp', label: t('players.detail.xpCharXP'), color: '#c9820a' },
    { key: 'combat_xp', label: t('players.detail.xpCombat'), color: '#e05252' },
    { key: 'crafting_xp', label: t('players.detail.xpCrafting'), color: '#5296e0' },
    { key: 'gathering_xp', label: t('players.detail.xpGathering'), color: '#52c080' },
    { key: 'exploration_xp', label: t('players.detail.xpExploration'), color: '#9b59b6' },
    { key: 'sabotage_xp', label: t('players.detail.xpSabotage'), color: '#e07d52' },
  ]

  const toggle = (key: string) => {
    setHidden((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  if (data.length === 0) {
    return (
      <div>
        <SectionLabel>{t('players.detail.xpHistory')}</SectionLabel>
        <p className="text-muted text-sm mt-2">{t('players.detail.xpHistoryEmpty')}</p>
      </div>
    )
  }

  const visibleLines = LINES.filter((l) => !hidden.has(l.key))

  return (
    <div>
      <SectionLabel>{t('players.detail.xpHistory')}</SectionLabel>
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
      <AreaChart
        data={data as unknown as Record<string, string | number>[]}
        height={200}
        margin={{ top: 4, right: 8, bottom: 0, left: 0 }}
      >
        <defs>
          {LINES.map((l) => (
            <linearGradient key={l.key} id={`xp-${l.key}`} x1="0" x2="0" y1="0" y2="1">
              <stop offset="0%" stopColor={l.color} stopOpacity={0.2} />
              <stop offset="100%" stopColor={l.color} stopOpacity={0.02} />
            </linearGradient>
          ))}
        </defs>
        <AreaChart.Grid vertical={false} />
        <AreaChart.XAxis dataKey="snapped_at" tickFormatter={fmtTime} tickMargin={8} />
        <AreaChart.YAxis tickFormatter={fmtXP} width={44} />
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
            fill={`url(#xp-${l.key})`}
          />
        ))}
        <AreaChart.Tooltip
          content={(
            <AreaChart.TooltipContent
              indicator="line"
              labelFormatter={(d) => fmtTime(String(d))}
              valueFormatter={(v) => fmtXP(Number(v))}
            />
          )}
        />
      </AreaChart>
    </div>
  )
}
