import type { TabId } from '../../types'

export const TAB_IDS = [
  'dashboard',
  'battlegroup',
  'players',
  'database',
  'logs',
  'blueprints',
  'bases',
  'guilds',
  'landsraad',
  'storage',
  'livemap',
  'server',
  'director',
  'market',
  'welcome',
  'events',
  'battlepass',
  'permissions',
  'diagnostics',
] as const

export const DEFAULT_TAB: TabId = 'dashboard'

export const currentTabFromPath = (pathname: string): TabId => {
  const seg = pathname.replace(/^\//, '').split('/')[0]
  return (TAB_IDS as readonly string[]).includes(seg) ? (seg as TabId) : DEFAULT_TAB
}

// Lucide icon per top-level tab, shown in the collapsible sidebar rail.
export const TAB_ICONS: Record<TabId, string> = {
  dashboard: 'layout-grid',
  battlegroup: 'activity',
  logs: 'scroll-text',
  database: 'database',
  server: 'settings-2',
  director: 'clapperboard',
  players: 'users',
  livemap: 'map',
  storage: 'package',
  bases: 'house',
  guilds: 'shield',
  landsraad: 'landmark',
  blueprints: 'scroll',
  market: 'store',
  welcome: 'gift',
  events: 'calendar-clock',
  battlepass: 'medal',
  permissions: 'lock',
  diagnostics: 'stethoscope',
}

// Read-level capability required to see each tab when backend auth is on.
// 'owner' is special: only owners (guild owner, configured owners, local
// account) see the Permissions tab.
export const TAB_CAPABILITIES: Record<TabId, string> = {
  dashboard: 'server:read',
  battlegroup: 'server:read',
  logs: 'logs:read',
  database: 'database:read',
  server: 'config:read',
  director: 'config:read',
  players: 'players:read',
  livemap: 'world:read',
  storage: 'world:read',
  bases: 'world:read',
  guilds: 'players:read',
  landsraad: 'players:read',
  blueprints: 'world:read',
  market: 'market:read',
  welcome: 'welcome:read',
  events: 'events:read',
  battlepass: 'battlepass:track',
  permissions: 'owner',
  diagnostics: 'owner',
}

export const BETA_TABS = new Set<TabId>(['events', 'battlepass'])

export interface NavGroup {
  title: string
  items: { key: TabId, label: string }[]
}
