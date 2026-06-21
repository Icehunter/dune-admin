export type FieldKind = { kind: 'bool' } | { kind: 'number' } | { kind: 'enum', options: string[] } | { kind: 'text' }

export type BasesTabKey = 'id' | 'name' | 'pieces' | 'placeables' | 'actions'

export type BlueprintsTabKey = 'id' | 'owner_name' | 'name' | 'item_id' | 'pieces' | 'placeables' | 'actions'

export type TaskKey = 'board_index' | 'house' | 'goal_amount' | 'completed' | 'sysselraad'

export type GuildsTabKey = 'name' | 'faction' | 'members' | 'description' | 'actions'

export type ActiveView = 'pod' | 'cheats'
export type NavKey = 'cheats' | `pod:${string}`

export type CheatKey = 'time' | 'character' | 'cheat_type'
