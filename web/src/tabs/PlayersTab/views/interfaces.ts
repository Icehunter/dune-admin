import type { Player } from '../../../api/client'
import type { ItemEntry } from '../../../data/store'

export interface GiveItemsViewProps {
  player: Player
}

export interface InventoryViewProps {
  player: Player
}

export interface VehiclesViewProps {
  player: Player
}

export interface StagedItemCellProps {
  templateId: string
  name: string
  itemData: { items: Record<string, ItemEntry> }
}
