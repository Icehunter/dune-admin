/**
 * Unified data store — fetches static JSON files from the Go backend first
 * (enabling local overrides), falls back to the CDN when Go returns 404 or
 * when running on a pure CDN deploy with no backend reachable.
 *
 * All atoms use jotai's implicit default store so cache persists until a hard
 * refresh. Components use useAtom(loadable(atomRef)); imperative callers use
 * getDefaultStore().get(atomRef) via the helper functions below.
 */

import { atom, getDefaultStore } from 'jotai'
import { unwrap } from 'jotai/utils'
import { useAtomValue } from 'jotai'
import { useMemo } from 'react'
import type { Atom } from 'jotai'
import { apiBase, isCdnDeploy } from '../api/client'

// ── CDN base ──────────────────────────────────────────────────────────────────

/** Returns the CDN base URL, stripped of trailing slashes. */
export const cdnBase = (): string =>
  ((import.meta.env.VITE_CDN_BASE_URL as string) ?? 'https://assets.dune.layout.tools').replace(/\/$/, '')

// ── Fetch primitive ───────────────────────────────────────────────────────────

/**
 * Fetches a named data file. On non-CDN deploys the Go backend is tried first
 * (allows local overrides); a non-ok response or network error causes
 * transparent fallback to the CDN. Throws only when both sources fail.
 */
async function fetchDataFile<T>(filename: string): Promise<T> {
  if (!isCdnDeploy) {
    try {
      const res = await fetch(`${apiBase}/data/${filename}`)
      if (res.ok) return res.json() as Promise<T>
    }
    catch {
      // fall through to CDN
    }
  }
  const res = await fetch(`${cdnBase()}/${filename}`)
  if (!res.ok) throw new Error(`Failed to load ${filename}`)
  return res.json() as Promise<T>
}

// ── Types ─────────────────────────────────────────────────────────────────────

export type ItemEntry = {
  name?: string
  category?: string
  tier?: number
  rarity?: string
  is_gradeable?: boolean
  armor_value?: number
  mitigation?: Record<string, number>
}

export type ItemDataFile = {
  default_stack_max?: number
  default_volume?: number
  names?: Record<string, string>
  items: Record<string, ItemEntry>
}

export type QualityData = {
  weapon_damage: number[]
  armor: number[]
  volume: number[]
}

export type SkillModule = {
  id: string
  label: string
}

export type Vehicle = {
  id: string
  label: string
  actor_class: string
  templates: string[]
}

export type CheatScript = {
  name: string
  danger: boolean
}

export type PacksData = {
  packs: Record<string, {
    name: string
    category: string
    tier: number
    items: { template: string, qty: number, quality: number }[]
  }>
}

// ── Atoms ─────────────────────────────────────────────────────────────────────

export const itemDataAtom = atom<Promise<ItemDataFile>>(async () => {
  try {
    return await fetchDataFile<ItemDataFile>('item-data.json')
  }
  catch {
    return { items: {} }
  }
})

export const tagsDataAtom = atom<Promise<unknown>>(async () => {
  try {
    return await fetchDataFile<unknown>('tags-data.json')
  }
  catch {
    return {}
  }
})

export const qualityDataAtom = atom<Promise<QualityData>>(async () => {
  try {
    return await fetchDataFile<QualityData>('quality-data.json')
  }
  catch {
    return { weapon_damage: [], armor: [], volume: [] }
  }
})

export const gameplayTagsAtom = atom<Promise<string[]>>(async () => {
  try {
    return await fetchDataFile<string[]>('gameplayTags.json')
  }
  catch {
    return []
  }
})

export const skillModulesAtom = atom<Promise<SkillModule[]>>(async () => {
  try {
    return await fetchDataFile<SkillModule[]>('skillModules.json')
  }
  catch {
    return []
  }
})

export const vehiclesAtom = atom<Promise<Vehicle[]>>(async () => {
  try {
    return await fetchDataFile<Vehicle[]>('vehicles.json')
  }
  catch {
    return []
  }
})

export const cheatScriptsAtom = atom<Promise<CheatScript[]>>(async () => {
  try {
    return await fetchDataFile<CheatScript[]>('cheatScripts.json')
  }
  catch {
    return []
  }
})

export const packsAtom = atom<Promise<PacksData>>(async () => {
  try {
    return await fetchDataFile<PacksData>('packs.json')
  }
  catch {
    return { packs: {} }
  }
})

// ── Sync (unwrapped) atoms — safe to use without Suspense ─────────────────────

export const gameplayTagsSyncAtom = unwrap(gameplayTagsAtom, (): string[] => [])
export const skillModulesSyncAtom = unwrap(skillModulesAtom, (): SkillModule[] => [])
export const vehiclesSyncAtom = unwrap(vehiclesAtom, (): Vehicle[] => [])
export const cheatScriptsSyncAtom = unwrap(cheatScriptsAtom, (): CheatScript[] => [])
export const packsSyncAtom = unwrap(packsAtom, (): PacksData => ({ packs: {} }))

// ── React hook ────────────────────────────────────────────────────────────────

/** Wraps a data atom with unwrap so components can read it without Suspense. */
export function useDataFile<T>(fileAtom: Atom<Promise<T>>): {
  data: T | null
  loading: boolean
  error: string | null
} {
  const syncAtom = useMemo(() => unwrap(fileAtom, (): T | null => null), [fileAtom])
  const data = useAtomValue(syncAtom)
  return { data, loading: data === null, error: null }
}

// ── Imperative accessors (non-React call sites) ───────────────────────────────

/** Returns a Promise resolving to the full item data. Shares the jotai cache. */
export function getItemData(): Promise<ItemDataFile> {
  return getDefaultStore().get(itemDataAtom)
}

/** Returns the ItemEntry for the given template ID, or null if not found. */
export async function getItemEntry(templateId: string): Promise<ItemEntry | null> {
  const data = await getItemData()
  return data.items[templateId] ?? null
}
