import { type ReactNode } from 'react'
import { Icon } from '@/dune-ui/Icon'
import { cn } from '@/lib/utils'

interface StatTileProps {
  label: ReactNode
  value: ReactNode
  unit?: ReactNode
  /** Lucide icon name (kebab-case), shown in a tinted tile. */
  icon?: string
  hint?: ReactNode
  className?: string
}

/**
 * Compact KPI: a tinted icon tile + label, a big tabular-numeral value with an
 * optional unit, and an optional hint line. Mirrors the webui StatTile.
 */
export function StatTile({ label, value, unit, icon, hint, className }: StatTileProps) {
  return (
    <div className={cn('flex flex-col gap-2', className)}>
      <div className="flex items-center gap-2">
        {icon && (
          <span className="grid size-7 shrink-0 place-items-center rounded-[8px] bg-primary/12 text-accent-brand">
            <Icon name={icon} className="size-4" />
          </span>
        )}
        <span className="truncate text-xs font-semibold text-muted">{label}</span>
      </div>
      <div className="flex items-baseline gap-1">
        <span className="tabular-nums text-2xl font-extrabold tracking-tight text-foreground">{value}</span>
        {unit && <span className="text-sm font-bold text-muted">{unit}</span>}
      </div>
      {hint && <div className="text-[11px] text-muted">{hint}</div>}
    </div>
  )
}
