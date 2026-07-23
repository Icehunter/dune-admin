declare global {
  interface Window {
    Clerk?: { session?: { getToken(): Promise<string | null> } }
  }
}

function sanitizeBackendBase(raw: string): string | null {
  try {
    const u = new URL(raw.trim())
    if (u.protocol !== 'http:' && u.protocol !== 'https:') return null
    return `${u.origin}${u.pathname}`.replace(/\/$/, '')
  }
  catch {
    return null
  }
}

// isCdnDeploy is true on Cloudflare Pages builds (VITE_CDN_BASE_URL set at
// build time). On CDN deploys the SPA and the Go backend are on different
// origins, so we must not use window.location.origin as the API base.
export const isCdnDeploy = !!(import.meta.env.VITE_CDN_BASE_URL as string | undefined)

function getApiBase(): string {
  const stored = localStorage.getItem('dune_admin_backend')
  if (stored) {
    const safeBase = sanitizeBackendBase(stored)
    if (safeBase) return safeBase + '/api/v1'
  }

  // Single-binary deploys (AMP, local Go, k8s port-forward): SPA and API are
  // same-origin unless we're on the Vite dev server.
  if (!isCdnDeploy && typeof window !== 'undefined') {
    if (window.location.port !== '5173') {
      return window.location.origin + '/api/v1'
    }
  }
  return 'http://localhost:8080/api/v1'
}

export function getWsBase(): string {
  return getApiBase().replace(/^http/, 'ws')
}

// currentBackendBase is the resolved backend origin (without the /api/v1 suffix),
// shown to the user when the SPA can't reach the backend so they can see which
// target it's trying.
export function currentBackendBase(): string {
  return getApiBase().replace(/\/api\/v1$/, '')
}

// apiBase is the resolved /api/v1 URL for this deployment. Exported so the
// data store can derive the /api/v1/data/{file} URL without duplicating logic.
export const apiBase = getApiBase()

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

// Fired on any 401 so the AuthProvider can drop the stale session and show
// the login page without every caller handling it.
export const AUTH_EXPIRED_EVENT = 'dune-auth-expired'

function notifyAuthExpired(): void {
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new Event(AUTH_EXPIRED_EVENT))
  }
}

// Active server ID — injected into every API request as X-Dune-Server so the
// backend's serverSelectorMiddleware routes to the correct ServerContext.
// Single-server installs leave this empty; the backend falls back to its active server.
let _activeServerID = (typeof localStorage !== 'undefined'
  ? localStorage.getItem('dune_admin_active_server')
  : null) ?? ''

export function setActiveServerID(id: string): void {
  _activeServerID = id
  if (typeof localStorage !== 'undefined') {
    if (id) localStorage.setItem('dune_admin_active_server', id)
    else localStorage.removeItem('dune_admin_active_server')
  }
}

export function getActiveServerID(): string {
  return _activeServerID
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const token = await window.Clerk?.session?.getToken()
  const headers: Record<string, string> = {}
  if (body) headers['Content-Type'] = 'application/json'
  if (token) headers['Authorization'] = `Bearer ${token}`
  if (_activeServerID) headers['X-Dune-Server'] = _activeServerID
  const res = await fetch(`${apiBase}${path}`, {
    method,
    headers,
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
    credentials: 'include',
  })
  if (!res.ok) {
    if (res.status === 401 && path !== '/auth/login') notifyAuthExpired()
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
  source: 'userGame' | 'userGameOverrides' | 'userEngine' | 'defaultGame' | 'defaultEngine' | 'amp' | ''
  layers: SettingLayer[]
  // Present for curated settings only — its presence marks the setting as
  // AMP-managed (written via the AMP API under the AMP control plane).
  field_name?: string
}

export type ServerSettingUpdate = {
  section: string
  key: string
  value: string
}

export type RawLine = {
  prefix: string // '', '+', or '-'
  key: string
  value: string
}

export type RawSection = {
  section: string
  source: 'userGame' | 'userGameOverrides' | 'userEngine' | 'defaultGame' | 'defaultEngine'
  lines: RawLine[]
}

export type ServerSettingsResponse = {
  settings: ServerSetting[]
  raw: RawSection[]
  control?: string // active control plane: 'amp' | 'docker' | 'kubectl' | 'local'
}

export const MASKED = '••••••••'

export type AppConfig = {
  // Control plane
  control: string
  // SSH
  ssh_host: string
  ssh_user: string
  ssh_key: string
  ssh_mode: string // '' | 'library' (in-process) | 'command' (OS ssh wrapper)
  // Database
  db_host: string
  db_port: number
  db_user: string
  db_pass: string // masked when non-empty
  db_name: string
  db_schema: string
  // kubectl
  control_namespace: string
  // docker
  docker_gameserver: string
  docker_broker_game: string
  docker_broker_admin: string
  docker_db: string
  // local shell commands
  cmd_start: string
  cmd_stop: string
  cmd_restart: string
  cmd_status: string
  // Broker
  broker_game_addr: string
  broker_admin_addr: string
  broker_tls: boolean
  broker_user: string
  broker_pass: string // masked when non-empty
  broker_jwt_secret: string // masked when non-empty
  broker_exec_prefix: string
  // Server paths
  backup_dir: string
  server_ini_dir: string
  default_ini_dir: string
  // AMP
  amp_instance: string
  amp_container: string
  amp_user: string
  amp_log_path: string
  amp_use_container: boolean
  amp_data_root: string
  amp_api_user: string
  amp_api_pass: string // masked when non-empty
  amp_api_port: number
  director_url: string
  // Optional host override for control-plane-discovered Web Interface URLs
  // (e.g. kubectl director/file-browser). When set it takes precedence over
  // the SSH host so operators whose SSH jump host differs from the game VM host
  // can still get correct URLs (issue #234).
  web_interface_host_override: string
  // Server-level IANA timezone (e.g. "America/New_York"). Applies to activity
  // charts, scheduled restarts, and backups. Empty = host-local time.
  // Schedule-level timezone fields fall back to this when set.
  timezone: string
  // Market bot (startup config — tuning is managed in the Bot Control panel)
  market_bot_enabled: boolean
  market_bot_cache_db: string
  market_bot_item_data: string
  market_bot_state: string
  market_bot_buy_interval: string // duration string e.g. "5m0s"
  market_bot_list_interval: string // duration string e.g. "30m0s"
  market_bot_buy_threshold: number
  market_bot_max_buys: number
  market_bot_remote_url: string
  market_bot_remote_token: string // masked when non-empty
  // Discord bot
  discord_bot_enabled: boolean
  discord_bot_token: string // masked when non-empty
  discord_guild_id: string
  discord_roles_viewer: string
  discord_roles_economy: string
  discord_roles_admin: string
  discord_announce_channel_id: string
  // Discord persistent status embed (#188)
  discord_status_enabled: boolean
  discord_status_channel_id: string
  discord_status_interval_seconds: number
  // Dashboard authentication
  auth_enabled: boolean
  auth_local_username: string
  auth_local_password_hash: string // masked when non-empty
  auth_local_password_new?: string // write-only: plaintext hashed server-side
  auth_discord_enabled: boolean
  auth_discord_client_id: string
  auth_discord_client_secret: string // masked when non-empty
  auth_discord_redirect_url: string
  auth_owner_discord_ids: string
  auth_owner_role_ids: string
  auth_session_ttl_hours: number
  auth_cookie_samesite: string
  auth_guest_enabled: boolean
  // Advanced
  listen_addr: string
  scrip_currency: number
}

// ServerConfig is the per-server subset of AppConfig (everything that varies
// between game servers), plus id/name. Secrets are masked on read and the
// placeholder is restored server-side on write. Returned by GET
// /servers/{id}/config and accepted by PUT /servers/{id}/config.
export type ServerConfig = { id: number, name: string } & Partial<AppConfig>

export type ServerInfo = {
  id: number
  name: string
  active: boolean
}

// ServerHealth is the dashboard health summary for one registered server,
// returned by GET /servers/health.
export type ServerHealth = {
  id: number
  name: string
  active: boolean
  control: string
  running: boolean
  phase: string
  uptime_seconds: number
  players_online: number
  db_connected: boolean
  error?: string
}

export type Status = {
  executor: string // "ssh" | "local" | "none"
  control: string // "kubectl" | "docker" | "local" | "none"
  ssh_connected: boolean
  db_connected: boolean
  ssh_host: string
  db_host: string
  pod_ns: string
  pod_ip: string
  version?: string
  commit?: string
  build_time?: string
  director_url?: string
  listen_addr?: string
  shutdown_pending?: boolean // a broadcast restart/stop is armed on the backend
  shutdown_at?: number // Unix seconds the armed action fires (0 when none)
  needs_setup?: boolean // true when no config exists or DB password is unset
}
export type Player = {
  id: number
  account_id: number
  controller_id: number
  fls_id: string
  name: string
  class: string
  map: string
  faction_id: number
  online_status: string
  discord_user_id: string
  discord_avatar: string
}
export type LabeledCount = {
  label: string
  count: number
}
export type ActivityPoint = {
  day: string
  count: number
}
export type FactionStat = {
  faction: string
  players: number
  solaris: number
  scrip: number
  avg_level: number
}
export type FactionTrendPoint = {
  day: string
  values: Record<string, number>
}
export type FactionTrends = {
  metric: string
  factions: string[]
  points: FactionTrendPoint[]
}
export type ServerSummary = {
  total_players: number
  online_players: number
  by_map: LabeledCount[]
  by_faction: FactionStat[]
  total_solaris: number
  total_scrip: number
  avg_char_level: number
  total_playtime_secs: number
  activity_trend: ActivityPoint[]
  trend_days: number
}
export type InventoryItem = {
  id: number
  template_id: string
  name: string
  stack_size: number
  quality: number
  durability: string
  max_durability: string
}
export type CurrencyRow = {
  player_id: number
  currency_id: number
  balance: number
}
export type FactionRep = {
  actor_id: number
  faction_id: number
  faction_name: string
  reputation: number
  scrips: number
}
export type SpecTrack = {
  player_id: number
  track_type: string
  xp: number
  level: number
}
export type KeystoneRow = {
  id: number
  track: string
  name: string
  level: number
  cost: number
}
export type JourneyNode = {
  node_id: string
  is_complete: boolean
  is_revealed: boolean
  has_pending_reward: boolean
}
export type BlueprintRow = {
  id: number
  owner_name: string
  item_id: number
  pieces: number
  placeables: number
  name?: string
}
export type MapCalibration = {
  map_key: string
  min_x: number
  max_x: number
  min_y: number
  max_y: number
  flip_x: boolean
  flip_y: boolean
}

export type MapMarker = {
  type: string
  id: number
  name: string
  class?: string
  map: string
  partition_id: number
  x: number
  y: number
  z: number
  online_status?: string
  fls_id?: string
}
export type BaseRow = {
  id: number
  name: string
  pieces: number
  placeables: number
}
export type DirectorKV = {
  key: string
  value: string
  comment?: string
  secret?: boolean
}
export type DirectorSection = {
  name: string
  read_only: boolean
  lines: DirectorKV[]
}
export type DirectorConfig = {
  path: string
  sections: DirectorSection[]
}
export type RestartRule = {
  days: number[] // 0=Sun .. 6=Sat
  time: string // "HH:MM"
}
export type ScheduledRestarts = {
  enabled: boolean
  timezone: string
  rules: RestartRule[]
  warn_minutes: number
  last_fired: number
  next_restart?: string
}
export type DBBackupFile = {
  name: string
  size_bytes: number
  modified: string
}
export type RestoreStep = {
  key: string
  status: 'pending' | 'running' | 'done' | 'skipped' | 'failed'
}
export type RestoreStatus = {
  running: boolean
  done: boolean
  failed: boolean
  file: string
  steps: RestoreStep[]
  error: string
  output: string
  ignored_errors: number
  servers_stopped: boolean
}
export type CharacterBackup = {
  id: number
  account_id: number
  fls_id: string
  character_name: string
  action: string
  reason: string
  file_path: string
  patches_checksum: string
  created_at: string
}
export type BackupRule = {
  days: number[] // 0=Sun .. 6=Sat
  time: string // "HH:MM"
}
export type ScheduledBackups = {
  enabled: boolean
  timezone: string
  rules: BackupRule[]
  keep_n: number
  last_fired: number
  next_backup?: string
}
export type WebInterface = {
  label: string
  url: string
  // noProxy opts this entry out of the mesh web proxy: the SPA opens `url`
  // as-is instead of a rewritten proxy port. For NAT/reverse-proxy setups
  // where only fixed published ports are reachable, the proxy's rewritten
  // URL is unreachable — this restores the pre-v0.42.0 behaviour per entry.
  noProxy?: boolean
  // proxyPort, when set, is the local dune-admin port that reverse-proxies this
  // service over the mesh tunnel. The SPA opens it via the current host on that
  // port, so the (possibly unresolvable) game-side url is bypassed.
  proxyPort?: number
  // proxyScheme is the scheme to reach the proxy port (always "http" — the
  // listeners are plain HTTP). Use it instead of window.location.protocol so an
  // HTTPS-served dashboard still opens the http proxy port correctly.
  proxyScheme?: string
}
export type GuildSummary = {
  guild_id: number
  name: string
  description: string
  faction_id: number
  faction_name: string
  member_count: number
}
export type GuildMember = {
  player_id: number
  role_id: number
  character_name: string
}
export type GuildInvite = {
  invite_id: number
  player_id: number
  character_name: string
  sender_player_id: number
  sender_name: string
}
export type GuildDetail = GuildSummary & {
  members: GuildMember[]
  invites: GuildInvite[]
}
export type LandsraadTerm = {
  term_id: number
  start_time: string
  end_time: string
  test_term: boolean
  reigning_faction: string
  active_decree: string
  elected_decree: string
  winning_faction: string
}
export type LandsraadDecree = {
  id: number
  name: string
  weight: number
  disabled: boolean
}
export type LandsraadTask = {
  id: number
  board_index: number
  house: string
  completed: boolean
  winning_faction: string
  sysselraad: boolean
  goal_amount: number
}
export type LandsraadOverview = {
  term: LandsraadTerm | null
  decrees: LandsraadDecree[]
  tasks: LandsraadTask[]
}
export type LogPod = {
  namespace: string
  name: string
}
export type MutateResult = { ok: string }
export type BulkGiveResult = {
  given: string[]
  skipped: { template: string, reason: string }[]
}
export type BGOutput = { output: string }
export type VehicleRow = {
  id: number
  class: string
  map: string
  chassis_durability: number
  vehicle_name: string
  is_recovered: boolean
  is_backup: boolean
}
export type CheatEntry = {
  fls_id: string
  cheat_type: string
  event_time: string
  character_name: string
}
export interface DiagnosticsEnvironment {
  version: string
  go_version: string
  os: string
  arch: string
  control_plane: string
  auth_enabled: boolean
  market_bot_enabled: boolean
  active_server_count: number
}
export interface DiagnosticsReport {
  title: string
  body: string
  repo: string
}
export type GameEvent = {
  actor_id: number
  universe_time: string
  map: string
  event_type: number
  x: number
  y: number
  z: number
  custom_data: string
}
export type DungeonRecord = {
  dungeon_id: string
  difficulty: string
  duration_ms: number
  players_num: number
  completion_id: number
}
export type PlayerStats = {
  solaris_balance: number
  scrip_balance: number
  solaris_earned: number
  solaris_spent: number
  pois_discovered: number
  story_milestones: number
  max_faction_tier: number
  faction: string
  char_xp: number
  skill_points: number
  total_playtime_secs: number
  session_count: number
  avg_session_secs: number
  last_seen: string | null
}

export type SolarisPoint = {
  time: string
  balance: number
  cum_earned: number
  cum_spent: number
}

export type SessionRecord = {
  started_at: string
  ended_at: string
  duration_secs: number
}

export type StatSnapshot = {
  account_id: number
  snapped_at: string
  char_xp: number | null
  skill_points: number | null
  intel_points: number | null
  combat_xp: number | null
  crafting_xp: number | null
  gathering_xp: number | null
  exploration_xp: number | null
  sabotage_xp: number | null
  solaris_balance: number | null
}

export type IntelInfo = {
  intel: number
  level: number
  expected_at_level: number
  max: number
}

export type IntelAuditRow = {
  account_id: number
  pawn_id: number
  name: string
  level: number
  intel: number
  expected_intel: number
  online: boolean
}

export type BattlepassResetResult = {
  mode: string
  account_id: number
  ledger_rows: number
  claims: number
  seen_markers?: number
}

export type TeleportLocation = {
  name: string
  x: number
  y: number
  z: number
}
export type OnlineRow = {
  player_id: number
  name: string
  map: string
  status: string
  last_seen: string
}
export type BackupFile = {
  name: string
  size_bytes: number
  modified: string
  has_yaml: boolean
}

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
  search?: string | undefined
  category?: string | undefined
  tier?: number | undefined
  rarity?: string | undefined
  owner?: 'bot' | 'player' | undefined
  page?: number | undefined
  limit?: number | undefined
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
  running: boolean
  mode?: 'embedded' | 'remote' | 'none'
  configured?: boolean
  enabled?: boolean
  uptime: string
  last_list_tick: string | null
  last_buy_tick: string | null
  next_list_tick?: string | null
  next_buy_tick?: string | null
  listing_count: number
  balance?: number
  error_count: number
  error?: string // set when running=false
}
export type BotConfig = {
  list_interval: string
  buy_interval: string
  rarity_multipliers: Record<string, number>
  vendor_multipliers?: Record<string, number>
  grade_multipliers: number[]
  buy_threshold: number
  max_buys: number
  listings_per_grade: number
  disabled_items: string[]
  enabled: boolean
}

function normalizeBotConfig(raw: unknown): BotConfig {
  const src = (raw && typeof raw === 'object') ? (raw as Record<string, unknown>) : {}
  return {
    list_interval: typeof src.list_interval === 'string'
      ? src.list_interval
      : (typeof src.list_tick_interval === 'string' ? src.list_tick_interval : '30m0s'),
    buy_interval: typeof src.buy_interval === 'string'
      ? src.buy_interval
      : (typeof src.buy_tick_interval === 'string' ? src.buy_tick_interval : '5m0s'),
    rarity_multipliers: (src.rarity_multipliers as Record<string, number> | undefined) ?? {},
    vendor_multipliers: (src.vendor_multipliers as Record<string, number> | undefined) ?? {},
    grade_multipliers: Array.isArray(src.grade_multipliers)
      ? (src.grade_multipliers as number[])
      : [],
    buy_threshold: typeof src.buy_threshold === 'number' ? src.buy_threshold : 1.05,
    max_buys: typeof src.max_buys === 'number'
      ? src.max_buys
      : (typeof src.max_buys_per_tick === 'number' ? src.max_buys_per_tick : 50),
    listings_per_grade: typeof src.listings_per_grade === 'number' ? src.listings_per_grade : 5,
    disabled_items: Array.isArray(src.disabled_items) ? (src.disabled_items as string[]) : [],
    enabled: typeof src.enabled === 'boolean' ? src.enabled : true,
  }
}

export type ProgressionPreset = {
  id: string
  name: string
  description: string
  node_count: number
  nodes: string[]
}

export type ContractEntry = {
  id: string
  alias: string
  tag_count: number
}

export interface UpdateCheckResult {
  current: string
  latest: string
  needs_update: boolean
  release_url?: string
}

export interface UpdateApplyResult {
  updated: boolean
  version?: string
  message: string
}

export interface WelcomePackageItem {
  template: string
  qty: number
  quality: number
}

export interface WelcomePackage {
  version: string
  items: WelcomePackageItem[]
}

export interface WelcomePackageConfig {
  enabled: boolean
  scan_interval_secs: number
  active_version: string
  active_versions: string[]
  packages: WelcomePackage[]
  welcome_message_enabled: boolean
  welcome_message: string
  welcome_whisper_source_player: string
  motd_enabled: boolean
  motd_message: string
  motd_source_player: string
  region_join_enabled: boolean
  region_leave_enabled: boolean
  region_join_template: string
  region_leave_template: string
  region_chat_channel?: string
}

export interface WelcomeGrantRecord {
  fls_id: string
  package_version: string
  account_id: number
  character_name: string
  status: string
  granted_at: string
  attempts: number
  last_error: string
  updated_at: string
}

export interface GivePackItem {
  template: string
  qty: number
  quality: number
}

export interface GivePack {
  id: string
  name: string
  category: string
  tier: number
  items: GivePackItem[]
}

export interface GivePacksConfig {
  packs: GivePack[]
}

export type EventType = 'zone_race' | 'milestone'

export interface EventDefinition {
  id: number
  name: string
  type: EventType
  enabled: boolean
  version: number
  config: string
  reward: string
  announce_channel_id: string
  announce_template: string
  poll_seconds: number
  jitter_seconds: number
  created_at: string
  updated_at: string
}

export interface EventClaimRecord {
  event_id: number
  version: number
  account_id: number
  status: string
  claimed_at: string
  attempts: number
  last_error: string
  next_attempt_at: string
  updated_at: string
}

export interface EventStatus {
  event: EventDefinition
  claims: EventClaimRecord[]
}

export type BattlepassSignal = 'level' | 'journey_node' | 'player_tag'
export type BattlepassClaimStatus = 'baseline' | 'earned' | 'granted'

export interface BattlepassTier {
  id: number
  tier_key: string
  category: string
  label: string
  signal: BattlepassSignal
  signal_key: string
  threshold: number
  intel: number
  reward_items: string
  enabled: boolean
}

export interface BattlepassTierUpdate {
  label?: string
  intel: number
  enabled: boolean
  reward_items?: string
  category?: string
  signal?: BattlepassSignal
  signal_key?: string
  threshold?: number
}

export interface BattlepassTierCreate {
  tier_key: string
  category: string
  label: string
  signal: BattlepassSignal
  signal_key: string
  threshold: number
  intel: number
  enabled: boolean
  reward_items?: string
}

export interface BattlepassCatalogExport {
  version: number
  tiers: Omit<BattlepassTier, 'id'>[]
}

export interface BattlepassTierCounts {
  baseline: number
  earned: number
  granted: number
}

export interface BattlepassTiersResponse {
  tiers: BattlepassTier[]
  counts: Record<string, BattlepassTierCounts>
  player_count: number
  default_count: number
}

export interface BattlepassClaim {
  tier_key: string
  account_id: number
  status: BattlepassClaimStatus
  intel: number
  earned_at: string
  granted_at: string
  attempts: number
  last_error: string
}

export interface BattlepassProgress {
  claims: BattlepassClaim[]
  pending_intel: number
}

export interface BattlepassPendingRow {
  account_id: number
  name: string
  online: boolean
  tier_key: string
  tier_label: string
  intel: number
  reward_items: string
}

export interface EventsConfig {
  events_enabled: boolean | null
}

export interface BattlepassConfig {
  battlepass_enabled: boolean | null
  battlepass_award_past: boolean | null
  battlepass_auto_grant: boolean | null
  battlepass_poll_seconds: number
  battlepass_scan_pace_ms: number
  battlepass_scan_start_delay_ms: number
}

// ── dashboard authentication ────────────────────────────────────────────────

export type AuthSessionInfo = {
  sub: string
  name: string
  method: 'local' | 'discord' | 'guest'
  avatar?: string
  owner: boolean
  capabilities: string[] | null
}

export type AuthStatus = {
  enabled: boolean
  methods: { local: boolean, discord: boolean, guest: boolean }
  session: AuthSessionInfo | null
}

export type DiscordMember = {
  id: string
  name: string
  username: string
  avatar?: string
}

// DiscordServerLink is one game server's Discord wiring: the single guild it is
// linked to plus its own announce/status channels and status-embed tuning.
// Mirrors the Go discordServerLink struct returned by GET /discord/servers.
export type DiscordServerLink = {
  server_id: number
  guild_id: string
  announce_channel_id: string
  status_channel_id: string
  status_enabled: boolean
  status_interval_seconds: number
}

// DiscordGuild now carries only guild-level capability roles (three
// comma-separated CSVs). Servers link to a guild from their own per-server
// Discord tab; the link is no longer stored on the guild. Mirrors the Go
// discordGuild struct returned by GET /discord/guilds. Auth is guild-level;
// the same bot token serves every guild.
export type DiscordGuild = {
  guild_id: string
  roles_viewer: string
  roles_economy: string
  roles_admin: string
}

// A guild the bot belongs to (GET /discord/available-guilds) — used to offer a
// name-labelled guild dropdown instead of pasting raw snowflake ids.
export type DiscordGuildOption = {
  id: string
  name: string
}

// A postable text/announcement channel in a guild (GET /discord/channels) — used
// for the searchable announce/status channel pickers.
export type DiscordChannelOption = {
  id: string
  name: string
}

export type PermissionsCapability = {
  id: string
  description: string
}

export type PermissionsData = {
  matrix: Record<string, string[]>
  capabilities: PermissionsCapability[]
  guild_roles: { id: string, name: string }[]
  seed_defaults: string[]
}

export type AuthLocalUser = {
  username: string
  capabilities: string[]
  enabled: boolean
  created_at: string
  updated_at: string
}

export const authApi = {
  status: () => req<AuthStatus>('GET', '/auth/status'),
  login: (username: string, password: string) =>
    req<{ status: string }>('POST', '/auth/login', { username, password }),
  logout: () => req<{ status: string }>('POST', '/auth/logout'),
  guest: () => req<{ status: string }>('POST', '/auth/guest'),
  discordLoginUrl: () => `${apiBase}/auth/discord/login`,
  permissions: {
    get: () => req<PermissionsData>('GET', '/auth/permissions'),
    save: (matrix: Record<string, string[]>) =>
      req<{ status: string }>('PUT', '/auth/permissions', { matrix }),
  },
  users: {
    list: () => req<AuthLocalUser[]>('GET', '/auth/users'),
    save: (username: string, body: { password?: string | undefined, capabilities: string[], enabled: boolean }) =>
      req<{ status: string }>('PUT', `/auth/users/${encodeURIComponent(username)}`, body),
    remove: (username: string) =>
      req<{ status: string }>('DELETE', `/auth/users/${encodeURIComponent(username)}`),
  },
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
    // global=true (Settings modal) persists only global settings and never
    // touches the connection or creates/reconnects a server.
    save: (cfg: AppConfig, global = false) =>
      req<Status>('POST', global ? '/config?scope=global' : '/config', cfg),
    discover: (persist = false) =>
      req<{
        db_user: string
        db_name: string
        db_pass: string
        broker_game: string
        broker_admin: string
        director_url: string
        persisted: boolean
      }>('POST', `/discover${persist ? '?persist=true' : ''}`),
  },
  serverSettings: {
    get: () => req<ServerSettingsResponse>('GET', '/server-settings'),
    update: (updates: ServerSettingUpdate[]) =>
      req<{ ok: string, applied: number, cleared: number }>('PUT', '/server-settings', { updates }),
    updateRaw: (section: string, lines: string) =>
      req<{ ok: string }>('PUT', '/server-settings/raw', { section, lines }),
  },

  battlegroup: {
    status: () => req<unknown>('GET', '/battlegroup/status'),
    exec: (cmd: string) => req<BGOutput>('POST', '/battlegroup/exec', { cmd }),
    pods: () => req<{ pods: string[], namespace: string }>('GET', '/battlegroup/pods'),
    backupFiles: () => req<BackupFile[]>('GET', '/battlegroup/backup-files'),
    backupDownloadUrl: (file: string) => `${apiBase}/battlegroup/backup-files/download?file=${encodeURIComponent(file)}`,
    backupUpload: async (file: File): Promise<{ name: string }> => {
      const form = new FormData()
      form.append('backup', file)
      const token = await window.Clerk?.session?.getToken()
      const headers: Record<string, string> = {}
      if (token) headers['Authorization'] = `Bearer ${token}`
      const res = await fetch(`${apiBase}/battlegroup/backup-files/upload`, { method: 'POST', headers, body: form, credentials: 'include' })
      if (!res.ok) {
        const e = await res.json().catch(() => ({ error: res.statusText }))
        throw new ApiError(res.status, e.error)
      }
      return res.json()
    },
    restore: (file: string) => req<{ ok: string }>('POST', '/battlegroup/restore', { file }),
  },

  players: {
    list: () => req<Player[]>('GET', '/players'),
    summary: () => req<ServerSummary>('GET', '/players/summary'),
    factionTrends: (metric: 'solaris' | 'level') =>
      req<FactionTrends>('GET', `/players/faction-trends?metric=${metric}`),
    online: () => req<OnlineRow[]>('GET', '/players/online'),
    currency: () => req<CurrencyRow[]>('GET', '/players/currency'),
    factions: () => req<FactionRep[]>('GET', '/players/factions'),
    specs: () => req<SpecTrack[]>('GET', '/players/specs'),
    templates: () => req<{ id: string, name: string }[]>('GET', '/players/templates'),
    inventory: (id: number) => req<InventoryItem[]>('GET', `/players/${id}/inventory`),
    journey: (accountId: number) => req<JourneyNode[]>('GET', `/players/${accountId}/journey`),
    giveItem: (player_id: number, template: string, qty: number, quality: number) =>
      req<MutateResult>('POST', '/players/give-item', { player_id, template, qty, quality }),
    giveItems: (player_id: number, items: { template: string, qty: number, quality: number }[]) =>
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
    intelCurrent: (id: number) => req<IntelInfo>('GET', `/players/${id}/intel`),
    setIntel: (player_id: number, amount: number) =>
      req<MutateResult>('POST', '/players/set-intel', { player_id, amount }),
    intelAudit: () => req<IntelAuditRow[]>('GET', '/players/intel-audit'),
    rename: (account_id: number, name: string) => req<MutateResult>('POST', '/players/rename', { account_id, name }),
    deleteCharacter: (account_id: number, reason: string, character_name: string, backup: boolean) =>
      req<MutateResult>('POST', '/players/delete', { account_id, reason, character_name, backup }),
    tags: (account_id: number) => req<string[]>('GET', `/players/${account_id}/tags`),
    updateTags: (account_id: number, add: string[], remove: string[]) => req<MutateResult>('POST', '/players/update-tags', { account_id, add, remove }),
    returningPlayerAward: (account_id: number) => req<MutateResult>('POST', '/players/returning-player-award', { account_id }),
    dismissReturningPlayerAward: (account_id: number) => req<MutateResult>('POST', '/players/dismiss-returning-player-award', { account_id }),
    backupCharacter: (account_id: number, character_name: string, reason: string) =>
      req<MutateResult>('POST', `/players/${account_id}/backup`, { character_name, reason }),
    listBackups: (account_id: number) => req<CharacterBackup[]>('GET', `/players/${account_id}/backups`),
    deleteAccount: (account_id: number, reason: string) => req<MutateResult>('POST', '/players/delete-account', { account_id, reason }),
    deleteItem: (id: number) => req<MutateResult>('DELETE', `/players/item/${id}`),
    updateItem: (id: number, stack_size: number, quality: number) =>
      req<MutateResult>('PUT', `/players/item/${id}`, { stack_size, quality }),
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
    charXPCurrent: (id: number) => req<{ xp: number, level: number }>('GET', `/players/${id}/char-xp`),
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
      req<{ repaired: number, scanned: number }>('POST', '/players/repair-gear', { player_id }),
    partitions: () => req<TeleportLocation[]>('GET', '/players/partitions'),
    teleport: (fls_id: string, partition_label: string) =>
      req<MutateResult>('POST', '/players/teleport', { fls_id, partition_label }),
    teleportCoords: (fls_id: string, x: number, y: number, z: number, partition_id?: number) =>
      req<MutateResult>('POST', '/players/teleport-coords', { fls_id, x, y, z, partition_id }),
    position: (id: number) =>
      req<{ partition_id: number, map: string, x: number, y: number, z: number }>('GET', `/players/${id}/position`),
    teleportToPlayer: (source_fls_id: string, target_id: number) =>
      req<MutateResult & { path: string, x: number, y: number, z: number }>(
        'POST', '/players/teleport-to-player', { source_fls_id, target_id }),
    events: (id: number) => req<GameEvent[]>('GET', `/players/${id}/events`),
    dungeons: (id: number) => req<DungeonRecord[]>('GET', `/players/${id}/dungeons`),
    stats: (id: number) => req<PlayerStats>('GET', `/players/${id}/stats`),
    solarisHistory: (id: number) => req<SolarisPoint[]>('GET', `/players/${id}/solaris-history`),
    sessionHistory: (id: number) => req<SessionRecord[]>('GET', `/players/${id}/session-history`),
    statSnapshots: (id: number) => req<StatSnapshot[]>('GET', `/players/${id}/stat-snapshot-history`),
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
    spawnVehicle: (
      fls_id: string,
      class_name: string,
      x: number,
      y: number,
      z: number,
      options?: { rotation?: number, template_name?: string, persistent?: boolean, faction?: string },
    ) =>
      req<MutateResult>('POST', '/vehicles/spawn', { fls_id, class_name, x, y, z, ...options }),
  },

  locations: {
    list: () => req<TeleportLocation[]>('GET', '/locations'),
    upsert: (name: string, x: number, y: number, z: number) =>
      req<TeleportLocation[]>('POST', '/locations', { name, x, y, z }),
    rename: (old_name: string, new_name: string) =>
      req<TeleportLocation[]>('PUT', '/locations', { old_name, new_name }),
    remove: (name: string) =>
      req<TeleportLocation[]>('DELETE', '/locations', { name }),
  },

  broadcast: {
    send: (texts: { Key: string, Title: string, Body: string }[], duration_sec = 30) =>
      req<MutateResult>('POST', '/broadcast', { texts, duration_sec }),
    shutdown: (shutdown_type: string, delay_minutes: number, cancel = false) =>
      req<MutateResult>('POST', '/broadcast/shutdown', { shutdown_type, delay_minutes, cancel }),
  },

  chat: {
    // Whisper a single player from the seeded GM/Server persona. Identified by
    // recipient account id; the backend resolves the chat identity and attributes
    // the message to the GM persona.
    whisper: (account_id: number, message: string) =>
      req<MutateResult>('POST', '/chat/whisper', { account_id, message }),
  },

  contracts: {
    list: () => req<ContractEntry[]>('GET', '/contracts'),
  },

  database: {
    tables: () => req<{ name: string, row_count: number }[]>('GET', '/database/tables'),
    describe: (table: string) => req<{ table: string, columns: { name: string, data_type: string, nullable: string }[] }>('GET', `/database/describe?table=${encodeURIComponent(table)}`),
    sample: (table: string, limit = 20) => req<{ table: string, headers: string[], rows: string[][] }>('GET', `/database/sample?table=${encodeURIComponent(table)}&limit=${limit}`),
    search: (term: string) => req<{ headers: string[], rows: string[][] }>('GET', `/database/search?term=${encodeURIComponent(term)}`),
    sql: (sql: string) => req<{ headers: string[], rows: string[][], truncated: boolean }>('POST', '/database/sql', { sql }),
  },

  map: {
    markers: (mapKey: string) => req<MapMarker[]>('GET', `/map/markers?map=${encodeURIComponent(mapKey)}`),
    calibration: {
      get: (mapKey: string) => req<MapCalibration>('GET', `/map/calibration?map=${encodeURIComponent(mapKey)}`),
      save: (mapKey: string, c: Omit<MapCalibration, 'map_key'>) =>
        req<MapCalibration>('PUT', `/map/calibration?map=${encodeURIComponent(mapKey)}`, c),
      remove: (mapKey: string) =>
        req<{ deleted: boolean }>('DELETE', `/map/calibration?map=${encodeURIComponent(mapKey)}`),
    },
  },

  logs: {
    pods: () => req<LogPod[]>('GET', '/logs/pods'),
    cheats: () => req<CheatEntry[]>('GET', '/logs/cheats'),
  },

  diagnostics: {
    environment: () => req<DiagnosticsEnvironment>('GET', '/diagnostics/environment'),
    report: () => req<DiagnosticsReport>('GET', '/diagnostics/report'),
    bundleUrl: () => `${apiBase}/diagnostics/bundle`,
    streamUrl: () => `${getWsBase()}/diagnostics/logs/stream`,
  },

  storage: {
    list: () => req<{ id: number, name: string, class: string, map: string, item_count: number, item_templates: string[], item_names: string[], owner_name: string }[]>('GET', '/storage'),
    items: (id: number) => req<InventoryItem[]>('GET', `/storage/${id}/items`),
    giveItem: (id: number, template: string, qty: number, quality: number) =>
      req<MutateResult>('POST', `/storage/${id}/give-item`, { template, qty, quality }),
    giveItems: (id: number, items: { template: string, qty: number, quality: number }[]) =>
      req<BulkGiveResult>('POST', `/storage/${id}/give-items`, { items }),
  },

  blueprints: {
    list: () => req<BlueprintRow[]>('GET', '/blueprints'),
    exportUrl: (id: number) => `${apiBase}/blueprints/${id}/export`,
    import: async (file: File, player_id: number) => {
      const token = await window.Clerk?.session?.getToken()
      const headers: Record<string, string> = {}
      if (token) headers['Authorization'] = `Bearer ${token}`
      const fd = new FormData()
      fd.append('file', file)
      fd.append('player_id', String(player_id))
      return fetch(`${apiBase}/blueprints/import`, { method: 'POST', headers, body: fd, credentials: 'include' })
        .then((r) => r.json())
    },
  },

  bases: {
    list: () => req<BaseRow[]>('GET', '/bases'),
    exportUrl: (id: number) => `${apiBase}/bases/${id}/export`,
  },

  director: {
    get: () => req<DirectorConfig>('GET', '/director-config'),
    update: (updates: Record<string, Record<string, string>>) =>
      req<MutateResult>('PUT', '/director-config', { updates }),
  },

  scheduledRestarts: {
    get: () => req<ScheduledRestarts>('GET', '/scheduled-restarts'),
    update: (body: { enabled: boolean, timezone: string, rules: RestartRule[], warn_minutes: number }) =>
      req<MutateResult>('PUT', '/scheduled-restarts', body),
    skipNext: () => req<MutateResult>('POST', '/scheduled-restarts/skip-next'),
  },

  dbBackups: {
    list: () => req<{ backups: DBBackupFile[] }>('GET', '/db-backups'),
    take: () => req<{ ok: string, name: string, size_bytes: number }>('POST', '/db-backups'),
    remove: (file: string) => req<MutateResult>('DELETE', `/db-backups?file=${encodeURIComponent(file)}`),
    downloadUrl: (file: string) => `${apiBase}/db-backups/download?file=${encodeURIComponent(file)}`,
    restore: (file: string) =>
      req<{ ok: string }>('POST', '/db-backups/restore', { file, confirm: true }),
    restoreStatus: () => req<RestoreStatus>('GET', '/db-backups/restore/status'),
  },
  characterBackups: {
    restore: (id: number) =>
      req<{ ok: string, player_controller_id: number }>('POST', `/character-backups/${id}/restore`, { confirm: true }),
    remove: (id: number) => req<MutateResult>('DELETE', `/character-backups/${id}`),
    downloadUrl: (id: number) => `${apiBase}/character-backups/${id}/download`,
  },
  scheduledBackups: {
    get: () => req<ScheduledBackups>('GET', '/scheduled-backups'),
    update: (body: { enabled: boolean, timezone: string, rules: BackupRule[], keep_n: number }) =>
      req<MutateResult>('PUT', '/scheduled-backups', body),
  },
  webInterfaces: {
    get: () => req<{ interfaces: WebInterface[], discovered?: WebInterface[] }>('GET', '/web-interfaces'),
    update: (interfaces: WebInterface[]) => req<MutateResult>('PUT', '/web-interfaces', { interfaces }),
  },
  guilds: {
    list: () => req<GuildSummary[]>('GET', '/guilds'),
    get: (id: number) => req<GuildDetail>('GET', `/guilds/${id}`),
    update: (id: number, body: { name?: string, description?: string }) =>
      req<GuildDetail>('PATCH', `/guilds/${id}`, body),
    setRole: (id: number, playerId: number, role: number) =>
      req<MutateResult>('PUT', `/guilds/${id}/members/${playerId}/role`, { role }),
  },

  landsraad: {
    get: () => req<LandsraadOverview>('GET', '/landsraad'),
  },

  market: {
    items: (params?: MarketItemsParams) => {
      const q = new URLSearchParams()
      if (params?.search) q.set('search', params.search)
      if (params?.category) q.set('category', params.category)
      if (params?.tier != null) q.set('tier', String(params.tier))
      if (params?.rarity) q.set('rarity', params.rarity)
      if (params?.owner) q.set('owner', params.owner)
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
    config: async () => normalizeBotConfig(await req<unknown>('GET', '/market-bot/config')),
    saveConfig: async (cfg: BotConfig) => normalizeBotConfig(await req<unknown>('PUT', '/market-bot/config', cfg)),
    lifecycle: (cmd: 'start' | 'stop' | 'restart') => req<{ output: string }>('POST', '/market-bot/exec', { cmd }),
    cleanup: () => req<{ orders_deleted: number, items_deleted: number }>('POST', '/market-bot/cleanup'),
    logsReady: () => req<{ ready: boolean, reason?: string, namespace?: string, name?: string }>('GET', '/market-bot/logs-ready'),
  },

  discord: {
    // roles is per-guild: pass a guild id to list that guild's roles for its
    // role pickers. Omitting it falls back to the bot's default guild.
    roles: (guildId?: string) =>
      req<{ id: string, name: string }[]>(
        'GET',
        guildId ? `/discord/roles?guild=${encodeURIComponent(guildId)}` : '/discord/roles',
      ),
    // availableGuilds lists the guilds the bot is a member of (id + name) so the
    // UI can offer a guild dropdown. Requires a running bot or a configured token.
    availableGuilds: () => req<DiscordGuildOption[]>('GET', '/discord/available-guilds'),
    // channels lists a guild's postable channels for the announce/status pickers.
    channels: (guildId?: string) =>
      req<DiscordChannelOption[]>(
        'GET',
        guildId ? `/discord/channels?guild=${encodeURIComponent(guildId)}` : '/discord/channels',
      ),
    membersSearch: (q: string) =>
      req<DiscordMember[]>(`GET`, `/discord/members/search?q=${encodeURIComponent(q)}`),
    // guilds carry capability roles only. Servers link to a guild from their
    // own per-server Discord tab (discord.servers below).
    guilds: {
      list: () => req<DiscordGuild[]>('GET', '/discord/guilds'),
      upsert: (g: DiscordGuild) => req<DiscordGuild>('POST', '/discord/guilds', g),
      remove: (guildId: string) =>
        req<{ deleted: boolean }>('DELETE', `/discord/guilds/${encodeURIComponent(guildId)}`),
    },
    // servers: each game server links to exactly one guild + its own channels.
    servers: {
      list: () => req<DiscordServerLink[]>('GET', '/discord/servers'),
      set: (serverId: number, link: Omit<DiscordServerLink, 'server_id'>) =>
        req<DiscordServerLink>('PUT', `/discord/servers/${serverId}`, link),
      unlink: (serverId: number) =>
        req<{ deleted: boolean }>('DELETE', `/discord/servers/${serverId}`),
    },
  },

  update: {
    check: () => req<UpdateCheckResult>('GET', '/update/check'),
    apply: (force?: boolean) => req<UpdateApplyResult>('POST', '/update/apply', force ? { force: true } : undefined),
  },

  welcomePackage: {
    config: () => req<WelcomePackageConfig>('GET', '/welcome-package/config'),
    saveConfig: (cfg: WelcomePackageConfig) => req<WelcomePackageConfig>('PUT', '/welcome-package/config', cfg),
    grants: (limit?: number) =>
      req<WelcomeGrantRecord[]>('GET', `/welcome-package/grants${limit ? `?limit=${limit}` : ''}`),
    retry: (flsId: string, packageVersion: string, accountId: number) =>
      req<{ cleared: number }>('POST', '/welcome-package/retry', {
        fls_id: flsId,
        package_version: packageVersion,
        account_id: accountId,
      }),
    revoke: (flsId: string, packageVersion: string, accountId: number) =>
      req<{ revoked: number }>('POST', '/welcome-package/revoke', {
        fls_id: flsId,
        package_version: packageVersion,
        account_id: accountId,
      }),
    run: () => req<{ granted: number, failed: number, skipped: number }>('POST', '/welcome-package/run'),
    override: (accountId: number, packageVersion: string) =>
      req<{ granted: boolean, account_id: number, character_name: string }>('POST', '/welcome-package/override', {
        account_id: accountId,
        package_version: packageVersion,
      }),
  },

  givePacks: {
    config: () => req<GivePacksConfig>('GET', '/give-packs/config'),
    saveConfig: (cfg: GivePacksConfig) => req<GivePacksConfig>('PUT', '/give-packs/config', cfg),
  },

  maps: {
    list: () => req<string[]>('GET', '/maps'),
  },

  events: {
    list: () => req<EventDefinition[]>('GET', '/events'),
    create: (def: Partial<EventDefinition>) => req<EventDefinition>('POST', '/events', def),
    update: (id: number, def: Partial<EventDefinition>) => req<EventDefinition>('PUT', `/events/${id}`, def),
    delete: (id: number) => req<{ ok: boolean }>('DELETE', `/events/${id}`),
    setEnabled: (id: number, enabled: boolean) =>
      req<{ ok: boolean }>('POST', `/events/${id}/enable`, { enabled }),
    status: (id: number) => req<EventStatus>('GET', `/events/${id}/status`),
    reset: (id: number) => req<{ ok: boolean }>('POST', `/events/${id}/reset`),
    grantClaim: (id: number, accountId: number) =>
      req<{ ok: boolean }>('POST', `/events/${id}/claims/${accountId}/grant`),
    config: () => req<EventsConfig>('GET', '/events/config'),
    saveConfig: (cfg: EventsConfig) => req<EventsConfig>('PUT', '/events/config', cfg),
  },
  servers: {
    list: () => req<ServerInfo[]>('GET', '/servers'),
    health: () => req<ServerHealth[]>('GET', '/servers/health'),
    add: (cfg: ServerConfig) =>
      req<ServerInfo>('POST', '/servers', cfg),
    discover: (cfg: ServerConfig) =>
      req<Partial<AppConfig>>('POST', '/servers/discover', cfg),
    setActive: (id: number) => req<{ active: number }>('PUT', '/servers/active', { id }),
    remove: (id: number) => req<{ deleted: boolean }>('DELETE', `/servers/${id}`),
    reconnect: (id: number) => req<{ connected: boolean }>('POST', `/servers/${id}/reconnect`),
    getConfig: (id: number) => req<ServerConfig>('GET', `/servers/${id}/config`),
    saveConfig: (id: number, cfg: ServerConfig) =>
      req<ServerConfig>('PUT', `/servers/${id}/config`, cfg),
  },

  battlepass: {
    tiers: () => req<BattlepassTiersResponse>('GET', '/battlepass/tiers'),
    createTier: (body: BattlepassTierCreate) => req<BattlepassTier>('POST', '/battlepass/tiers', body),
    updateTier: (id: number, body: BattlepassTierUpdate) =>
      req<BattlepassTier>('PUT', `/battlepass/tiers/${id}`, body),
    tiersBulk: (ids: number[], action: 'enable' | 'disable' | 'delete') =>
      req<{ ok: boolean, count: number }>('POST', '/battlepass/tiers/bulk', { ids, action }),
    progress: (accountId: number) => req<BattlepassProgress>('GET', `/battlepass/progress/${accountId}`),
    pending: () => req<BattlepassPendingRow[]>('GET', '/battlepass/pending'),
    reseed: () => req<{ seeded: number }>('POST', '/battlepass/reseed'),
    grant: (account_id: number) => req<{ granted_intel: number, tiers: number }>('POST', '/battlepass/grant', { account_id }),
    grantTier: (account_id: number, tier_key: string) => req<{ granted_intel: number }>('POST', '/battlepass/grant-tier', { account_id, tier_key }),
    exportCatalog: () => req<BattlepassCatalogExport>('GET', '/battlepass/export'),
    importCatalog: (payload: BattlepassCatalogExport) => req<{ imported: number }>('POST', '/battlepass/import', payload),
    config: () => req<BattlepassConfig>('GET', '/battlepass/config'),
    saveConfig: (cfg: BattlepassConfig) => req<BattlepassConfig>('PUT', '/battlepass/config', cfg),
    resetClaims: (mode: 'demote' | 'purge', account_id?: number) =>
      req<BattlepassResetResult>('POST', '/battlepass/claims/reset', { mode, account_id: account_id ?? 0 }),
  },
}
