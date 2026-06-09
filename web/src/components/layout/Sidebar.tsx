import { useTranslation } from 'react-i18next'
import { Icon } from '@/dune-ui/Icon'
import { cn } from '@/lib/utils'
import { NAV_DASHBOARD, NAV_GROUPS, type NavItem } from './nav.config'

interface SidebarProps {
  currentTab: string
  dbSection: string
  welcomeSection: string
  onNavigate: (tab: string) => void
  onSubNavigate: (section: 'database' | 'welcome', key: string) => void
  collapsed?: boolean
  onToggleCollapse?: () => void
  /** Hide the collapse control (e.g. inside the mobile drawer, always expanded). */
  hideCollapse?: boolean
}

/**
 * Organised, responsive shell sidebar — a Dashboard link plus the Server /
 * Player / Economy Management groups with expandable sub-sections. Collapses to
 * an icon rail on desktop; rendered expanded inside the mobile drawer. Active
 * items use an amber-tinted fill; every row is keyboard-operable with a focus
 * ring. The Icehunter attribution lives in the footer.
 */
export function Sidebar({
  currentTab,
  dbSection,
  welcomeSection,
  onNavigate,
  onSubNavigate,
  collapsed = false,
  onToggleCollapse,
  hideCollapse = false,
}: SidebarProps) {
  const { t } = useTranslation()
  // Nav labels come from config as dynamic keys; the project's t() is strictly
  // typed to known literal keys, so widen it for these config-driven lookups.
  const tr = t as unknown as (key: string) => string
  const label = (item: NavItem) => (item.labelKey ? tr(item.labelKey) : item.literal ?? item.tab)

  const renderItem = (item: NavItem) => {
    const active = currentTab === item.tab
    const sectionVal = item.section === 'database' ? dbSection : item.section === 'welcome' ? welcomeSection : ''
    return (
      <li key={item.tab}>
        <button
          type="button"
          onClick={() => onNavigate(item.tab)}
          aria-current={active ? 'page' : undefined}
          title={collapsed ? label(item) : undefined}
          className={cn(
            'flex w-full items-center gap-2.5 rounded-md border border-transparent text-[13.5px] font-semibold outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
            collapsed ? 'h-10 justify-center' : 'h-9 px-2.5',
            active
              ? 'border-primary/25 bg-primary/12 text-accent-brand'
              : 'text-muted hover:bg-surface-hover hover:text-foreground',
          )}
        >
          <Icon name={item.icon} className="size-[18px] shrink-0" />
          {!collapsed && <span className="flex-1 truncate text-left">{label(item)}</span>}
        </button>

        {!collapsed && active && item.children && item.section && (
          <ul className="mt-0.5 ml-4 flex flex-col gap-0.5 border-l border-border/60 pl-2">
            {item.children.map((c) => {
              const subActive = sectionVal === c.key
              return (
                <li key={c.key}>
                  <button
                    type="button"
                    onClick={() => onSubNavigate(item.section as 'database' | 'welcome', c.key)}
                    aria-current={subActive ? 'page' : undefined}
                    className={cn(
                      'flex h-7 w-full items-center rounded-md px-2.5 text-xs outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
                      subActive ? 'font-semibold text-accent-brand' : 'text-muted hover:bg-surface-hover hover:text-foreground',
                    )}
                  >
                    {tr(c.labelKey)}
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </li>
    )
  }

  return (
    <div className={cn('flex h-full flex-col bg-surface/70 py-3 backdrop-blur-xl', collapsed ? 'w-16 px-2' : 'w-60 px-3')}>
      <div className={cn('mb-3 flex items-center', collapsed ? 'justify-center' : 'justify-between')}>
        <button
          type="button"
          onClick={() => onNavigate('dashboard')}
          aria-label={t('app.goHome')}
          className="flex items-center gap-2 rounded-md outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className="grid size-8 shrink-0 place-items-center rounded-[9px] bg-primary/15 text-accent-brand">
            <Icon name="hexagon" className="size-5" />
          </span>
          {!collapsed && (
            <span className="font-brand text-sm font-extrabold uppercase tracking-[0.16em] text-accent-brand">{t('app.title')}</span>
          )}
        </button>
        {!collapsed && !hideCollapse && onToggleCollapse && (
          <button
            type="button"
            onClick={onToggleCollapse}
            aria-label="Collapse sidebar"
            className="grid size-7 place-items-center rounded-md text-muted outline-none transition-colors hover:bg-surface-hover hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Icon name="panel-left-close" className="size-4" />
          </button>
        )}
      </div>

      <nav className="flex-1 overflow-y-auto" aria-label={t('app.title')}>
        <ul className="flex flex-col gap-0.5">{renderItem(NAV_DASHBOARD)}</ul>
        {NAV_GROUPS.map((g) => (
          <div key={g.title} className="mt-4">
            {!collapsed && (
              <div className="px-2.5 pb-1.5 text-[10px] font-bold uppercase tracking-[0.08em] text-muted/70">{g.title}</div>
            )}
            <ul className="flex flex-col gap-0.5">{g.items.map(renderItem)}</ul>
          </div>
        ))}
      </nav>

      <div className="mt-2 flex flex-col gap-1 border-t border-border/50 pt-2">
        {collapsed && !hideCollapse && onToggleCollapse && (
          <button
            type="button"
            onClick={onToggleCollapse}
            aria-label="Expand sidebar"
            className="grid h-9 place-items-center rounded-md text-muted outline-none transition-colors hover:bg-surface-hover hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Icon name="panel-left-open" className="size-4" />
          </button>
        )}
        <a
          href="https://github.com/Icehunter/dune-admin"
          target="_blank"
          rel="noreferrer"
          title="dune-admin by Icehunter"
          className={cn(
            'flex items-center gap-2 rounded-md px-2.5 py-1.5 text-[11px] text-muted outline-none transition-colors hover:bg-surface-hover hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring',
            collapsed && 'justify-center px-0',
          )}
        >
          <Icon name="github" className="size-4 shrink-0" />
          {!collapsed && <span className="truncate">by Icehunter</span>}
        </a>
      </div>
    </div>
  )
}
