import type React from 'react'
import type { ReactNode } from 'react'
import { Widget } from '@heroui-pro/react'

type PanelProps = {
  children: ReactNode
  className?: string
}

export const Panel: React.FC<PanelProps> = ({ children, className = '' }) => (
  <Widget className="dune-panel">
    <Widget.Content className={`flex flex-col gap-2 ${className}`}>
      {children}
    </Widget.Content>
  </Widget>
)
