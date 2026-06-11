export type FieldKind = { kind: 'bool' } | { kind: 'number' } | { kind: 'enum', options: string[] } | { kind: 'text' }

export type BasesTabKey = 'id' | 'name' | 'pieces' | 'placeables' | 'actions'

export interface BasesTabProps {
  isSignedIn?: boolean
}

export type BlueprintsTabKey = 'id' | 'owner_name' | 'name' | 'item_id' | 'pieces' | 'placeables' | 'actions'

export interface BlueprintsTabProps {
  isSignedIn?: boolean
}

export interface ImportModalProps {
  open: boolean
  onClose: () => void
  onSuccess: () => void
}

export type TaskKey = 'board_index' | 'house' | 'goal_amount' | 'completed' | 'sysselraad'

export interface FieldProps {
  label: string
  value: string
}

export type GuildsTabKey = 'name' | 'faction' | 'members' | 'description' | 'actions'

export interface GuildsTabProps {
  isSignedIn?: boolean
}

export type ActiveView = 'pod' | 'cheats'
export type NavKey = 'cheats' | `pod:${string}`

export type CheatKey = 'time' | 'character' | 'cheat_type'

export interface LogsTabProps {
  control?: string
}
