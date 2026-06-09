import { type ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface DonutSegment {
  /** Share of the ring this segment occupies, 0–100. */
  pct: number
  /** Segment colour (CSS colour or token reference). */
  color: string
  /** Optional name, folded into the accessible description. */
  label?: string
}

interface DonutProps {
  segments: DonutSegment[]
  size?: number
  thickness?: number
  centerTop?: ReactNode
  centerMain?: ReactNode
  centerSub?: ReactNode
  /** Accessible name; segment labels are folded in so meaning isn't colour-only. */
  title: string
  className?: string
}

/**
 * Segmented capacity ring with a value in the middle — hand-rolled SVG using
 * `stroke-dasharray` arcs over a subtle track (mirrors the webui Donut). The
 * dasharray transition is neutralised globally under prefers-reduced-motion.
 */
export function Donut({
  segments, size = 150, thickness = 18, centerTop, centerMain, centerSub, title, className,
}: DonutProps) {
  const radius = (size - thickness) / 2 - 2
  const cx = size / 2
  const cy = size / 2
  const circ = 2 * Math.PI * radius
  const labels = segments.map((s) => s.label).filter((l): l is string => Boolean(l)).join(', ')
  const accessibleName = labels ? `${title}: ${labels}` : title

  const arcs = segments.reduce<{ color: string, len: number, offset: number }[]>((acc, s) => {
    const len = (s.pct / 100) * circ
    const prev = acc.length > 0 ? acc[acc.length - 1] : undefined
    const offset = prev ? prev.offset + prev.len : 0
    acc.push({ color: s.color, len, offset })
    return acc
  }, [])

  return (
    <div className={cn('relative', className)} style={{ width: size, height: size }}>
      <svg
        viewBox={`0 0 ${size} ${size}`}
        width={size}
        height={size}
        role="img"
        aria-label={accessibleName}
        style={{ transform: 'rotate(-90deg)' }}
      >
        <title>{accessibleName}</title>
        <circle cx={cx} cy={cy} r={radius} fill="none" stroke="var(--surface-secondary)" strokeWidth={thickness} />
        {arcs.map((arc, i) => (
          <circle
            key={i}
            cx={cx}
            cy={cy}
            r={radius}
            fill="none"
            stroke={arc.color}
            strokeWidth={thickness}
            strokeDasharray={`${arc.len} ${circ - arc.len}`}
            strokeDashoffset={-arc.offset}
            strokeLinecap="butt"
            style={{ transition: 'stroke-dasharray 0.8s ease-out' }}
          />
        ))}
      </svg>
      <div className="absolute inset-0 grid place-items-center text-center">
        <div>
          {centerTop != null && (
            <div className="text-[10.5px] font-bold tracking-[0.06em] text-muted uppercase">{centerTop}</div>
          )}
          {centerMain != null && (
            <div className="tabular-nums font-extrabold tracking-tight text-foreground" style={{ fontSize: size * 0.2, lineHeight: 1.05 }}>
              {centerMain}
            </div>
          )}
          {centerSub != null && <div className="mt-0.5 text-[11px] text-muted">{centerSub}</div>}
        </div>
      </div>
    </div>
  )
}
