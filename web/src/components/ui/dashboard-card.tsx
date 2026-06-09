import { type ReactNode } from 'react'
import { Icon } from '@/dune-ui/Icon'
import { cn } from '@/lib/utils'

interface DashboardCardProps {
  title?: ReactNode
  subtitle?: ReactNode
  /** Lucide icon name (kebab-case) for the tinted header tile. */
  icon?: string
  /** Right-aligned header slot (legend, menu, badge). */
  action?: ReactNode
  className?: string
  children: ReactNode
}

/**
 * A bento-style dashboard panel — generous radius + soft elevation + a card
 * header with a tinted icon tile (mirrors the webui BentoCard + CardHead, in the
 * Dune amber/dark identity). Compose viz/stat widgets inside.
 */
export function DashboardCard({ title, subtitle, icon, action, className, children }: DashboardCardProps) {
  return (
    <section
      className={cn(
        'flex flex-col rounded-[14px] border border-border/70 bg-surface p-[18px]',
        'shadow-[0_2px_8px_rgba(0,0,0,0.32),0_18px_44px_-20px_rgba(0,0,0,0.55)]',
        className,
      )}
    >
      {(title != null || action != null) && (
        <header className="mb-4 flex items-center gap-3">
          {icon && (
            <span className="grid size-9 shrink-0 place-items-center rounded-[10px] bg-primary/12 text-accent-brand">
              <Icon name={icon} className="size-[18px]" />
            </span>
          )}
          <div className="min-w-0 flex-1">
            {title != null && <div className="truncate text-sm font-bold text-foreground">{title}</div>}
            {subtitle != null && <div className="truncate text-xs text-muted">{subtitle}</div>}
          </div>
          {action != null && <div className="shrink-0">{action}</div>}
        </header>
      )}
      {children}
    </section>
  )
}
