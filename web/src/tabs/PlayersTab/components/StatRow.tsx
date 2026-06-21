import * as React from 'react'
import type { StatRowProps } from './interfaces'

export const StatRow: React.FC<StatRowProps> = ({ label, value }) => {
  return (
    <div className="flex items-center justify-between py-1 border-b border-border/30 last:border-0">
      <span className="text-sm text-muted">{label}</span>
      <span className="text-sm font-semibold">{value}</span>
    </div>
  )
}
