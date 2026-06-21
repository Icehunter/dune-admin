import { atomWithStorage } from 'jotai/utils'
import type { Bounds, CalibPoint } from './types'
import { TYPE_MERGE_KEY, IMG_W, IMG_H, HEATMAP_TO_FILTER } from './constants'

const MAP_BASE = ((import.meta.env.VITE_CDN_BASE_URL as string) ?? 'https://assets.dune.layout.tools').replace(/\/$/, '')

export const mapUrl = (path: string): string => {
  return `${MAP_BASE}/${path}`
}

export const filterKey = (type: string): string => {
  return TYPE_MERGE_KEY[type] ?? type
}

export const heatmapFilterKey = (type: string): string => {
  return HEATMAP_TO_FILTER[type] ?? type
}

export const clamp01 = (v: number): number => {
  if (v < 0) return 0
  if (v > 1) return 1
  return v
}

export const worldToLatLng = (x: number, y: number, cfg: Bounds): [number, number] => {
  const normX = (x - cfg.minX) / (cfg.maxX - cfg.minX)
  const normY = (y - cfg.minY) / (cfg.maxY - cfg.minY)
  const fracX = clamp01(cfg.flipX ? 1 - normX : normX)
  const fracYup = clamp01(cfg.flipY ? 1 - normY : normY)
  return [fracYup * IMG_H, fracX * IMG_W]
}

export const latLngToWorld = (lat: number, lng: number, cfg: Bounds): { x: number, y: number } => {
  const fracX = lng / IMG_W
  const fracYup = lat / IMG_H
  const rawX = cfg.flipX ? 1 - fracX : fracX
  const rawY = cfg.flipY ? 1 - fracYup : fracYup
  return {
    x: rawX * (cfg.maxX - cfg.minX) + cfg.minX,
    y: rawY * (cfg.maxY - cfg.minY) + cfg.minY,
  }
}

// solveBounds fits a linear world→fraction transform to all calibration points
// using ordinary least-squares per axis. Requires ≥2 distinct points.
export const solveBounds = (pts: CalibPoint[]): Bounds | null => {
  if (pts.length < 2) return null

  // OLS fit: frac = slope * world + intercept
  const fitAxis = (worlds: number[], fracs: number[]): { slope: number, intercept: number } | null => {
    const n = worlds.length
    const sumW = worlds.reduce((a, b) => a + b, 0)
    const sumF = fracs.reduce((a, b) => a + b, 0)
    const sumWW = worlds.reduce((a, b, i) => a + b * worlds[i], 0)
    const sumWF = worlds.reduce((a, b, i) => a + b * fracs[i], 0)
    const denom = n * sumWW - sumW * sumW
    if (Math.abs(denom) < 1e-12) return null
    const slope = (n * sumWF - sumW * sumF) / denom
    const intercept = (sumF - slope * sumW) / n
    return { slope, intercept }
  }

  const fitX = fitAxis(pts.map((p) => p.wx), pts.map((p) => p.fracX))
  const fitY = fitAxis(pts.map((p) => p.wy), pts.map((p) => p.fracYup))
  if (!fitX || !fitY) return null

  const { slope: sX, intercept: iX } = fitX
  const { slope: sY, intercept: iY } = fitY
  if (Math.abs(sX) < 1e-12 || Math.abs(sY) < 1e-12) return null

  const flipY = sY < 0
  const minX = -iX / sX
  const maxX = (1 - iX) / sX
  const R = flipY ? -1 / sY : 1 / sY
  const minY = flipY ? (iY - 1) * R : -iY * R
  return { minX, maxX, minY, maxY: minY + R, flipY }
}

const LIVE_FILTER_DEFAULTS: Record<string, boolean> = {
  players: true, vehicles: true, bases: true,
}
const FILTER_LS_KEY = 'dune_admin_livemap_filter'

export const liveFilterAtom = atomWithStorage<Record<string, boolean>>(
  FILTER_LS_KEY,
  LIVE_FILTER_DEFAULTS,
)
