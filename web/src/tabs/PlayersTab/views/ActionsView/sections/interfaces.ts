import type { CharacterBackup, Player } from '../../../../../api/client'

export interface DeleteCharacterModalProps {
  open: boolean
  playerName: string
  online: boolean
  busy: boolean
  onCancel: () => void
  onConfirm: (reason: string, backup: boolean) => void
}

export interface ContractsSectionProps { player: Player }

export interface CharacterBackupsPanelProps { player: Player }

export interface CharacterRestoreModalProps {
  /** The backup to restore; `null` closes the modal. */
  backup: CharacterBackup | null
  playerName: string
  busy: boolean
  onCancel: () => void
  onConfirm: (backup: CharacterBackup) => void
}

export interface HistorySectionProps { player: Player }

export interface ExperimentalSectionProps { player: Player }

export interface SpecsSectionProps { player: Player }

export interface ResourcesSectionProps { player: Player }

export interface TagsSectionProps { player: Player }

export interface AdminSectionProps {
  player: Player
  onManageLocations: () => void
  onTeleportPicker: (cb: (x: number, y: number, z: number) => void) => void
  onSpawnPicker: (cb: (x: number, y: number, z: number) => void) => void
}

export interface ProgressionSectionProps { player: Player }

export interface JourneySectionProps { player: Player }
