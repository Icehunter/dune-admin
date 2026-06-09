import type React from 'react'
import type { ReactNode } from 'react'

interface SectionLabelProps {
  children: ReactNode
}

/**
 * Small uppercase amber label — sub-section heading inside a Panel. An <h3>:
 * it sits directly under the page title (PageHeader, <h2>) in most tabs, so a
 * lower level would skip a heading rank (WCAG heading-order). Valid too when
 * nested under a [[SectionDivider]] (also <h3>) — same rank, no skip.
 */
export const SectionLabel: React.FC<SectionLabelProps> = ({ children }) => {
  return (
    <h3 className="text-xs font-semibold uppercase tracking-widest text-accent border-l-2 border-accent/60 pl-2">
      {children}
    </h3>
  )
}
