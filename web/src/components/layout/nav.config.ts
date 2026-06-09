/**
 * Single source of truth for the shell navigation. Group headers are literal
 * (new sections); item/sub-item labels reference existing i18n keys, translated
 * at render. `section` items carry inline sub-sections (Database / Welcome) that
 * drive the matching App state (dbSection / welcomeSection).
 */
export interface NavSubItem {
  /** Sub-section key — the dbSection / welcomeSection value. */
  key: string
  labelKey: string
}

export interface NavItem {
  /** Tab id / route segment. */
  tab: string
  /** i18n key for the label (empty when `literal` is used). */
  labelKey: string
  /** Literal label, used when there's no i18n key yet (Dashboard). */
  literal?: string
  /** Lucide icon name (kebab-case). */
  icon: string
  /** Which App section-state these children drive. */
  section?: 'database' | 'welcome'
  children?: NavSubItem[]
}

export interface NavGroup {
  title: string
  items: NavItem[]
}

export const NAV_DASHBOARD: NavItem = {
  tab: 'dashboard',
  labelKey: '',
  literal: 'Dashboard',
  icon: 'layout-dashboard',
}

export const NAV_GROUPS: NavGroup[] = [
  {
    title: 'Server Management',
    items: [
      { tab: 'battlegroup', labelKey: 'nav.battlegroup', icon: 'swords' },
      { tab: 'server', labelKey: 'nav.server', icon: 'settings-2' },
      { tab: 'logs', labelKey: 'nav.logs', icon: 'scroll-text' },
      {
        tab: 'database',
        labelKey: 'nav.database',
        icon: 'database',
        section: 'database',
        children: [
          { key: 'tables', labelKey: 'database.sections.tables' },
          { key: 'describe', labelKey: 'database.sections.describe' },
          { key: 'sample', labelKey: 'database.sections.sample' },
          { key: 'search', labelKey: 'database.sections.search' },
          { key: 'sql', labelKey: 'database.sections.sql' },
        ],
      },
    ],
  },
  {
    title: 'Player Management',
    items: [
      { tab: 'players', labelKey: 'nav.players', icon: 'users' },
      { tab: 'livemap', labelKey: 'nav.liveMap', icon: 'map' },
      { tab: 'storage', labelKey: 'nav.storage', icon: 'package' },
      { tab: 'bases', labelKey: 'nav.bases', icon: 'castle' },
      { tab: 'guilds', labelKey: 'nav.guilds', icon: 'shield' },
      { tab: 'landsraad', labelKey: 'nav.landsraad', icon: 'gavel' },
      { tab: 'blueprints', labelKey: 'nav.blueprints', icon: 'scroll' },
    ],
  },
  {
    title: 'Economy Management',
    items: [
      { tab: 'market', labelKey: 'nav.market', icon: 'store' },
      {
        tab: 'welcome',
        labelKey: 'nav.welcome',
        icon: 'gift',
        section: 'welcome',
        children: [
          { key: 'config', labelKey: 'welcome.sections.config' },
          { key: 'packages', labelKey: 'welcome.sections.packages' },
          { key: 'grants', labelKey: 'welcome.sections.grants' },
        ],
      },
    ],
  },
]

/** Dashboard + every group item, flattened (for lookups and the ⌘K palette). */
export const ALL_NAV_ITEMS: NavItem[] = [NAV_DASHBOARD, ...NAV_GROUPS.flatMap((g) => g.items)]

export function findNavItem(tab: string): NavItem | undefined {
  return ALL_NAV_ITEMS.find((i) => i.tab === tab)
}
