import * as React from 'react'
import type { BotStatus } from '../../../api/client'

export interface ConfigEditorHandle {
  save: () => Promise<void>
  reset: () => void
  setEnabled: (v: boolean) => void
}

export interface ConfigFooterProps {
  editorRef: React.RefObject<ConfigEditorHandle | null>
  initialEnabled: boolean
  onReload: () => void
}

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

export interface FieldProps {
  label: string
  hint?: string
  children: React.ReactNode
}
