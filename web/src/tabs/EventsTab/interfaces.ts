import type { EventDefinition } from '../../api/client'
import type { MilestoneSignal } from './types'

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

export interface KeyedRewardItem extends RewardItem {
  _key: string
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

export interface EventEditorValues {
  name: string
  type: EventDefinition['type']
  config: string
  reward: string
  announce_channel_id: string
  announce_template: string
}
