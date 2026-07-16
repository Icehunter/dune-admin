import type { InventoryItem, Player, TeleportLocation, GivePack } from '../../../api/client'
import type { PackDiff } from './types'

export interface GiveItemsModalProps {
  player: Player
  open: boolean
  onClose: () => void
}

export interface EditItemModalProps {
  item: InventoryItem | null
  onClose: () => void
  onSaved: (item: InventoryItem) => void
}

export interface ManageLocationsModalProps {
  onClose: (updated?: TeleportLocation[]) => void
}

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

export interface DiffStatusProps {
  diff: PackDiff
}

export interface ClickCapturerProps {
  onPick: (x: number, y: number, z: number) => void
}

export interface MapCoordPickerModalProps {
  onPick: (x: number, y: number, z: number) => void
  onClose: () => void
}
