import * as React from 'react'

import type { SectionLabelProps } from './interfaces'

/**
 * Small uppercase amber label — sub-section heading inside a Panel.
 * Pairs with [[PageHeader]] (top-level) and [[SectionDivider]] (mid-level).
 */
export const SectionLabel: React.FC<SectionLabelProps> = ({ children }) => {
  return (
    <h4 className="text-xs font-semibold uppercase tracking-widest text-accent border-l-2 border-(--accent-soft-border) pl-2">
      {children}
    </h4>
  )
}
