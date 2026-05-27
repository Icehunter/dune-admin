declare global {
  interface Window {
    Clerk?: { session?: { getToken(): Promise<string | null> } }
  }
}

function getApiBase(): string {
  const stored = localStorage.getItem('dune_admin_backend')
  if (stored) return stored.replace(/\/$/, '') + '/api/v1'
  // When the SPA is served from a Go-binary deploy (any host other than the
  // Vite dev server's localhost), the backend is the same origin. Only the
  // dev-time localhost case needs the :8080 default.
  if (typeof window !== 'undefined' &&
      window.location.hostname !== 'localhost' &&
      window.location.hostname !== '127.0.0.1') {
    return window.location.origin + '/api/v1'
  }
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

export type SettingLayer = {
  source: string
  value: string
}

export type ServerSetting = {
  section: string
  key: string
  type: 'float' | 'int' | 'bool' | 'string'
  default: string
  label: string
  description: string
  category: string
  current: string
  is_overridden: boolean
  source: 'userGame' | 'userEngine' | 'defaultGame' | 'defaultEngine' | ''
  layers: SettingLayer[]
}

export type ServerSettingUpdate = {
  section: string
  key: string
  value: string
}

export type RawLine = {
  prefix: string  // '', '+', or '-'
  key: string
  value: string
}

export type RawSection = {
  section: string
  source: 'userGame' | 'userEngine' | 'defaultGame' | 'defaultEngine'
  lines: RawLine[]
}

export type ServerSettingsResponse = {
  settings: ServerSetting[]
  raw: RawSection[]
}

export type AppConfig = {
  control: string
  ssh_host: string
  ssh_user: string
  ssh_key: string
  db_host: string
  db_port: number
  db_user: string
  db_pass: string
  db_name: string
  db_schema: string
  control_namespace: string
  docker_gameserver: string
  docker_broker_game: string
  docker_broker_admin: string
  cmd_start: string
  cmd_stop: string
  cmd_restart: string
  cmd_status: string
  broker_game_addr: string
  broker_admin_addr: string
  broker_tls: boolean
  broker_exec_prefix: string
  backup_dir: string
  listen_addr: string
  scrip_currency: number
}

export type Status = {
  executor: string       // "ssh" | "local" | "none"
  control: string        // "kubectl" | "docker" | "local" | "none"
  ssh_connected: boolean
  db_connected: boolean
  ssh_host: string
  db_host: string
  pod_ns: string
  pod_ip: string
  version?: string
  commit?: string
  build_time?: string
}
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

export type MarketItem = {
  template_id: string
  quality: number
  display_name: string
  category: string
  tier: number
  rarity: string
  lowest_price: number
  total_stock: number
  bot_stock: number
  listing_count: number
  icon: string | null
}
export type MarketListing = {
  order_id: number
  template_id: string
  owner_type: 'bot' | 'player'
  owner_name: string
  price: number
  stock: number
  quality: number
}
export type MarketSale = {
  order_id: number
  template_id: string
  seller_type: 'bot' | 'player'
  seller_name: string
  price: number
  quantity: number
}
export type MarketStats = {
  total_listings: number
  bot_listings: number
  player_listings: number
  total_stock: number
  bot_stock: number
  player_stock: number
  unique_items: number
}
export type MarketItemsParams = {
  search?: string
  category?: string
  tier?: number
  rarity?: string
  owner?: 'bot' | 'player'
  page?: number
  limit?: number
}
export type MarketItemsResponse = {
  items: MarketItem[]
  total: number
  page: number
  limit: number
}
export type CatalogItem = {
  template_id: string
  display_name: string
}
export type BotStatus = {
  running: boolean      // injected by dune-admin proxy (true = bot responded)
  uptime: string
  last_list_tick: string | null
  last_buy_tick: string | null
  next_list_tick?: string | null
  next_buy_tick?: string | null
  listing_count: number
  balance?: number
  error_count: number
  error?: string        // set when running=false
}
export type BotConfig = {
  list_tick_interval: string
  buy_tick_interval: string
  rarity_multipliers: Record<string, number>
  vendor_multipliers?: Record<string, number>
  grade_multipliers: number[]
  buy_threshold: number
  max_buys_per_tick: number
  listings_per_grade: number
  disabled_items: string[]
  enabled: boolean
}

export type ProgressionPreset = {
  id: string
  name: string
  description: string
  node_count: number
  nodes: string[]
}

export const api = {
  status: () => req<Status>('GET', '/status'),
  reconnect: () => req<Status>('POST', '/reconnect'),
  progression: {
    presets: () => req<ProgressionPreset[]>('GET', '/progression/presets'),
    applyPreset: (account_id: number, preset_id: string) =>
      req<MutateResult>('POST', '/players/progression/apply-preset', { account_id, preset_id }),
  },
  config: {
    get: () => req<AppConfig>('GET', '/config'),
    save: (cfg: AppConfig) => req<Status>('POST', '/config', cfg),
  },
  serverSettings: {
    get: () => req<ServerSettingsResponse>('GET', '/server-settings'),
    update: (updates: ServerSettingUpdate[]) =>
      req<{ ok: string; applied: number; cleared: number }>('PUT', '/server-settings', { updates }),
    updateRaw: (section: string, lines: string) =>
      req<{ ok: string }>('PUT', '/server-settings/raw', { section, lines }),
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
    awardXP: (player_id: number, track_type: string, delta: number, fls_id?: string) =>
      req<MutateResult>('POST', '/players/award-xp', { player_id, track_type, delta, fls_id }),
    awardCharXP: (player_id: number, amount: number, fls_id?: string) =>
      req<MutateResult>('POST', '/players/award-char-xp', { player_id, amount, fls_id }),
    awardIntel: (player_id: number, amount: number) =>
      req<MutateResult>('POST', '/players/award-intel', { player_id, amount }),
    rename: (account_id: number, name: string) => req<MutateResult>('POST', '/players/rename', { account_id, name }),
    tags: (account_id: number) => req<string[]>('GET', `/players/${account_id}/tags`),
    updateTags: (account_id: number, add: string[], remove: string[]) => req<MutateResult>('POST', '/players/update-tags', { account_id, add, remove }),
    returningPlayerAward: (account_id: number) => req<MutateResult>('POST', '/players/returning-player-award', { account_id }),
    dismissReturningPlayerAward: (account_id: number) => req<MutateResult>('POST', '/players/dismiss-returning-player-award', { account_id }),
    exportUrl: (account_id: number) => `${BASE}/players/${account_id}/export`,
    exportPlayer: async (account_id: number): Promise<void> => {
      const token = await window.Clerk?.session?.getToken()
      const headers: Record<string, string> = {}
      if (token) headers['Authorization'] = `Bearer ${token}`
      const res = await fetch(`${BASE}/players/${account_id}/export`, { headers })
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText }))
        throw new ApiError(res.status, err.error ?? res.statusText)
      }
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `player_${account_id}_export.json`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    },
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
    resetAllKeystones: (player_id: number) =>
      req<MutateResult>('POST', '/players/reset-all-keystones', { player_id }),
    reverseContracts: (account_id: number, contract_ids: string[]) =>
      req<MutateResult>('POST', '/players/contracts/reverse', { account_id, contract_ids }),
    progressionReverse: (player_id: number, faction: string, preset: string) =>
      req<MutateResult>('POST', '/players/progression-reverse', { player_id, faction, preset }),
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
    kick: (fls_id: string) =>
      req<MutateResult>('POST', '/players/kick', { fls_id }),
    fillWater: (fls_id: string, water_amount = 1000000) =>
      req<MutateResult>('POST', '/players/fill-water', { fls_id, water_amount }),
    setSkillPoints: (fls_id: string, skill_points: number) =>
      req<MutateResult>('POST', '/players/set-skill-points', { fls_id, skill_points }),
    cheatScript: (fls_id: string, script_name: string) =>
      req<MutateResult>('POST', '/players/cheat-script', { fls_id, script_name }),
    cleanInventory: (fls_id: string) =>
      req<MutateResult>('POST', '/players/clean-inventory', { fls_id }),
    resetProgression: (fls_id: string) =>
      req<MutateResult>('POST', '/players/reset-progression', { fls_id }),
    setSkillModule: (fls_id: string, module: string, level: number) =>
      req<MutateResult>('POST', '/players/set-skill-module', { fls_id, module, level }),
    spawnVehicle: (fls_id: string, class_name: string, x: number, y: number, z: number, options?: { rotation?: number; template_name?: string; persistent?: boolean; faction?: string }) =>
      req<MutateResult>('POST', '/vehicles/spawn', { fls_id, class_name, x, y, z, ...options }),
  },

  broadcast: {
    send: (texts: { Key: string; Title: string; Body: string }[], duration_sec = 30) =>
      req<MutateResult>('POST', '/broadcast', { texts, duration_sec }),
    shutdown: (shutdown_type: string, delay_minutes: number, cancel = false) =>
      req<MutateResult>('POST', '/broadcast/shutdown', { shutdown_type, delay_minutes, cancel }),
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
    list: () => req<{id: number; name: string; class: string; map: string; item_count: number; item_templates: string[]; item_names: string[]; owner_name: string}[]>('GET', '/storage'),
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

  market: {
    items: (params?: MarketItemsParams) => {
      const q = new URLSearchParams()
      if (params?.search)   q.set('search', params.search)
      if (params?.category) q.set('category', params.category)
      if (params?.tier != null) q.set('tier', String(params.tier))
      if (params?.rarity)   q.set('rarity', params.rarity)
      if (params?.owner)    q.set('owner', params.owner)
      if (params?.page != null) q.set('page', String(params.page))
      if (params?.limit != null) q.set('limit', String(params.limit))
      const qs = q.toString()
      return req<MarketItemsResponse>('GET', `/market/items${qs ? '?' + qs : ''}`)
    },
    listings: (templateId?: string, owner?: 'bot' | 'player') => {
      const q = new URLSearchParams()
      if (templateId) q.set('template_id', templateId)
      if (owner) q.set('owner', owner)
      const qs = q.toString()
      return req<MarketListing[]>('GET', `/market/listings${qs ? '?' + qs : ''}`)
    },
    sales: () => req<MarketSale[]>('GET', '/market/sales'),
    stats: () => req<MarketStats>('GET', '/market/stats'),
    categories: () => req<string[]>('GET', '/market/categories'),
    catalog: () => req<CatalogItem[]>('GET', '/market/catalog'),
  },

  marketBot: {
    status: () => req<BotStatus>('GET', '/market-bot/status'),
    config: () => req<BotConfig>('GET', '/market-bot/config'),
    saveConfig: (cfg: BotConfig) => req<BotConfig>('PUT', '/market-bot/config', cfg),
    lifecycle: (cmd: 'start' | 'stop' | 'restart') => req<{ output: string }>('POST', '/market-bot/exec', { cmd }),
    logsReady: () => req<{ ready: boolean; reason?: string; namespace?: string; name?: string }>('GET', '/market-bot/logs-ready'),
  },
}
