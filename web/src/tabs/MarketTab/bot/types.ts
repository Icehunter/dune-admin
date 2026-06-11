import * as React from 'react'
import type { AppConfig, BotStatus, BotConfig } from '../../../api/client'

export interface ConfigEditorHandle {
  save: () => Promise<void>
  reset: () => void
  setEnabled: (v: boolean) => void
}

export type BotActionsProps = {
  status: BotStatus
  onRefresh: () => void
}

export type BusyOp = 'start' | 'stop' | 'restart' | 'cleanup'

export type BotLogViewerProps = {
  active?: boolean
}

export type ConnState = 'idle' | 'connecting' | 'connected' | 'error'

export type BotControlPanelProps = {
  open: boolean
  onClose: () => void
}

export interface ConfigFooterProps {
  editorRef: React.RefObject<ConfigEditorHandle | null>
  initialEnabled: boolean
  onReload: () => void
}

export type StringAppConfigKey = { [K in keyof AppConfig]: AppConfig[K] extends string ? K : never }[keyof AppConfig]

export interface BotServerConfigHandle {
  save: () => Promise<void>
}

export interface ServerConfigFooterProps {
  configRef: React.RefObject<BotServerConfigHandle | null>
}

export interface BotStatusCardProps {
  status: BotStatus
}

export interface StatProps {
  label: string
  first?: boolean
  danger?: boolean
  children: React.ReactNode
}

export type BotConfigEditorProps = {
  config: BotConfig
  onSaved: (cfg: BotConfig) => void
}

export interface FieldProps {
  label: string
  hint?: string
  children: React.ReactNode
}

export type DisabledItemsManagerProps = {
  config: BotConfig
  onSaved: (cfg: BotConfig) => void
}

export type DisabledRow = { template_id: string, display_name: string }
export type RowKey = 'name' | 'template_id' | 'actions'
