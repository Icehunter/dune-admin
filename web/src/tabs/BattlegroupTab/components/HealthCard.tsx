import * as React from 'react'
import type { ReactNode } from 'react'
import { Icon, SectionLabel } from '../../../dune-ui'

export const HealthCard: React.FC<{
  title: string
  icon?: string
  accessory?: ReactNode
  className?: string
  children: ReactNode
}> = ({ title, icon, accessory, className = '', children }) => (
  <div className={`rounded-[var(--radius)] p-8 flex flex-col gap-3 bg-surface-secondary border border-border dune-lift ${className}`}>
    <div className="flex items-center justify-between gap-2">
      <div className="flex items-center gap-2">
        {icon && <Icon name={icon} className="size-4 text-accent" />}
        <SectionLabel>{title}</SectionLabel>
      </div>
      {accessory}
    </div>
    {children}
  </div>
)
