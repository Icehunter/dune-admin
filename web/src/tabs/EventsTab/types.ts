import type { EventDefinition } from '../../api/client'

export type EventsTabSection = 'list' | 'status'
export type MilestoneSignal = 'level' | 'achievement_tag'
export type XPType = 'character' | 'specialization'
export const XP_TRACKS = ['Combat', 'Crafting', 'Gathering', 'Exploration', 'Sabotage'] as const

export interface ZoneRaceFields {
  map: string
  x: number
  y: number
  z: number
  radius: number
  participants: number[]
}

export interface MilestoneFields {
  signal: MilestoneSignal
  threshold: number
  tagName: string
  awardPast: boolean
}

export interface RewardItem {
  template: string
  qty: number
  quality: number
}

export interface RewardXP {
  track: string
  amount: number
}

export interface RewardFields {
  currency: number
  items: RewardItem[]
  xpRewards: RewardXP[]
}

export type ListKey = 'name' | 'type' | 'enabled' | 'version' | 'actions'
export type ClaimKey = 'account_id' | 'version' | 'status' | 'attempts' | 'claimed_at' | 'last_error'

// Kept for any code that still references EventEditorValues
export interface EventEditorValues {
  name: string
  type: EventDefinition['type']
  config: string
  reward: string
  announce_channel_id: string
  announce_template: string
}
