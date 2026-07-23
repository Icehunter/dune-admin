export type DetailTab = 'overview' | 'inventory' | 'vehicles' | 'give' | 'actions'

export type ActionSection
  = | 'resources' | 'specs' | 'progression' | 'contracts' | 'journey' | 'admin' | 'tags' | 'history' | 'experimental'

export type PlayerSortKey = 'id' | 'name' | 'class' | 'map' | 'faction_id'

export type SortDir = 'asc' | 'desc'

export type PlayerStatusFilter = 'all' | 'online' | 'offline'

export type PacksData = {
  packs: Record<string, {
    name: string
    category: string
    tier: number
    items: { template: string, qty: number, quality: number }[]
  }>
}

export const ACTION_SECTIONS: { key: ActionSection, label: string }[] = [
  { key: 'resources', label: 'players.actions.sections.resources' },
  { key: 'specs', label: 'players.actions.sections.specs' },
  { key: 'progression', label: 'players.actions.sections.progression' },
  { key: 'contracts', label: 'players.actions.sections.contracts' },
  { key: 'journey', label: 'players.actions.sections.journey' },
  { key: 'admin', label: 'players.actions.sections.admin' },
  { key: 'tags', label: 'players.actions.sections.tags' },
  { key: 'history', label: 'players.actions.sections.history' },
  { key: 'experimental', label: 'players.actions.sections.experimental' },
]

// Sort-axis menu (#281): `label` is an i18n key, resolved with `t(label as
// never)` at render time in PlayerListControls — same dynamic-key pattern as
// ACTION_SECTIONS above. Order here is the dropdown display order — Name
// first as the primary/default axis, ID last as the least-used one.
export const PLAYER_COLUMNS: { key: PlayerSortKey, label: string }[] = [
  { key: 'name', label: 'players.sort.columns.name' },
  { key: 'faction_id', label: 'players.sort.columns.faction_id' },
  { key: 'class', label: 'players.sort.columns.class' },
  { key: 'map', label: 'players.sort.columns.map' },
  { key: 'id', label: 'players.sort.columns.id' },
]

export const XP_TRACKS = ['Combat', 'Crafting', 'Gathering', 'Exploration', 'Sabotage']

export const FACTIONS = [
  { id: 1, name: 'Atreides' },
  { id: 2, name: 'Harkonnen' },
  { id: 3, name: 'None' },
  { id: 4, name: 'Smuggler' },
]
