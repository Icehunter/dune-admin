import type { GivePack, GivePackItem } from '../../../api/client'

export type SkippedItem = { template: string, reason: string }
export type GiveResult = { given: string[], skipped: SkippedItem[] } | null
export type StagedItem = { template: string, qty: number, quality: number, _key: string }

export type LocationKey = 'name' | 'x' | 'y' | 'z' | 'actions'

export type ItemKey = 'template' | 'stack' | 'quality' | 'durability' | 'actions'
export type VehicleKey = 'class' | 'location' | 'chassis' | 'name' | 'type' | 'actions'

export type PackDiff = { added: number, updated: number, removed: number, isDirty: boolean }

export type KeyedItem = GivePackItem & { _key: string }
export type KeyedPack = Omit<GivePack, 'items'> & { items: KeyedItem[] }
