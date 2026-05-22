declare global {
  interface Window {
    Clerk?: { session?: { getToken(): Promise<string | null> } }
  }
}

function getApiBase(): string {
  const stored = localStorage.getItem('dune_admin_backend')
  if (stored) return stored.replace(/\/$/, '') + '/api/v1'
  return 'http://localhost:8080/api/v1'
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

export type Status = { ssh_connected: boolean; db_connected: boolean; pod_ns: string; pod_ip: string; ssh_host: string }
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
export type BGOutput = { output: string }
export type VehicleRow = { id: number; class: string; map: string; chassis_durability: number; vehicle_name: string; is_recovered: boolean; is_backup: boolean }
export type CheatEntry = { fls_id: string; cheat_type: string; event_time: string; character_name: string }
export type GameEvent = { actor_id: number; universe_time: string; map: string; event_type: number; x: number; y: number; z: number; custom_data: string }
export type DungeonRecord = { dungeon_id: string; difficulty: string; duration_ms: number; players_num: number; completion_id: number }
export type TeleportLocation = { name: string; x: number; y: number; z: number }
export type OnlineRow = { player_id: number; name: string; map: string; status: string; last_seen: string }

export const api = {
  status: () => req<Status>('GET', '/status'),
  reconnect: () => req<Status>('POST', '/reconnect'),

  battlegroup: {
    status: () => req<BGOutput>('GET', '/battlegroup/status'),
    exec: (cmd: string) => req<BGOutput>('POST', '/battlegroup/exec', { cmd }),
    pods: () => req<{ pods: string[]; namespace: string }>('GET', '/battlegroup/pods'),
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
    kick: (player_id: number) => req<MutateResult>('POST', '/players/kick', { player_id }),
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
    vehicles: (account_id: number) => req<VehicleRow[]>('GET', `/players/${account_id}/vehicles`),
    repairItem: (id: number) => req<MutateResult>('POST', '/players/repair-item', { id }),
    partitions: () => req<TeleportLocation[]>('GET', '/players/partitions'),
    teleport: (fls_id: string, partition_label: string) =>
      req<MutateResult>('POST', '/players/teleport', { fls_id, partition_label }),
    events: (id: number) => req<GameEvent[]>('GET', `/players/${id}/events`),
    dungeons: (id: number) => req<DungeonRecord[]>('GET', `/players/${id}/dungeons`),
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
    list: () => req<{id: number; class: string; map: string; item_count: number}[]>('GET', '/storage'),
    items: (id: number) => req<InventoryItem[]>('GET', `/storage/${id}/items`),
    giveItem: (id: number, template: string, qty: number, quality: number) =>
      req<MutateResult>('POST', `/storage/${id}/give-item`, { template, qty, quality }),
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
