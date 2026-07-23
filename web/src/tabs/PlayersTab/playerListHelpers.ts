import type { Player } from '../../api/client'
import { FACTIONS } from './types'
import type { PlayerSortKey, SortDir } from './types'

// collate is the shared string comparator for every sort axis (#281): numeric
// so "Bob2" sorts before "Bob10", case-insensitive so casing doesn't split an
// otherwise-alphabetical run.
export const collate = (a: string, b: string): number =>
  a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' })

// factionLabel resolves a faction_id to its display name. id 0 means "no
// faction picked yet" (unalignedLabel, e.g. t('players.detail.unaligned')) —
// distinct from the in-game "None" faction (id 3, in FACTIONS). Unknown ids
// fall back to their raw numeric string defensively.
export const factionLabel = (factionId: number, unalignedLabel: string): string => {
  if (factionId === 0) return unalignedLabel
  const known = FACTIONS.find((f) => f.id === factionId)
  return known ? known.name : String(factionId)
}

// sortValue is the per-axis sort key extracted from a player: name/class/map
// sort on their raw string, faction sorts on its display label (not the raw
// id, so it groups by human-readable faction name), id sorts numerically.
export const sortValue = (p: Player, key: PlayerSortKey, unalignedLabel: string): string | number => {
  switch (key) {
    case 'id': return p.id
    case 'faction_id': return factionLabel(p.faction_id, unalignedLabel)
    case 'class': return p.class
    case 'map': return p.map
    case 'name':
    default: return p.name
  }
}

// comparePlayers is the full SORT comparator (#281): a single axis + direction,
// orthogonal to the FILTER facets applied before it. Non-name axes tie-break
// on name so equal-value groups (e.g. same faction) still read alphabetically.
export const comparePlayers = (
  a: Player,
  b: Player,
  key: PlayerSortKey,
  dir: SortDir,
  unalignedLabel: string,
): number => {
  const av = sortValue(a, key, unalignedLabel)
  const bv = sortValue(b, key, unalignedLabel)
  let primary = typeof av === 'number' && typeof bv === 'number'
    ? av - bv
    : collate(String(av), String(bv))
  if (primary === 0 && key !== 'name') primary = collate(a.name, b.name)
  return dir === 'asc' ? primary : -primary
}
