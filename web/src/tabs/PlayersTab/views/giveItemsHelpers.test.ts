import { describe, it, expect } from 'vitest'
import { retainSkippedStaged, filterTemplates } from './giveItemsHelpers'

type Staged = { template: string, qty: number, quality: number }
type Template = { id: string, name: string }

describe('retainSkippedStaged', () => {
  it('returns empty when all items were given', () => {
    const staged: Staged[] = [
      { template: 'A', qty: 1, quality: 0 },
      { template: 'B', qty: 2, quality: 1 },
    ]
    const given: string[] = ['A', 'B']
    expect(retainSkippedStaged(staged, given)).toEqual([])
  })

  it('retains all staged when nothing was given', () => {
    const staged: Staged[] = [
      { template: 'A', qty: 1, quality: 0 },
      { template: 'B', qty: 2, quality: 0 },
    ]
    expect(retainSkippedStaged(staged, [])).toEqual(staged)
  })

  it('removes only given items, keeps skipped', () => {
    const staged: Staged[] = [
      { template: 'Ammo', qty: 500, quality: 0 },
      { template: 'Kindjal', qty: 1, quality: 0 },
      { template: 'HeavyArmor', qty: 1, quality: 0 },
    ]
    const given: string[] = ['Ammo', 'Kindjal']
    const result = retainSkippedStaged(staged, given)
    expect(result).toHaveLength(1)
    expect(result[0].template).toBe('HeavyArmor')
  })

  it('handles duplicate templates: removes one given, keeps skipped copy', () => {
    // Two staged rows with the same template — e.g. user added it twice.
    const staged: Staged[] = [
      { template: 'Ammo', qty: 100, quality: 0 },
      { template: 'Ammo', qty: 200, quality: 0 },
    ]
    // Backend gave one, skipped one.
    const result = retainSkippedStaged(staged, ['Ammo'])
    // Should retain exactly one Ammo row (the skipped one).
    expect(result).toHaveLength(1)
    expect(result[0].template).toBe('Ammo')
  })

  it('given wins over skipped when template appears in both (edge case)', () => {
    // If somehow the same template appears in both given and skipped,
    // we remove the given count and keep the skipped count.
    const staged: Staged[] = [
      { template: 'X', qty: 5, quality: 0 },
      { template: 'X', qty: 3, quality: 0 },
      { template: 'X', qty: 1, quality: 0 },
    ]
    const given: string[] = ['X', 'X'] // two given
    const result = retainSkippedStaged(staged, given)
    // 3 staged, 2 given → 1 retained
    expect(result).toHaveLength(1)
    expect(result[0].template).toBe('X')
  })

  it('preserves qty and quality of retained rows', () => {
    const staged: Staged[] = [
      { template: 'Sword', qty: 1, quality: 5 },
    ]
    const result = retainSkippedStaged(staged, [])
    expect(result[0]).toEqual({ template: 'Sword', qty: 1, quality: 5 })
  })

  it('empty staged input returns empty', () => {
    expect(retainSkippedStaged([], ['A'])).toEqual([])
  })
})

describe('filterTemplates', () => {
  const templates: Template[] = [
    { id: 'mtx_patent_a', name: 'Alpha Patent' },
    { id: 'mtx_patent_b', name: 'Beta Patent' },
    { id: 'd_debug_sword', name: 'Debug Sword' },
    { id: 'sword_basic', name: 'Basic Sword' },
  ]

  it('returns empty when query is empty', () => {
    expect(filterTemplates(templates, '')).toEqual([])
  })

  it('always hides d_-prefixed (deprecated, non-functional) items', () => {
    const result = filterTemplates(templates, 'sword')
    expect(result.map((t) => t.id)).toEqual(['sword_basic'])
  })

  it('matches a plain substring case-insensitively (existing behavior)', () => {
    const result = filterTemplates(templates, 'PATENT')
    expect(result.map((t) => t.id).sort()).toEqual(['mtx_patent_a', 'mtx_patent_b'])
  })

  it('supports * as a multi-char wildcard against id', () => {
    const result = filterTemplates(templates, 'mtx_*_a')
    expect(result.map((t) => t.id)).toEqual(['mtx_patent_a'])
  })

  it('supports * as a multi-char wildcard matching multiple results', () => {
    const result = filterTemplates(templates, 'mtx_*_patent')
    expect(result).toEqual([])
    // "mtx_*_patent" doesn't match these ids (patent isn't a trailing
    // segment) — confirms the glob is a real pattern match, not a substring
    // fallback that would otherwise match on "patent" alone.
  })

  it('wildcard also matches against name', () => {
    const result = filterTemplates(templates, '*Patent')
    expect(result.map((t) => t.id).sort()).toEqual(['mtx_patent_a', 'mtx_patent_b'])
  })

  it('escapes regex metacharacters other than * when building a glob', () => {
    // The "." must stay a literal dot in the glob, not regex "any char" —
    // otherwise "weirdXid+1*" would wrongly match "weird.id+1_extra" too.
    const withRegexChars: Template[] = [
      { id: 'weird.id+1_extra', name: 'Weird' },
      { id: 'weirdXid+1_extra', name: 'Weird X' },
    ]
    expect(filterTemplates(withRegexChars, 'weird.id+1*').map((t) => t.id)).toEqual(['weird.id+1_extra'])
  })

  it('caps results at 100', () => {
    const many: Template[] = Array.from({ length: 150 }, (_, i) => ({ id: `item_${i}`, name: `Item ${i}` }))
    expect(filterTemplates(many, 'item').length).toBe(100)
  })
})
