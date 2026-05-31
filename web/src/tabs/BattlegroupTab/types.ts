import type { TFunction } from 'i18next'
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

export function getServerColumns(t: TFunction): Column<ServerSortKey>[] {
  return [
    { key: 'map', label: t('battlegroup.columns.map'), isRowHeader: true },
    { key: 'phase', label: t('battlegroup.columns.phase') },
    { key: 'players', label: t('battlegroup.columns.players') },
    { key: 'ready', label: t('battlegroup.columns.ready') },
    { key: 'dimension', label: t('battlegroup.columns.dim') },
    { key: 'partition', label: t('battlegroup.columns.part') },
  ]
}

export const ACTIONS: ActionDef[] = [
  { label: 'start', cmd: 'start', danger: false, msg: 'startMsg' },
  { label: 'stop', cmd: 'stop', danger: true, msg: 'stopMsg' },
  { label: 'restart', cmd: 'restart', danger: false, msg: 'restartMsg' },
  { label: 'update', cmd: 'update', danger: false, msg: 'updateMsg' },
  { label: 'backup', cmd: 'backup', danger: false, msg: 'backupMsg' },
]

export const INIT_WARN_MS = 3 * 60 * 1000
