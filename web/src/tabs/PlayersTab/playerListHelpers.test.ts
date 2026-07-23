import { describe, it, expect } from 'vitest'
import { collate, factionLabel, sortValue, comparePlayers } from './playerListHelpers'
import type { Player } from '../../api/client'

const UNALIGNED = 'Unaligned'

const makePlayer = (overrides: Partial<Player>): Player => ({
  id: 1,
  account_id: 1,
  controller_id: 1,
  fls_id: 'fls-1',
  name: 'Player',
  class: 'PlayerCharacter',
  map: 'HaggaBasin',
  faction_id: 0,
  online_status: 'Offline',
  discord_user_id: '',
  discord_avatar: '',
  ...overrides,
})

describe('collate', () => {
  it('is case-insensitive', () => {
    expect(collate('bob', 'Bob')).toBe(0)
    expect(collate('alice', 'Bob') < 0).toBe(true)
  })

  it('is numeric-aware — Bob2 sorts before Bob10', () => {
    expect(collate('Bob2', 'Bob10') < 0).toBe(true)
    expect(collate('Bob10', 'Bob2') > 0).toBe(true)
  })
})

describe('factionLabel', () => {
  it('maps id 0 to the unaligned label, not the "None" faction', () => {
    expect(factionLabel(0, UNALIGNED)).toBe(UNALIGNED)
  })

  it('resolves known faction ids to their name', () => {
    expect(factionLabel(1, UNALIGNED)).toBe('Atreides')
    expect(factionLabel(2, UNALIGNED)).toBe('Harkonnen')
    expect(factionLabel(4, UNALIGNED)).toBe('Smuggler')
  })

  it('id 3 ("None" faction) is distinct from unaligned (id 0)', () => {
    expect(factionLabel(3, UNALIGNED)).toBe('None')
    expect(factionLabel(3, UNALIGNED)).not.toBe(UNALIGNED)
  })

  it('falls back to the raw numeric string for unknown ids', () => {
    expect(factionLabel(99, UNALIGNED)).toBe('99')
  })
})

describe('sortValue', () => {
  const p = makePlayer({ name: 'Zed', class: 'Fremen', map: 'DeepDesert', faction_id: 2, id: 42 })

  it('returns the raw string for name/class/map', () => {
    expect(sortValue(p, 'name', UNALIGNED)).toBe('Zed')
    expect(sortValue(p, 'class', UNALIGNED)).toBe('Fremen')
    expect(sortValue(p, 'map', UNALIGNED)).toBe('DeepDesert')
  })

  it('returns the faction display label, not the raw id', () => {
    expect(sortValue(p, 'faction_id', UNALIGNED)).toBe('Harkonnen')
  })

  it('returns the numeric id for the id axis', () => {
    expect(sortValue(p, 'id', UNALIGNED)).toBe(42)
  })
})

describe('comparePlayers', () => {
  it('sorts by name ascending, numeric/case-insensitive', () => {
    const a = makePlayer({ id: 1, name: 'Bob10' })
    const b = makePlayer({ id: 2, name: 'bob2' })
    expect(comparePlayers(a, b, 'name', 'asc', UNALIGNED) > 0).toBe(true)
    expect(comparePlayers(b, a, 'name', 'asc', UNALIGNED) < 0).toBe(true)
  })

  it('reverses order for desc', () => {
    const a = makePlayer({ id: 1, name: 'Alice' })
    const b = makePlayer({ id: 2, name: 'Bob' })
    expect(comparePlayers(a, b, 'name', 'asc', UNALIGNED) < 0).toBe(true)
    expect(comparePlayers(a, b, 'name', 'desc', UNALIGNED) > 0).toBe(true)
  })

  it('sorts the id axis numerically, not lexicographically', () => {
    const a = makePlayer({ id: 2, name: 'Z' })
    const b = makePlayer({ id: 10, name: 'A' })
    // Lexicographic string sort would put "10" before "2"; numeric must not.
    expect(comparePlayers(a, b, 'id', 'asc', UNALIGNED) < 0).toBe(true)
  })

  it('faction axis groups by faction, tie-broken by name within the group', () => {
    const atreides1 = makePlayer({ id: 1, name: 'Zed', faction_id: 1 })
    const atreides2 = makePlayer({ id: 2, name: 'Amy', faction_id: 1 })
    const harkonnen = makePlayer({ id: 3, name: 'Aaron', faction_id: 2 })
    const sorted = [atreides1, harkonnen, atreides2].sort(
      (x, y) => comparePlayers(x, y, 'faction_id', 'asc', UNALIGNED),
    )
    // Atreides < Harkonnen alphabetically; within Atreides, Amy < Zed.
    expect(sorted.map((p) => p.name)).toEqual(['Amy', 'Zed', 'Aaron'])
  })

  it('name axis has no secondary tie-break beyond itself', () => {
    const a = makePlayer({ id: 1, name: 'Same' })
    const b = makePlayer({ id: 2, name: 'Same' })
    expect(comparePlayers(a, b, 'name', 'asc', UNALIGNED)).toBe(0)
  })
})
