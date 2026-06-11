import type { Player } from '../../../../../api/client'

export interface ContractsSectionProps { player: Player }

export interface HistorySectionProps { player: Player }

export type ChipColor = 'default' | 'accent' | 'success' | 'warning' | 'danger'

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

export const TRAINERS = ['BeneGesserit', 'Mentat', 'Planetologist', 'Swordmaster', 'Trooper'] as const
export type TrainerKey = typeof TRAINERS[number]

export type FilterTab = 'all' | 'done' | 'revealed' | 'reward'

export interface JourneySectionProps { player: Player }
