import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'
import type { SolarisPoint } from '../../../api/client'
import { SectionLabel } from '../../../dune-ui'

interface Props {
  data: SolarisPoint[]
}

function fmtBalance(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function fmtTime(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export function SolarisChart({ data }: Props) {
  if (data.length === 0) {
    return (
      <div>
        <SectionLabel>Solaris History</SectionLabel>
        <p className="text-muted text-sm mt-2">No economy events recorded yet.</p>
      </div>
    )
  }

  return (
    <div>
      <SectionLabel>Solaris History</SectionLabel>
      <div className="mt-3 h-48">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={data} margin={{ top: 4, right: 8, left: 8, bottom: 0 }}>
            <XAxis
              dataKey="time"
              tickFormatter={fmtTime}
              tick={{ fontSize: 11, fill: 'var(--muted)' }}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              tickFormatter={fmtBalance}
              tick={{ fontSize: 11, fill: 'var(--muted)' }}
              tickLine={false}
              axisLine={false}
              width={48}
            />
            <Tooltip
              formatter={(val: number) => [fmtBalance(val), 'Balance']}
              labelFormatter={fmtTime}
              contentStyle={{
                background: 'var(--surface)',
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius)',
                fontSize: 12,
              }}
            />
            <Line
              type="monotone"
              dataKey="balance"
              stroke="var(--accent)"
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4 }}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
