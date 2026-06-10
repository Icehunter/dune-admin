import type { ReactNode } from 'react'

type RenderSlot = ReactNode | ((active: boolean) => ReactNode)

type Item<K extends string> = {
  key: K
  label: ReactNode
  /** Optional sub-label rendered below the main label (e.g. namespace, item count). */
  sublabel?: ReactNode
  /** Optional right-aligned hint (e.g. "18 items" count chip). Receives active state when a function. */
  hint?: RenderSlot
  /** Optional left icon/avatar rendered outside the truncated label area. Receives active state when a function. */
  icon?: RenderSlot
  /** Indentation level (0 = top-level, 1 = child item). */
  depth?: number
}

type SideNavProps<K extends string> = {
  items: Item<K>[]
  active: K | null
  onSelect: (key: K) => void
  /** Header text shown above the list (e.g. "PODS", "CONTAINERS (70)"). */
  title?: ReactNode
  /** Action element rendered next to the title (e.g. a refresh button). */
  titleAction?: ReactNode
  /** Width of the side nav. Defaults to 240px (w-60). */
  width?: string
  children?: ReactNode
}

/**
 * Reusable left side-navigation panel: bordered card with a title row +
 * scrollable list of selectable items. Used by Players sidebar, Database
 * section nav, Logs pod list, Storage container list, etc.
 *
 * Pass arbitrary `children` to render extra content (search inputs, info
 * banners) between the title and the list.
 */
export const SideNav = <K extends string>({
  items, active, onSelect, title, titleAction, width, children,
}: SideNavProps<K>) => {
  const w = width ?? 'w-60'
  return (
    <div className={`${w} shrink-0 flex flex-col rounded-[var(--radius)] bg-surface border border-border/60 dune-lift overflow-hidden`}>
      {(title || titleAction) && (
        <div className="flex items-center justify-between px-3 py-2 border-b border-border/60 shrink-0 bg-gradient-to-b from-(--surface-secondary) to-transparent">
          {title && <span className="text-xs font-semibold uppercase tracking-widest text-accent">{title}</span>}
          {titleAction}
        </div>
      )}
      {children && <div className="px-2 py-1.5 shrink-0">{children}</div>}
      <div className="overflow-y-auto flex-1 flex flex-col gap-0.5 p-1">
        {items.map((item) => {
          const isActive = item.key === active
          return (
            <button
              key={item.key}
              onClick={() => onSelect(item.key)}
              className={
                'text-left rounded-[var(--radius)] text-sm transition-colors flex items-center gap-2 '
                + (item.depth ? 'pl-6 pr-3 py-1.5 ' : 'px-3 py-2 ')
                + (isActive
                  ? 'bg-accent text-accent-foreground font-semibold'
                  : 'text-foreground hover:bg-surface-hover')
              }
            >
              {item.icon && <div className="shrink-0">{typeof item.icon === 'function' ? item.icon(isActive) : item.icon}</div>}
              <div className="flex-1 min-w-0">
                <div className="truncate">{item.label}</div>
                {item.sublabel && (
                  <div className={'truncate text-xs ' + (isActive ? 'opacity-80' : 'text-muted')}>
                    {item.sublabel}
                  </div>
                )}
              </div>
              {item.hint && <div className="shrink-0 text-xs">{typeof item.hint === 'function' ? item.hint(isActive) : item.hint}</div>}
            </button>
          )
        })}
      </div>
    </div>
  )
}
