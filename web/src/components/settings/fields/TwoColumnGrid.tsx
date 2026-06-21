import * as React from 'react'
import type { GridRowProps } from '../../interfaces'

export const TwoColumnGrid: React.FC<GridRowProps> = ({ children }) => {
  return <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-1">{children}</div>
}
