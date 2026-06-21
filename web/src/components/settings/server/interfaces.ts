import * as React from 'react'
import type { AppConfig } from '../../../api/client'
import type { ServerAdvancedVariant } from './types'

export interface SshPanelProps {
  cfg: AppConfig
  set: (key: keyof AppConfig) => (v: string) => void
}

export interface ServerDiscordPanelProps {
  /** The persisted server (id > 0) whose single Discord link is edited here. */
  serverId: number
}

export interface ConnectionPanelProps {
  cfg: AppConfig
  set: (key: keyof AppConfig) => (v: string) => void
  setBool: (key: keyof AppConfig) => (v: boolean) => void
  /** Show the Database panel. */
  showDb: boolean
  /** Show the RabbitMQ broker panel. */
  showBroker: boolean
}

export interface ServerSettingsFormProps {
  saveRef?: React.MutableRefObject<(() => Promise<void>) | null>
  onSavingChange?: (saving: boolean) => void
  /**
   * Manage-server page: target this server id for load/save instead of the
   * active server. Also enables the inline Name (rename) field.
   */
  serverId?: number
  /**
   * Add-server wizard: persist creates a NEW per-server entry via POST /servers
   * (not the flat config). Only per-server fields are sent.
   */
  addMode?: boolean
  /** Add-server wizard: the name entered for the new server. */
  addServerName?: string
  /**
   * Add-server wizard: called whenever the live form config changes so the
   * wizard can read current values (control plane + SSH) to drive discovery.
   */
  onConfigChange?: (cfg: AppConfig) => void
  /** Add-server wizard: discovered values to merge into the form config. */
  prefill?: Partial<AppConfig> | null
  /** When set, overrides the internal tab state (wizard mode). */
  activeTab?: string
  /** When true, hides the Segment tab bar (wizard drives navigation). */
  hideTabBar?: boolean
  /**
   * Settings-modal only: invoked from the per-server Advanced "Danger Zone" to
   * request deletion of the active server. When omitted, the Danger Zone is hidden.
   */
  onRequestDeleteServer?: (() => void) | undefined
  /** Initial tab to open on; still switchable. */
  initialTab?: string | undefined
}

export interface ControlPanelProps {
  cfg: AppConfig
  set: (key: keyof AppConfig) => (v: string) => void
  setBool: (key: keyof AppConfig) => (v: boolean) => void
  setControl: (v: string) => void
}

export interface ServerAdvancedPanelProps {
  variant: ServerAdvancedVariant
  cfg: AppConfig
  set: (key: keyof AppConfig) => (v: string) => void
  setBool: (key: keyof AppConfig) => (v: boolean) => void
  backendUrl: string
  setBackendUrl: (v: string) => void
  activeName: string
  onRequestDeleteServer?: (() => void) | undefined
}

export interface MarketBotPanelProps {
  cfg: AppConfig
  setBool: (key: keyof AppConfig) => (v: boolean) => void
}

export interface PathsPanelProps {
  cfg: AppConfig
  set: (key: keyof AppConfig) => (v: string) => void
}
