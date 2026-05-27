export type Sidebar = 'players' | 'currency' | 'factions' | 'specs' | 'online'

export type ActionSection =
  | 'resources' | 'specs' | 'progression' | 'journey' | 'admin' | 'tags' | 'history' | 'experimental'

export type PlayerSortKey = 'id' | 'name' | 'class' | 'map' | 'faction_id'

export type PacksData = {
  packs: Record<string, {
    name: string
    category: string
    tier: number
    items: { template: string; qty: number; quality: number }[]
  }>
}

export const SIDEBAR_ITEMS: { key: Sidebar; label: string }[] = [
  { key: 'players',  label: 'Players' },
  { key: 'online',   label: 'Online State' },
  { key: 'currency', label: 'Currency' },
  { key: 'factions', label: 'Factions' },
  { key: 'specs',    label: 'Specs / XP' },
]

export const ACTION_SECTIONS: { key: ActionSection; label: string }[] = [
  { key: 'resources',    label: 'Stats' },
  { key: 'specs',        label: 'Specs' },
  { key: 'progression',  label: 'Progression' },
  { key: 'journey',      label: 'Journey' },
  { key: 'admin',        label: 'Admin' },
  { key: 'tags',         label: 'Tags' },
  { key: 'history',      label: 'History' },
  { key: 'experimental', label: 'Experimental' },
]

export const PLAYER_COLUMNS: { key: PlayerSortKey; label: string }[] = [
  { key: 'id',         label: 'ID' },
  { key: 'name',       label: 'Name' },
  { key: 'class',      label: 'Class' },
  { key: 'map',        label: 'Map' },
  { key: 'faction_id', label: 'Faction' },
]

export const XP_TRACKS = ['Combat', 'Crafting', 'Gathering', 'Exploration', 'Sabotage']

export const FACTIONS = [
  { id: 1, name: 'Atreides' },
  { id: 2, name: 'Harkonnen' },
  { id: 3, name: 'None' },
  { id: 4, name: 'Smuggler' },
]
