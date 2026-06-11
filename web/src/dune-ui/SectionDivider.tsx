import * as React from 'react'
import type { SectionDividerProps } from './types'

/**
 * Amber section title with a top border + padding above to separate it from
 * the preceding section. Matches the "Server Control" divider in
 * BattlegroupTab.
 */
export const SectionDivider: React.FC<SectionDividerProps> = ({ title, children }) => (
  <div className="flex items-center gap-3 border-t border-(--accent-soft-border)/30 pt-3 mt-3 shrink-0">
    <h3 className="text-base font-semibold text-accent flex-1 border-l-2 border-(--accent-soft-border) pl-2">{title}</h3>
    {children && <div className="flex items-center gap-2 shrink-0">{children}</div>}
  </div>
)
