import * as React from 'react'
import type { ConnectionBadgeProps } from '../../types'

export const ConnectionBadge: React.FC<ConnectionBadgeProps> = ({ label, connected }) => {
  return (
    <div className="flex items-center gap-1.5 text-xs">
      <div className={`w-2 h-2 rounded-full ${connected ? 'bg-success' : 'bg-muted/40'}`} />
      <span className={connected ? 'text-foreground' : 'text-muted'}>{label}</span>
    </div>
  )
}
