import type { TabId } from '../../types'
import type { Status } from '../../api/client'
import type { NavGroup } from './nav'
import type { UpdatePhase } from '../UpdateProgressModal'

export interface AppSidebarProps {
  visibleNavGroups: NavGroup[]
  pathname: string
  navigate: (path: string) => void
}

export interface AppRoutesProps {
  currentTab: TabId
  status: Status | null
  isSignedIn: boolean
  canSeeTab: (key: TabId) => boolean
  onOpenSettings: (tab?: string) => void
}

export interface AppNavbarProps {
  status: Status | null
  reconnecting: boolean
  onReconnect: () => void
  can: (cap: string) => boolean
  onOpenSettings: (tab?: string) => void
}

export interface SettingsModalProps {
  status: Status | null
  can: (cap: string) => boolean
  onClose: () => void
}

export interface UpdatePromptModalProps {
  can: (cap: string) => boolean
}

export interface AddServerModalProps {
  onDone: () => void
}

export interface UpdateProgressModalProps {
  isOpen: boolean
  phase: UpdatePhase
  errorMessage?: string | undefined
  onDismiss?: () => void
}
