import type { AppConfig, BotStatus, BotConfig } from '../../../api/client'

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

export type StringAppConfigKey = { [K in keyof AppConfig]-?: AppConfig[K] extends string ? K : never }[keyof AppConfig]

export type BotConfigEditorProps = {
  config: BotConfig
  onSaved: (cfg: BotConfig) => void
}

export type DisabledItemsManagerProps = {
  config: BotConfig
  onSaved: (cfg: BotConfig) => void
}

export type DisabledRow = { template_id: string, display_name: string }
export type RowKey = 'name' | 'template_id' | 'actions'
