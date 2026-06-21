import * as React from 'react'
import type { FormSectionProps } from './interfaces'

export const FormSection: React.FC<FormSectionProps> = ({ children, className }) => (
  <div className={`flex flex-col gap-3 rounded-[var(--radius)] border border-border bg-surface-secondary p-4 dune-lift ${className ?? ''}`}>
    {children}
  </div>
)
