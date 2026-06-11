import * as React from 'react'
import type { StatusDotProps } from './types'

export const StatusDot: React.FC<StatusDotProps> = ({ status }) => {
  const cls
    = status === 'Online'
      ? 'bg-success'
      : status === 'LoggingOut'
        ? 'bg-warning'
        : 'bg-muted'
  return <span className={`inline-block w-2 h-2 rounded-full mr-1.5 shrink-0 ${cls}`} />
}
