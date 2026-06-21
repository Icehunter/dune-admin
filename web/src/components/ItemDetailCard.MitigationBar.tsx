import * as React from 'react'
import type { MitigationBarProps } from './interfaces'

export const MitigationBar: React.FC<MitigationBarProps> = ({ label, value }) => {
  const pct = Math.round(value * 100)
  return (
    <div className="flex items-center gap-2 text-xs py-0.5">
      <span className="text-muted shrink-0 w-20">{label}</span>
      <div className="flex-1 h-1.5 rounded-full bg-surface-secondary overflow-hidden">
        <div className="h-full bg-accent rounded-full" style={{ width: `${pct}%` }} />
      </div>
      <span className="text-muted tabular-nums w-8 text-right">
        {pct}
        %
      </span>
    </div>
  )
}
