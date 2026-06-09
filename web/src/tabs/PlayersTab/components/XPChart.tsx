import type React from 'react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  LineChart, Line, XAxis, YAxis, Tooltip, Legend, ResponsiveContainer,
} from 'recharts'
import type { StatSnapshot } from '../../../api/client'
import { SectionLabel } from '../../../dune-ui'

interface XPChartProps {
  data: StatSnapshot[]
}

function fmtXP(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return String(n)
}

function fmtTime(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export const XPChart: React.FC<XPChartProps> = ({ data }) => {
  const { t } = useTranslation()
  const [hidden, setHidden] = useState<Set<string>>(new Set())

  const LINES: { key: keyof StatSnapshot, label: string, color: string }[] = [
    { key: 'char_xp', label: t('players.detail.xpCharXP'), color: '#1574e5' },
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
        <p className="text-muted text-sm mt-2">
          {t('players.detail.xpHistoryEmpty')}
        </p>
      </div>
    )
  }

  const visibleLines = LINES.filter((l) => !hidden.has(l.key))

  return (
    <div>
      <SectionLabel>{t('players.detail.xpHistory')}</SectionLabel>
      <div className="mt-3 h-56">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={data} margin={{ top: 4, right: 8, left: 8, bottom: 0 }}>
            <XAxis
              dataKey="snapped_at"
              tickFormatter={fmtTime}
              tick={{ fontSize: 11, fill: 'var(--muted)' }}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              tickFormatter={fmtXP}
              tick={{ fontSize: 11, fill: 'var(--muted)' }}
              tickLine={false}
              axisLine={false}
              width={44}
            />
            <Tooltip
              formatter={(val, name) => [fmtXP(val as number), String(name)]}
              labelFormatter={(d) => fmtTime(String(d))}
              contentStyle={{
                background: 'var(--surface)',
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius)',
                fontSize: 12,
              }}
            />
            <Legend
              onClick={(e) => toggle(e.dataKey as string)}
              formatter={(value, entry) => (
                <span style={{ color: hidden.has((entry as { dataKey: string }).dataKey) ? 'var(--muted)' : undefined }}>
                  {value}
                </span>
              )}
            />
            {visibleLines.map((l) => (
              <Line
                key={l.key}
                type="monotone"
                dataKey={l.key}
                name={l.label}
                stroke={l.color}
                strokeWidth={2}
                dot={false}
                activeDot={{ r: 4 }}
                connectNulls
              />
            ))}
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
