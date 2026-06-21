import * as React from 'react'
import type { AppConfig, DiscordGuild } from '../../../api/client'
import type { DiscordRole } from '../../types'

export interface GlobalSettingsFormProps {
  saveRef?: React.MutableRefObject<(() => Promise<void>) | null>
  onSavingChange?: (saving: boolean) => void
  /** Initial tab to open on (e.g. deep-link to 'discord'); still switchable. */
  initialTab?: string | undefined
}

export interface AdminAdvancedPanelProps {
  cfg: AppConfig
  set: (key: keyof AppConfig) => (v: string) => void
  backendUrl: string
  setBackendUrl: (v: string) => void
}

export interface AuthPanelProps {
  cfg: AppConfig
  set: (key: keyof AppConfig) => (v: string) => void
  setBool: (key: keyof AppConfig) => (v: boolean) => void
  discordRoles: DiscordRole[]
  rolesLoading: boolean
  loadDiscordRoles: () => void
}

export interface DiscordPanelProps {
  cfg: AppConfig
  set: (key: keyof AppConfig) => (v: string) => void
  setBool: (key: keyof AppConfig) => (v: boolean) => void
}

export interface GuildEditModalProps {
  open: boolean
  /** When editing, the existing guild; null when adding a new one. */
  existing: DiscordGuild | null
  /** Guild ids already configured — hidden from the add dropdown and rejected on
   *  save so the same guild can't be configured twice. */
  takenGuildIds?: string[]
  onClose: () => void
  /** Called after a successful upsert so the parent can reload its list. */
  onSaved: () => void
}
