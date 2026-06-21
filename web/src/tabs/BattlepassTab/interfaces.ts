import type { BattlepassTier, BattlepassTierCounts } from '../../api/client'

export interface CardArtProps {
  folder: string
  file: string
}

export interface ThemeIconProps {
  folder: string
  name: string
  className?: string
}

export interface TrackViewProps {
  tiers: BattlepassTier[]
  counts: Record<string, BattlepassTierCounts>
  playerCount: number
  categoryLabel: (cat: string) => string
}
