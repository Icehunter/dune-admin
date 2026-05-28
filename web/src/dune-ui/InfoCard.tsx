import type { ReactNode } from 'react'

type CardProps = { children: ReactNode; className?: string }

type ItemProps = {
  label: ReactNode
  value: ReactNode
  /** Optional explicit value text color (e.g. phase status color). */
  valueColor?: string
}

/**
 * Bordered, slightly-elevated label/value row card — the "Phase Reconciling
 * | Database Ready" health row pattern from BattlegroupTab.
 */
export function InfoCard({ children, className = '' }: CardProps) {
  return (
    <div
      className={
        'flex items-center gap-6 rounded-[var(--radius)] px-4 py-3 text-sm shrink-0 ' +
        'bg-surface border border-border/60 dune-lift ' +
        className
      }
    >
      {children}
    </div>
  )
}

export function InfoCardItem({ label, value, valueColor }: ItemProps) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-muted">{label}</span>
      <span className="font-semibold" style={valueColor ? { color: valueColor } : undefined}>
        {value}
      </span>
    </div>
  )
}

// Namespace alias kept for callers using <InfoCard.Item>
InfoCard.Item = InfoCardItem
