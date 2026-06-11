import type { Player, GivePack } from '../../../api/client'

export interface GiveItemsViewProps {
  player: Player
}

export type ItemKey = 'template' | 'stack' | 'quality' | 'durability' | 'actions'

export interface InventoryViewProps {
  player: Player
}

export type SkippedItem = { template: string, reason: string }
export type GiveResult = { given: string[], skipped: SkippedItem[] } | null
export type StagedItem = { template: string, qty: number, quality: number, _key: string }

export type { GivePack }

export type VehicleKey = 'class' | 'location' | 'chassis' | 'name' | 'type' | 'actions'

export interface VehiclesViewProps {
  player: Player
}
