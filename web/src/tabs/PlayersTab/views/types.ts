import type { GivePack } from '../../../api/client'

export type ItemKey = 'template' | 'stack' | 'quality' | 'durability' | 'actions'

export type SkippedItem = { template: string, reason: string }
export type GiveResult = { given: string[], skipped: SkippedItem[] } | null
export type StagedItem = { template: string, qty: number, quality: number, _key: string }

export type { GivePack }

export type VehicleKey = 'class' | 'location' | 'chassis' | 'name' | 'type' | 'actions'
