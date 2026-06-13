import { describe, expect, it } from 'vitest'
import { groupPacksByCategory } from './CategorizedPackPicker'
import type { PackOption } from './CategorizedPackPicker'

const mk = (id: string, category: string, tier: number, name = id): PackOption => ({
  id, name, category, tier,
})

describe('groupPacksByCategory', () => {
  it('returns an empty array for no packs', () => {
    expect(groupPacksByCategory([])).toEqual([])
  })

  it('groups packs by category', () => {
    const result = groupPacksByCategory([
      mk('a', 'weapons', 1),
      mk('b', 'armor', 1),
      mk('c', 'weapons', 2),
    ])
    const cats = result.map(([cat]) => cat)
    expect(cats).toEqual(['armor', 'weapons'])
    const weapons = result.find(([cat]) => cat === 'weapons')![1]
    expect(weapons.map((p) => p.id)).toEqual(['a', 'c'])
  })

  it('sorts categories by localeCompare', () => {
    const result = groupPacksByCategory([
      mk('a', 'zeta', 1),
      mk('b', 'alpha', 1),
      mk('c', 'mid', 1),
    ])
    expect(result.map(([cat]) => cat)).toEqual(['alpha', 'mid', 'zeta'])
  })

  it('sorts packs within a category by tier ascending', () => {
    const result = groupPacksByCategory([
      mk('a', 'cat', 3),
      mk('b', 'cat', 1),
      mk('c', 'cat', 2),
    ])
    expect(result[0][1].map((p) => p.id)).toEqual(['b', 'c', 'a'])
  })

  it('preserves all pack fields', () => {
    const result = groupPacksByCategory([mk('x', 'cat', 5, 'Cool Pack')])
    expect(result[0][1][0]).toEqual({ id: 'x', name: 'Cool Pack', category: 'cat', tier: 5 })
  })
})
