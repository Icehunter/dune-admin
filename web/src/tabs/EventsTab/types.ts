export type EventsTabSection = 'list' | 'status'
export type MilestoneSignal = 'level' | 'achievement_tag'
export type XPType = 'character' | 'specialization'
export const XP_TRACKS = ['Combat', 'Crafting', 'Gathering', 'Exploration', 'Sabotage'] as const

export type ListKey = 'name' | 'type' | 'enabled' | 'version' | 'actions'
export type ClaimKey = 'account_id' | 'version' | 'status' | 'attempts' | 'claimed_at' | 'last_error' | 'next_attempt_at' | 'actions'
