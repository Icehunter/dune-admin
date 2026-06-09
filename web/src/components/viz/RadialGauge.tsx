import { type ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface GaugeSegment {
  value: number
  color: string
  label?: string
}

interface RadialGaugeProps {
  value: number
  max?: number
  size?: number
  label?: ReactNode
  sub?: ReactNode
  color?: string
  segments?: GaugeSegment[]
  thickness?: number
  unit?: string
  /** Accessible name; the value/unit and any segment labels are appended. */
  title: string
  className?: string
}

const ARC_START = 135
const ARC_SWEEP = 270

function clamp01(n: number): number {
  return !Number.isFinite(n) || n < 0 ? 0 : Math.min(1, n)
}

// Severity-aware arc colour: brand amber under 80%, warning under 90%, danger above.
function capColor(pct: number): string {
  return pct >= 90 ? 'var(--danger)' : pct >= 80 ? 'var(--warning)' : 'var(--accent)'
}

/**
 * 270° dial showing a value (or a stacked composition) against a track, value
 * large in the centre — hand-rolled SVG arc (mirrors the webui RadialGauge).
 * `role="img"` with a `<title>` carrying the numeric reading.
 */
export function RadialGauge({
  value,
  max = 100,
  size = 130,
  label,
  sub,
  color,
  segments,
  thickness = 11,
  unit = '%',
  title,
  className,
}: RadialGaugeProps) {
  const radius = (size - thickness) / 2 - 2
  const cx = size / 2
  const cy = size / 2
  const frac = clamp01(max === 0 ? 0 : value / max)
  const strokeColor = color ?? capColor(max === 100 ? value : frac * 100)
  const segmentLabels = (segments ?? []).map((s) => s.label).filter((l): l is string => Boolean(l)).join(', ')
  const accessibleName = segmentLabels
    ? `${title}: ${value}${unit} — ${segmentLabels}`
    : `${title}: ${value}${unit}`

  const pointAt = (deg: number): [number, number] => {
    const a = (deg * Math.PI) / 180
    return [cx + radius * Math.cos(a), cy + radius * Math.sin(a)]
  }
  const arc = (fromFrac: number, toFrac: number): string => {
    const from = ARC_START + fromFrac * ARC_SWEEP
    const to = ARC_START + toFrac * ARC_SWEEP
    const [x1, y1] = pointAt(from)
    const [x2, y2] = pointAt(to)
    const large = to - from > 180 ? 1 : 0
    return `M ${x1} ${y1} A ${radius} ${radius} 0 ${large} 1 ${x2} ${y2}`
  }

  const slices: { from: number, to: number, color: string }[] = []
  if (segments && segments.length > 0) {
    let cursor = 0
    for (const seg of segments) {
      const segFrac = clamp01(max === 0 ? 0 : seg.value / max)
      const from = Math.min(1, cursor)
      const to = Math.min(1, cursor + segFrac)
      if (to > from) slices.push({ from, to, color: seg.color })
      cursor = to
    }
  }

  return (
    <div className={cn('relative', className)} style={{ width: size, height: size }}>
      <svg viewBox={`0 0 ${size} ${size}`} width={size} height={size} role="img" aria-label={accessibleName}>
        <title>{accessibleName}</title>
        <path d={arc(0, 1)} fill="none" stroke="var(--surface-secondary)" strokeWidth={thickness} strokeLinecap="round" />
        {slices.length > 0
          ? slices.map((slice, i) => (
              <path key={i} d={arc(slice.from, slice.to)} fill="none" stroke={slice.color} strokeWidth={thickness} strokeLinecap="butt" style={{ transition: 'd 0.8s ease-out' }} />
            ))
          : <path d={arc(0, frac)} fill="none" stroke={strokeColor} strokeWidth={thickness} strokeLinecap="round" style={{ transition: 'd 0.8s ease-out' }} />}
      </svg>
      <div className="absolute inset-0 grid place-items-center text-center">
        <div>
          <div className="tabular-nums leading-none font-extrabold tracking-tight text-foreground" style={{ fontSize: size * 0.26 }}>
            {value}
            <span className="font-bold text-muted" style={{ fontSize: size * 0.13 }}>{unit}</span>
          </div>
          {label != null && <div className="mt-1 text-[11.5px] font-bold text-muted">{label}</div>}
          {sub != null && <div className="mt-px text-[10.5px] text-muted">{sub}</div>}
        </div>
      </div>
    </div>
  )
}
