import * as React from 'react'
import type { Player, SessionRecord, StatSnapshot } from '../../../api/client'

export interface PlayerCardProps {
  player: Player
  selected: boolean
  onSelect: (player: Player) => void
}

export interface PlayerDetailPanelProps {
  player: Player
}

export interface StatRowProps {
  label: string
  value: string | number
}

export interface SessionChartProps {
  data: SessionRecord[]
}

export interface DiscordBadgeProps {
  /** Discord user ID — renders nothing when falsy */
  discordUserId?: string
  size?: number
  /** SVG fill color. Defaults to Discord blurple. */
  color?: string
}

export interface XPChartProps {
  data: StatSnapshot[]
}

export interface StatProps { label: string, children: React.ReactNode }

export interface SolarisChartProps {
  data: StatSnapshot[]
}

export interface SolarisPoint {
  time: string
  balance: number
  cum_earned: number
  cum_spent: number
  [key: string]: string | number
}

export interface StatusDotProps {
  status: string
}
