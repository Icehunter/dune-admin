import * as React from 'react'
import { Widget } from '@heroui-pro/react'
import type { PanelProps } from './types'

export const Panel: React.FC<PanelProps> = ({ children, className = '' }) => (
  <Widget className={`dune-panel ${className}`}>
    <Widget.Content className="flex flex-col gap-2 !p-8">
      {children}
    </Widget.Content>
  </Widget>
)
