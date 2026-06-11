import type React from 'react'
import type { ReactNode } from 'react'
import { Widget } from '@heroui-pro/react'

type PanelProps = {
  children: ReactNode
  className?: string
}

export const Panel: React.FC<PanelProps> = ({ children, className = '' }) => (
  <Widget className={`dune-panel ${className}`}>
    <Widget.Content className="flex flex-col gap-2">
      {children}
    </Widget.Content>
  </Widget>
)
