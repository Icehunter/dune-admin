import type { Player, TeleportLocation, GivePack, GivePackItem } from '../../../api/client'
import type { ItemEntry } from '../../../data/store'

export interface GiveItemsModalProps {
  player: Player
  open: boolean
  onClose: () => void
}

export type SkippedItem = { template: string, reason: string }
export type GiveResult = { given: string[], skipped: SkippedItem[] } | null
export type StagedItem = { template: string, qty: number, quality: number, _key: string }

export interface ManageLocationsModalProps {
  onClose: (updated?: TeleportLocation[]) => void
}

export type LocationKey = 'name' | 'x' | 'y' | 'z' | 'actions'

export type ItemKey = 'template' | 'stack' | 'quality' | 'durability' | 'actions'
export type VehicleKey = 'class' | 'location' | 'chassis' | 'name' | 'type' | 'actions'

export interface InventoryModalProps {
  player: Player
  open: boolean
  onClose: () => void
}

export interface ManagePacksModalProps {
  isOpen: boolean
  onClose: () => void
  onSaved: (packs: GivePack[]) => void
  templates: { id: string, name: string }[]
}

export type PackDiff = { added: number, updated: number, removed: number, isDirty: boolean }

export interface DiffStatusProps {
  diff: PackDiff
}

export type KeyedItem = GivePackItem & { _key: string }
export type KeyedPack = Omit<GivePack, 'items'> & { items: KeyedItem[] }

export interface ClickCapturerProps {
  onPick: (x: number, y: number, z: number) => void
}

export interface MapCoordPickerModalProps {
  onPick: (x: number, y: number, z: number) => void
  onClose: () => void
}

export interface ModalStagedItemCellProps {
  templateId: string
  name: string
  itemData: { items: Record<string, ItemEntry> }
}
