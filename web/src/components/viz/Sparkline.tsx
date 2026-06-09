import { useId } from 'react'
import { cn } from '@/lib/utils'

interface SparklineProps {
  /** The series to plot. Needs at least two points to draw a line. */
  data: number[]
  color?: string
  fill?: boolean
  height?: number
  strokeWidth?: number
  /** Accessible name, rendered into the SVG `<title>`. */
  title: string
  className?: string
}

const VIEW_W = 240

interface Point {
  x: number
  y: number
}

function computePoints(data: number[], height: number): Point[] {
  if (data.length < 2) return []
  const max = Math.max(...data)
  const min = Math.min(...data)
  const range = max - min || 1
  const step = VIEW_W / (data.length - 1)
  return data.map((d, i) => ({
    x: i * step,
    y: height - 6 - ((d - min) / range) * (height - 12),
  }))
}

function toLinePath(points: Point[]): string {
  return points.map((p, i) => `${i ? 'L' : 'M'}${p.x.toFixed(1)} ${p.y.toFixed(1)}`).join(' ')
}

function toAreaPath(points: Point[], height: number): string {
  return `${toLinePath(points)} L${VIEW_W} ${height} L0 ${height} Z`
}

/**
 * Compact width-filling trend line with an optional gradient fill — for a
 * recent-history glance (mirrors the webui Sparkline). The terminal dot marks
 * the latest sample.
 */
export function Sparkline({ data, color = 'var(--accent)', fill = true, height = 44, strokeWidth = 2, title, className }: SparklineProps) {
  const gradientId = useId()
  const points = computePoints(data, height)
  const last = points.at(-1)

  return (
    <svg
      viewBox={`0 0 ${VIEW_W} ${height}`}
      width="100%"
      height={height}
      preserveAspectRatio="none"
      role="img"
      aria-label={title}
      className={cn('block overflow-visible', className)}
    >
      <title>{title}</title>
      {last && (
        <>
          <defs>
            <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity="0.28" />
              <stop offset="100%" stopColor={color} stopOpacity="0" />
            </linearGradient>
          </defs>
          {fill && <path d={toAreaPath(points, height)} fill={`url(#${gradientId})`} />}
          <path
            d={toLinePath(points)}
            fill="none"
            stroke={color}
            strokeWidth={strokeWidth}
            strokeLinejoin="round"
            strokeLinecap="round"
            vectorEffect="non-scaling-stroke"
          />
          <circle cx={last.x} cy={last.y} r="3" fill={color} vectorEffect="non-scaling-stroke" />
        </>
      )}
    </svg>
  )
}
