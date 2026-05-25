declare global {
  interface Window {
    Clerk?: { session?: { getToken(): Promise<string | null> } }
  }
}

function getApiBase(): string {
  const stored = localStorage.getItem('dune_admin_backend')
  if (stored) return stored.replace(/\/$/, '') + '/api/v1'
  return window.location.origin + '/api/v1'
}

export function getWsBase(): string {
  return getApiBase().replace(/^http/, 'ws')
}

const BASE = getApiBase()

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const token = await window.Clerk?.session?.getToken()
  const headers: Record<string, string> = {}
  if (body) headers['Content-Type'] = 'application/json'
  if (token) headers['Authorization'] = `Bearer ${token}`
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new ApiError(res.status, err.error ?? res.statusText)
  }
  return res.json()
}

export type Status = { ssh_connected: boolean; db_connected: boolean; connection_mode: string; pod_ns: string; pod_ip: string; ssh_host: string; version?: string }
export type Player = { id: number; account_id: number; controller_id: number; fls_id: string; name: string; class: string; map: string; faction_id: number; online_status: string }
export type InventoryItem = { id: number; template_id: string; name: string; stack_size: number; quality: number; durability: string; max_durability: string }
export type CurrencyRow = { player_id: number; currency_id: number; balance: number }
export type FactionRep = { actor_id: number; faction_id: number; faction_name: string; reputation: number; scrips: number }
export type SpecTrack = { player_id: number; track_type: string; xp: number; level: number }
export type KeystoneRow = { id: number; track: string; name: string; level: number; cost: number }
export type JourneyNode = { node_id: string; is_complete: boolean; is_revealed: boolean; has_pending_reward: boolean }
export type BlueprintRow = { id: number; owner_name: string; item_id: number; pieces: number; placeables: number; name?: string }
export type BaseRow = { id: number; name: string; pieces: number; placeables: number }
export type LogPod = { namespace: string; name: string }
export type MutateResult = { ok: string }
export type BulkGiveResult = { given: string[]; skipped: { template: string; reason: string }[] }
export type BGOutput = { output: string }
export type VehicleRow = { id: number; class: string; map: string; chassis_durability: number; vehicle_name: string; is_recovered: boolean; is_backup: boolean }
export type CheatEntry = { fls_id: string; cheat_type: string; event_time: string; character_name: string }
export type GameEvent = { actor_id: number; universe_time: string; map: string; event_type: number; x: number; y: number; z: number; custom_data: string }
export type DungeonRecord = { dungeon_id: string; difficulty: string; duration_ms: number; players_num: number; completion_id: number }
export type TeleportLocation = { name: string; x: number; y: number; z: number }
export type OnlineRow = { player_id: number; name: string; map: string; status: string; last_seen: string }
export type BackupFile = { name: string; size_bytes: number; modified: string; has_yaml: boolean }
export type ProgressionPreset = { id: string; name: string; description: string; node_count: number; nodes: string[] }
export type ServerSetting = {
  section: string
  key: string
  type: 'float' | 'int' | 'bool'
  default: number | boolean | string
  label: string
  description: string
  category: string
  current: string
  is_overridden: boolean
  source: 'userOverrides' | 'userGame' | ''
}
export type ServerSettingUpdate = { section: string; key: string; value: string }

export const api = {
  status: () => req<Status>('GET', '/status'),
  reconnect: () => req<Status>('POST', '/reconnect'),

  progression: {
    presets: () => req<ProgressionPreset[]>('GET', '/progression/presets'),
    applyPreset: (account_id: number, preset_id: string) =>
      req<MutateResult>('POST', '/players/progression/apply-preset', { account_id, preset_id }),
  },

  serverSettings: {
    get: () => req<ServerSetting[]>('GET', '/server-settings'),
    update: (updates: ServerSettingUpdate[]) =>
      req<{ ok: string; applied: number; cleared: number }>('PUT', '/server-settings', { updates }),
  },

  battlegroup: {
    status: () => req<unknown>('GET', '/battlegroup/status'),
    exec: (cmd: string) => req<BGOutput>('POST', '/battlegroup/exec', { cmd }),
    pods: () => req<{ pods: string[]; namespace: string }>('GET', '/battlegroup/pods'),
    backupFiles: () => req<BackupFile[]>('GET', '/battlegroup/backup-files'),
    backupDownloadUrl: (file: string) => `${BASE}/battlegroup/backup-files/download?file=${encodeURIComponent(file)}`,
    backupUpload: async (file: File): Promise<{ name: string }> => {
      const form = new FormData()
      form.append('backup', file)
      const token = await window.Clerk?.session?.getToken()
      const headers: Record<string, string> = {}
      if (token) headers['Authorization'] = `Bearer ${token}`
      const res = await fetch(`${BASE}/battlegroup/backup-files/upload`, { method: 'POST', headers, body: form })
      if (!res.ok) { const e = await res.json().catch(() => ({ error: res.statusText })); throw new ApiError(res.status, e.error) }
      return res.json()
    },
    restore: (file: string) => req<BGOutput>('POST', '/battlegroup/restore', { file }),
  },

  players: {
    list: () => req<Player[]>('GET', '/players'),
    online: () => req<OnlineRow[]>('GET', '/players/online'),
    currency: () => req<CurrencyRow[]>('GET', '/players/currency'),
    factions: () => req<FactionRep[]>('GET', '/players/factions'),
    specs: () => req<SpecTrack[]>('GET', '/players/specs'),
    templates: () => req<{id: string; name: string}[]>('GET', '/players/templates'),
    inventory: (id: number) => req<InventoryItem[]>('GET', `/players/${id}/inventory`),
    journey: (accountId: number) => req<JourneyNode[]>('GET', `/players/${accountId}/journey`),
    giveItem: (player_id: number, template: string, qty: number, quality: number) =>
      req<MutateResult>('POST', '/players/give-item', { player_id, template, qty, quality }),
    giveItems: (player_id: number, items: { template: string; qty: number; quality: number }[]) =>
      req<BulkGiveResult>('POST', '/players/give-items', { player_id, items }),
    grantLive: (controller_id: number, template: string, amount: number) =>
      req<MutateResult>('POST', '/players/grant-live', { controller_id, template, amount }),
    giveCurrency: (player_id: number, amount: number) =>
      req<MutateResult>('POST', '/players/give-currency', { player_id, amount }),
    giveFactionRep: (actor_id: number, faction_id: number, delta: number) =>
      req<MutateResult>('POST', '/players/give-faction-rep', { actor_id, faction_id, delta }),
    giveScrip: (actor_id: number, delta: number) =>
      req<MutateResult>('POST', '/players/give-scrip', { actor_id, delta }),
    awardXP: (player_id: number, track_type: string, delta: number) =>
      req<MutateResult>('POST', '/players/award-xp', { player_id, track_type, delta }),
    awardCharXP: (player_id: number, amount: number) =>
      req<MutateResult>('POST', '/players/award-char-xp', { player_id, amount }),
    awardIntel: (player_id: number, amount: number) =>
      req<MutateResult>('POST', '/players/award-intel', { player_id, amount }),
    rename: (account_id: number, name: string) => req<MutateResult>('POST', '/players/rename', { account_id, name }),
    tags: (account_id: number) => req<string[]>('GET', `/players/${account_id}/tags`),
    updateTags: (account_id: number, add: string[], remove: string[]) => req<MutateResult>('POST', '/players/update-tags', { account_id, add, remove }),
    returningPlayerAward: (account_id: number) => req<MutateResult>('POST', '/players/returning-player-award', { account_id }),
    dismissReturningPlayerAward: (account_id: number) => req<MutateResult>('POST', '/players/dismiss-returning-player-award', { account_id }),
    exportUrl: (account_id: number) => `${BASE}/players/${account_id}/export`,
    deleteAccount: (account_id: number, reason: string) => req<MutateResult>('POST', '/players/delete-account', { account_id, reason }),
    deleteItem: (id: number) => req<MutateResult>('DELETE', `/players/item/${id}`),
    resetSpec: (player_id: number, track_type: string) =>
      req<MutateResult>('POST', '/players/reset-spec', { player_id, track_type }),
    setFactionTier: (actor_id: number, faction_id: number, tier: number) =>
      req<MutateResult>('POST', '/players/set-faction-tier', { actor_id, faction_id, tier }),
    progressionUnlock: (player_id: number, faction: string, preset: string) =>
      req<MutateResult>('POST', '/players/progression-unlock', { player_id, faction, preset }),
    journeyComplete: (account_id: number, node_id: string) =>
      req<MutateResult>('POST', '/players/journey/complete', { account_id, node_id }),
    journeyReset: (account_id: number, node_id: string) =>
      req<MutateResult>('POST', '/players/journey/reset', { account_id, node_id }),
    journeyWipe: (account_id: number) =>
      req<MutateResult>('POST', '/players/journey/wipe', { account_id }),
    completeContract: (account_id: number, contract_id: string) =>
      req<MutateResult>('POST', '/players/contract/complete', { account_id, contract_id }),
    completeContracts: (account_id: number, contract_ids: string[]) =>
      req<MutateResult>('POST', '/players/contracts/complete', { account_id, contract_ids }),
    grantJobSkills: (account_id: number, job: string) =>
      req<MutateResult>('POST', '/players/grant-job-skills', { account_id, job }),
    resetJobSkills: (account_id: number, job: string) =>
      req<MutateResult>('POST', '/players/reset-job-skills', { account_id, job }),
    setStarterClass: (account_id: number, job: string) =>
      req<MutateResult>('POST', '/players/set-starter-class', { account_id, job }),
    deleteTutorials: (player_id: number) =>
      req<MutateResult>('POST', '/players/delete-tutorials', { player_id }),
    wipeCodex: (account_id: number) =>
      req<MutateResult>('POST', '/players/wipe-codex', { account_id }),
    charXPCurrent: (id: number) => req<{xp: number; level: number}>('GET', `/players/${id}/char-xp`),
    specs_for: (id: number) => req<SpecTrack[]>('GET', `/players/${id}/specs`),
    keystones: (id: number) => req<KeystoneRow[]>('GET', `/players/${id}/keystones`),
    grantMaxSpec: (player_id: number, track_type: string) =>
      req<MutateResult>('POST', '/players/grant-max-spec', { player_id, track_type }),
    grantAllKeystones: (player_id: number) =>
      req<MutateResult>('POST', '/players/grant-all-keystones', { player_id }),
    vehicles: (controller_id: number) => req<VehicleRow[]>('GET', `/players/${controller_id}/vehicles`),
    repairItem: (id: number) => req<MutateResult>('POST', '/players/repair-item', { id }),
    repairGear: (player_id: number) =>
      req<{repaired: number; scanned: number}>('POST', '/players/repair-gear', { player_id }),
    repairVehicle: (vehicle_id: number, player_id: number) =>
      req<{repaired: number; skipped: number; total: number}>('POST', '/players/repair-vehicle', { vehicle_id, player_id }),
    refuelVehicle: (vehicle_id: number, player_id: number) =>
      req<MutateResult>('POST', '/players/refuel-vehicle', { vehicle_id, player_id }),
    partitions: () => req<TeleportLocation[]>('GET', '/players/partitions'),
    teleport: (fls_id: string, partition_label: string) =>
      req<MutateResult>('POST', '/players/teleport', { fls_id, partition_label }),
    events: (id: number) => req<GameEvent[]>('GET', `/players/${id}/events`),
    dungeons: (id: number) => req<DungeonRecord[]>('GET', `/players/${id}/dungeons`),
  },

  contracts: {
    list: () => req<{id: string; alias: string; tag_count: number}[]>('GET', '/contracts'),
  },

  database: {
    tables: () => req<{name: string; row_count: number}[]>('GET', '/database/tables'),
    describe: (table: string) => req<{table: string; columns: {name: string; data_type: string; nullable: string}[]}>('GET', `/database/describe?table=${encodeURIComponent(table)}`),
    sample: (table: string, limit = 20) => req<{table: string; headers: string[]; rows: string[][]}>('GET', `/database/sample?table=${encodeURIComponent(table)}&limit=${limit}`),
    search: (term: string) => req<{headers: string[]; rows: string[][]}>('GET', `/database/search?term=${encodeURIComponent(term)}`),
    sql: (sql: string) => req<{result: string}>('POST', '/database/sql', { sql }),
  },

  logs: {
    pods: () => req<LogPod[]>('GET', '/logs/pods'),
    cheats: () => req<CheatEntry[]>('GET', '/logs/cheats'),
  },

  storage: {
    list: () => req<{id: number; name: string; class: string; map: string; item_count: number}[]>('GET', '/storage'),
    items: (id: number) => req<InventoryItem[]>('GET', `/storage/${id}/items`),
    giveItem: (id: number, template: string, qty: number, quality: number) =>
      req<MutateResult>('POST', `/storage/${id}/give-item`, { template, qty, quality }),
    giveItems: (id: number, items: { template: string; qty: number; quality: number }[]) =>
      req<BulkGiveResult>('POST', `/storage/${id}/give-items`, { items }),
  },

  blueprints: {
    list: () => req<BlueprintRow[]>('GET', '/blueprints'),
    exportUrl: (id: number) => `${BASE}/blueprints/${id}/export`,
    import: async (file: File, player_id: number) => {
      const token = await window.Clerk?.session?.getToken()
      const headers: Record<string, string> = {}
      if (token) headers['Authorization'] = `Bearer ${token}`
      const fd = new FormData()
      fd.append('file', file)
      fd.append('player_id', String(player_id))
      return fetch(`${BASE}/blueprints/import`, { method: 'POST', headers, body: fd })
        .then(r => r.json())
    },
  },

  bases: {
    list: () => req<BaseRow[]>('GET', '/bases'),
    exportUrl: (id: number) => `${BASE}/bases/${id}/export`,
  },
}
