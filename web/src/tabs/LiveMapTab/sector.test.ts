import { describe, expect, it } from 'vitest'
import { MAPS } from './constants'
import type { Bounds } from './types'
import { ddRowLabel, deepDesertSector, worldToLatLng } from './utils'

// Deep Desert sector mapping (#310, #213).
//
// Ground truth, captured from a live server: the character "Narisa" was standing
// in sector A6 in game, and dune.actors recorded the pawn at
// world x = 92450, y = 1077150 on the DeepDesert map.
//
// That is near maxY, i.e. sector A is at HIGH world Y. The code assigned 'A' to
// the LOWEST world Y and did not flip the Y axis for this map (unlike HaggaBasin
// and Arrakeen, which both set flipY), so the marker was drawn at the top of the
// map and fell in the cell labelled I6 — the exact complaint in both issues, with
// the column correct and the row mirrored.
const NARISA = { x: 92450, y: 1077150, sector: 'A6' }

const ddCfg = (): Bounds => {
  const m = MAPS.find((mm) => mm.key === 'DeepDesert')
  if (!m) throw new Error('DeepDesert map config missing')
  return m
}

describe('deep desert sectors', () => {
  it('places the known A6 character in A6', () => {
    expect(deepDesertSector(NARISA.x, NARISA.y, ddCfg())).toBe(NARISA.sector)
  })

  it('draws the A row at the bottom of the screen, where it already appeared', () => {
    const cfg = ddCfg()
    // Row index 0 is the lowest world Y band; row index 8 the highest.
    // Whatever the flip, the band that renders lowest on screen must read 'A'.
    const cellH = (cfg.maxY - cfg.minY) / 9
    const latOf = (ri: number): number =>
      worldToLatLng(cfg.minX, cfg.minY + (ri + 0.5) * cellH, cfg)[0]

    const lats = [...Array(9).keys()].map(latOf)
    // In Leaflet CRS.Simple latitude increases upward, so the smallest lat is
    // the bottom of the screen.
    const bottomRi = lats.indexOf(Math.min(...lats))
    const topRi = lats.indexOf(Math.max(...lats))
    expect(ddRowLabel(bottomRi)).toBe('A')
    expect(ddRowLabel(topRi)).toBe('I')
  })

  it('puts the A6 character in the bottom half of the screen', () => {
    const cfg = ddCfg()
    const [lat] = worldToLatLng(NARISA.x, NARISA.y, cfg)
    const [latMid] = worldToLatLng(cfg.minX, (cfg.minY + cfg.maxY) / 2, cfg)
    expect(lat).toBeLessThan(latMid)
  })

  it('keeps the column unaffected — both issues reported the column was correct', () => {
    expect(deepDesertSector(NARISA.x, NARISA.y, ddCfg()).slice(1)).toBe('6')
  })

  it('maps the corners to the expected sectors', () => {
    const cfg = ddCfg()
    const inset = (cfg.maxX - cfg.minX) * 0.01
    // Highest world Y is row A; lowest is row I. Lowest world X is column 1.
    expect(deepDesertSector(cfg.minX + inset, cfg.maxY - inset, cfg)).toBe('A1')
    expect(deepDesertSector(cfg.maxX - inset, cfg.maxY - inset, cfg)).toBe('A9')
    expect(deepDesertSector(cfg.minX + inset, cfg.minY + inset, cfg)).toBe('I1')
    expect(deepDesertSector(cfg.maxX - inset, cfg.minY + inset, cfg)).toBe('I9')
  })

  it('clamps points outside the configured bounds instead of returning a bogus sector', () => {
    const cfg = ddCfg()
    expect(deepDesertSector(cfg.minX - 5_000_000, cfg.minY - 5_000_000, cfg)).toBe('I1')
    expect(deepDesertSector(cfg.maxX + 5_000_000, cfg.maxY + 5_000_000, cfg)).toBe('A9')
  })
})
