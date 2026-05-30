import type { Column } from '../../dune-ui'

export type ServerSortKey = 'map' | 'phase' | 'players' | 'ready' | 'dimension' | 'partition'

export type ServerRow = {
  map: string
  sietch: string
  dimension: number
  partition: number
  phase: string
  ready: boolean
  players: number
}

export type BGInfo = {
  name: string
  title: string
  phase: string
  database: string
}

export type DetailedStatus = {
  battlegroup: BGInfo
  servers: ServerRow[]
}

export type ActionDef = {
  label: string
  cmd: string
  danger: boolean
  msg: string
}

export const SERVER_COLUMNS: Column<ServerSortKey>[] = [
  { key: 'map', label: 'Map', isRowHeader: true },
  { key: 'phase', label: 'Phase' },
  { key: 'players', label: 'Players' },
  { key: 'ready', label: 'Ready' },
  { key: 'dimension', label: 'Dim' },
  { key: 'partition', label: 'Part' },
]

export const ACTIONS: ActionDef[] = [
  { label: 'Start', cmd: 'start', danger: false, msg: 'Start the battlegroup server?' },
  { label: 'Stop', cmd: 'stop', danger: true, msg: 'Stop the server? All players will be disconnected.' },
  { label: 'Restart', cmd: 'restart', danger: false, msg: 'Restart the server? Players will be briefly disconnected.' },
  { label: 'Update', cmd: 'update', danger: false, msg: 'Run a server update? This takes the server offline briefly.' },
  { label: 'Backup', cmd: 'backup', danger: false, msg: 'Create a database backup? This may take a few minutes.' },
]

export const INIT_WARN_MS = 3 * 60 * 1000
