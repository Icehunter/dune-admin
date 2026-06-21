import * as React from 'react'
import type { StatProps } from './interfaces'

export const Stat: React.FC<StatProps> = ({ label, danger, children }) => {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[10px] font-semibold uppercase tracking-wider text-muted">{label}</span>
      <span className={`text-sm font-mono ${danger ? 'text-danger' : 'text-foreground'}`}>{children}</span>
    </div>
  )
}
