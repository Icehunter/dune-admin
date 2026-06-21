import type { AppConfig, Player, ServerConfig } from '../api/client'

export interface ItemDetailCardRowProps {
  label: string
  value: string
  accent?: boolean
  wrap?: boolean
}

export interface MitigationBarProps {
  label: string
  value: number
}

export interface PlayerSearchFieldProps {
  /** Called with the full player row — consumers pick the ID they need
   *  (account_id, fls_id, or actor id). */
  onSelect: (player: Player) => void
  ariaLabel: string
  placeholder?: string
  className?: string
  /** Pre-loaded player list. When omitted the field lazily loads
   *  api.players.list() on first focus. */
  players?: Player[]
  /** Max suggestions rendered (default 10) — keeps the dropdown cheap even
   *  with thousands of players. */
  resultLimit?: number
  /** Exclude players from suggestions (e.g. the current player). */
  filter?: (player: Player) => boolean
  /** Clear the input after picking (default: show the picked name). */
  clearOnSelect?: boolean
  /** Called when the user empties the input (clear button or deleting the
   *  text) — lets consumers drop their current selection. */
  onClear?: () => void
}

/** Minimal shape needed to render a pack in the categorized picker. The full
 *  GivePack (with items) and the PacksData entry are both structurally
 *  assignable to this. */
export interface PackOption {
  id: string
  name: string
  category: string
  tier: number
}

export interface CategorizedPackPickerProps {
  packs: PackOption[]
  /** Called with the selected pack id. */
  onSelectPack: (id: string) => void
  className?: string
}

export interface ManageServerModalProps {
  open: boolean
  serverId: number
  /** Whether the session may delete/control servers. */
  canControl: boolean
  onClose: () => void
  /** Called after the server is deleted (so the app can refresh status). */
  onDeleted?: () => void
}

export interface DiscoveryModalProps {
  open: boolean
  /** Connection settings to probe (control plane + SSH). */
  config: ServerConfig
  /** Called with the discovered values once all steps complete. */
  onDone: (discovered: Partial<AppConfig>) => void
  /** Called if the user dismisses before completion. */
  onSkip: () => void
}

export interface DeleteServerModalProps {
  open: boolean
  serverName: string
  busy: boolean
  onCancel: () => void
  onConfirm: () => void
}

export interface SetupWizardProps {
  /** Called after a successful "add server" — return to the main app. */
  onDone?: () => void
}
