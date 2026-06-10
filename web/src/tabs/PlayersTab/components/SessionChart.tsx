import type React from 'react'
import { useTranslation } from 'react-i18next'
import { BarChart } from '@heroui-pro/react'
import type { SessionRecord } from '../../../api/client'
import { SectionLabel } from '../../../dune-ui'

interface SessionChartProps {
  data: SessionRecord[]
}

type DayBucket = { date: string, minutes: number }

const WINDOW_DAYS = 14

function todayUTC(): string {
  return new Date().toISOString().slice(0, 10)
}

function aggregate(records: SessionRecord[]): DayBucket[] {
  const minutesByDay = new Map<string, number>()
  for (const r of records) {
    const day = r.started_at.slice(0, 10)
    minutesByDay.set(day, (minutesByDay.get(day) ?? 0) + Math.round(r.duration_secs / 60))
  }

  const buckets: DayBucket[] = []
  const today = todayUTC()
  for (let i = WINDOW_DAYS - 1; i >= 0; i--) {
    const d = new Date(today + 'T12:00:00Z')
    d.setUTCDate(d.getUTCDate() - i)
    const date = d.toISOString().slice(0, 10)
    buckets.push({ date, minutes: minutesByDay.get(date) ?? 0 })
  }
  return buckets
}

function fmtDate(d: string): string {
  return new Date(d + 'T12:00:00Z').toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export const SessionChart: React.FC<SessionChartProps> = ({ data }) => {
  const { t } = useTranslation()
  const buckets = aggregate(data)

  if (data.length === 0) {
    return (
      <div>
        <SectionLabel>{t('players.detail.sessionHistory')}</SectionLabel>
        <p className="text-muted text-sm mt-2">
          {t('players.detail.sessionHistoryEmpty')}
        </p>
      </div>
    )
  }

  return (
    <div>
      <SectionLabel>{t('players.detail.sessionHistory')}</SectionLabel>
      <BarChart data={buckets} height={140} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
        <BarChart.Grid vertical={false} />
        <BarChart.XAxis dataKey="date" tickFormatter={fmtDate} tickMargin={8} />
        <BarChart.YAxis width={36} tickFormatter={(v: number) => `${v}m`} />
        <BarChart.Bar
          dataKey="minutes"
          fill="var(--accent)"
          radius={[3, 3, 0, 0]}
          maxBarSize={32}
          name={t('players.detail.playtime')}
        />
        <BarChart.Tooltip
          content={(
            <BarChart.TooltipContent
              labelFormatter={(d) => fmtDate(String(d))}
              valueFormatter={(v) => `${v}m`}
            />
          )}
        />
      </BarChart>
    </div>
  )
}
