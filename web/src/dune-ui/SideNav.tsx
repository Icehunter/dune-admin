import { useEffect, useRef } from 'react'
import { ListView } from '@heroui-pro/react'

import type { SideNavProps } from './types'

const ROW_HEIGHT = 56

export const SideNav = <K extends string>({
  items, active, onSelect, title, titleAction, width, children,
}: SideNavProps<K>) => {
  const w = width ?? 'w-60'
  const wrapperRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (active == null) return
    const idx = items.findIndex((i) => i.key === active)
    if (idx < 0) return
    // Defer so the Virtualizer finishes its initial layout before we scroll
    const timer = setTimeout(() => {
      if (!wrapperRef.current) return
      // children[0] = ListView root (role="grid") which owns overflow-y-auto
      const scrollEl = wrapperRef.current.children[0] as HTMLElement | null
      if (!scrollEl) return
      const itemTop = idx * ROW_HEIGHT
      const { clientHeight } = scrollEl
      scrollEl.scrollTo({
        top: Math.max(0, itemTop - clientHeight / 2 + ROW_HEIGHT / 2),
        behavior: 'smooth',
      })
    }, 80)
    return () => clearTimeout(timer)
  }, [active, items])

  return (
    <div className={`${w} shrink-0 flex flex-col rounded-[var(--radius)] bg-surface border border-border/60 dune-lift overflow-hidden`}>
      {(title || titleAction) && (
        <div className="flex items-center justify-between px-3 pt-6 pb-3 border-b border-border/60 shrink-0">
          {title && <span className="text-xs font-semibold uppercase tracking-widest text-accent">{title}</span>}
          {titleAction}
        </div>
      )}
      {children && <div className="px-3 py-1.5 shrink-0">{children}</div>}

      {/* wrapperRef gives us the ListView's DOM scroll element via children[0] */}
      <div ref={wrapperRef} className="relative flex-1 min-h-0">
        <ListView
          aria-label={typeof title === 'string' ? title : 'Navigation'}
          items={items}
          selectedKeys={active != null ? new Set([active]) : new Set()}
          selectionMode="single"
          selectionBehavior="replace"
          variant="secondary"
          virtualized
          rowHeight={ROW_HEIGHT}
          className="h-full overflow-y-auto overflow-x-hidden [scrollbar-width:none] [&::-webkit-scrollbar]:hidden pt-2 pb-24 [scroll-padding-bottom:96px]"
          onSelectionChange={(keys) => {
            if (keys === 'all') return
            const k = [...(keys as Set<K>)][0]
            if (k !== undefined) onSelect(k)
          }}
        >
          {(item) => {
            const isActive = item.key === active
            return (
              <ListView.Item
                id={item.key}
                textValue={typeof item.label === 'string' ? item.label : item.key}
                className={[
                  '!px-3 !py-2',
                  item.depth ? 'pl-4' : '',
                  'data-[hovered=true]:!bg-[color-mix(in_srgb,var(--accent)_12%,transparent)]',
                  'data-[selected=true]:![background:linear-gradient(90deg,color-mix(in_srgb,var(--accent)_32%,transparent),color-mix(in_srgb,var(--accent)_14%,transparent))]',
                  'data-[selected=true]:!border-[color-mix(in_srgb,var(--accent)_55%,transparent)]',
                  'data-[selected=true]:![box-shadow:inset_0_0_18px_color-mix(in_srgb,var(--accent)_15%,transparent)]',
                  'data-[selected=true]:data-[hovered=true]:![background:linear-gradient(90deg,color-mix(in_srgb,var(--accent)_38%,transparent),color-mix(in_srgb,var(--accent)_18%,transparent))]',
                ].filter(Boolean).join(' ')}
              >
                <ListView.ItemContent>
                  {item.icon != null && (
                    <div className="shrink-0">
                      {typeof item.icon === 'function' ? item.icon(isActive) : item.icon}
                    </div>
                  )}
                  <div className="flex flex-col flex-1 min-w-0">
                    <ListView.Title className={isActive ? '!text-[var(--color-focus)] !font-semibold' : ''}>
                      {item.label}
                    </ListView.Title>
                    {item.sublabel != null && <ListView.Description>{item.sublabel}</ListView.Description>}
                  </div>
                </ListView.ItemContent>
                {item.hint != null && (
                  <ListView.ItemAction>
                    <div className="text-xs shrink-0">
                      {typeof item.hint === 'function' ? item.hint(isActive) : item.hint}
                    </div>
                  </ListView.ItemAction>
                )}
              </ListView.Item>
            )
          }}
        </ListView>

        {/* 96px gradient — matches corner art depth and scroll-padding-bottom */}
        <div
          className="pointer-events-none absolute bottom-0 left-0 right-0 h-24"
          style={{ background: 'linear-gradient(to bottom, transparent, var(--color-surface))' }}
        />
      </div>
    </div>
  )
}
