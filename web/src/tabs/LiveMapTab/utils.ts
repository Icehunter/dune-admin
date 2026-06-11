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

export const solveBounds = (pts: CalibPoint[]): Bounds | null => {
  if (pts.length < 2) return null
  const a = pts[0]
  const b = pts[pts.length - 1]
  if (b.wx === a.wx || b.wy === a.wy || b.fracX === a.fracX || b.fracYup === a.fracYup) return null
  const sX = (b.fracX - a.fracX) / (b.wx - a.wx)
  const iX = a.fracX - sX * a.wx
  const sY = (b.fracYup - a.fracYup) / (b.wy - a.wy)
  const iY = a.fracYup - sY * a.wy
  const flipY = sY < 0
  const minX = -iX / sX
  const maxX = (1 - iX) / sX
  const R = flipY ? -1 / sY : 1 / sY
  const minY = flipY ? (iY - 1) * R : -iY * R
  return { minX, maxX, minY, maxY: minY + R, flipY }
}

const CALIB_LS_KEY = 'dune_admin_livemap_calib'

export const loadCalib = (): Record<string, Bounds> => {
  try {
    return JSON.parse(localStorage.getItem(CALIB_LS_KEY) ?? '{}') as Record<string, Bounds>
  }
  catch {
    return {}
  }
}

const LIVE_FILTER_DEFAULTS: Record<string, boolean> = {
  players: true, vehicles: true, bases: true,
}
const FILTER_LS_KEY = 'dune_admin_livemap_filter'

export const loadFilter = (): Record<string, boolean> => {
  try {
    const saved = JSON.parse(localStorage.getItem(FILTER_LS_KEY) ?? '{}') as Record<string, boolean>
    return { ...LIVE_FILTER_DEFAULTS, ...saved }
  }
  catch {
    return LIVE_FILTER_DEFAULTS
  }
}

export const saveFilter = (f: Record<string, boolean>): void => {
  try {
    localStorage.setItem(FILTER_LS_KEY, JSON.stringify(f))
  }
  catch { /* quota */ }
}
