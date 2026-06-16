import type { AppConfig } from '../../api/client'

// ── defaults (all empty — never show fake values) ─────────────────────────────

export const EMPTY: AppConfig = {
  control: '',
  ssh_host: '', ssh_user: '', ssh_key: '',
  db_host: '', db_port: 0, db_user: '',
  db_pass: '', db_name: '', db_schema: '',
  control_namespace: '',
  docker_gameserver: '', docker_broker_game: '', docker_broker_admin: '', docker_db: '',
  cmd_start: '', cmd_stop: '', cmd_restart: '', cmd_status: '',
  broker_game_addr: '', broker_admin_addr: '', broker_tls: false,
  broker_user: '', broker_pass: '', broker_jwt_secret: '', broker_exec_prefix: '',
  backup_dir: '', server_ini_dir: '', default_ini_dir: '',
  amp_instance: '', amp_container: '', amp_user: '', amp_log_path: '',
  amp_use_container: false, amp_data_root: '',
  amp_api_user: '', amp_api_pass: '', amp_api_port: 0,
  director_url: '',
  market_bot_enabled: false,
  market_bot_cache_db: '', market_bot_item_data: '', market_bot_state: '',
  market_bot_buy_interval: '', market_bot_list_interval: '',
  market_bot_buy_threshold: 0, market_bot_max_buys: 0,
  market_bot_remote_url: '', market_bot_remote_token: '',
  discord_bot_enabled: false,
  discord_bot_token: '',
  discord_guild_id: '',
  discord_roles_viewer: '',
  discord_roles_economy: '',
  discord_roles_admin: '',
  discord_announce_channel_id: '',
  discord_status_enabled: false,
  discord_status_channel_id: '',
  discord_status_interval_seconds: 60,
  auth_enabled: false,
  auth_local_username: '', auth_local_password_hash: '', auth_local_password_new: '',
  auth_discord_enabled: false,
  auth_discord_client_id: '', auth_discord_client_secret: '', auth_discord_redirect_url: '',
  auth_owner_discord_ids: '', auth_owner_role_ids: '',
  auth_session_ttl_hours: 0,
  auth_cookie_samesite: '',
  auth_guest_enabled: false,
  listen_addr: '', scrip_currency: 0,
}

// Pointer-backed boolean fields in the Go config: null means "use server
// default" (effectively true). If the API returns null for these, coerce to
// true so the checkbox reflects the real server default rather than silently
// inheriting EMPTY's false and overwriting the default-on value on save.
// discord_bot_enabled is intentionally excluded: nil means default-off, not default-on.
export const pointerBoolFields = new Set<keyof AppConfig>(['amp_use_container', 'market_bot_enabled'])

export const mergeConfig = (fetched: Record<string, unknown>): AppConfig => {
  const result: AppConfig = { ...EMPTY }
  for (const key of Object.keys(fetched) as (keyof AppConfig)[]) {
    const v = fetched[key]
    if (v !== null && v !== undefined) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(result as any)[key] = v
    }
    else if (v === null && pointerBoolFields.has(key)) {
      // Null pointer-backed bool: the server field is unset (default-on).
      // Keep the EMPTY default only if it matches server intent (true = default).
      // Override EMPTY's false with true so the checkbox reflects the real default.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(result as any)[key] = true
    }
  }
  return result
}

// PER_SERVER_KEYS are the AppConfig fields that live on a ServerConfig (vary
// between game servers). Everything else (auth, Discord bot, listen addr, market
// bot, scrip currency) is global dune-admin config. Used to split the unified
// editing view into the two save targets: api.config (global) and
// api.servers.saveConfig (per-server).
export const PER_SERVER_KEYS: (keyof AppConfig)[] = [
  'control', 'control_namespace',
  'ssh_host', 'ssh_user', 'ssh_key',
  'db_host', 'db_port', 'db_user', 'db_pass', 'db_name', 'db_schema',
  'docker_gameserver', 'docker_broker_game', 'docker_broker_admin', 'docker_db',
  'cmd_start', 'cmd_stop', 'cmd_restart', 'cmd_status',
  'broker_game_addr', 'broker_admin_addr', 'broker_tls', 'broker_user',
  'broker_pass', 'broker_jwt_secret', 'broker_exec_prefix',
  'backup_dir', 'server_ini_dir', 'default_ini_dir',
  'amp_instance', 'amp_container', 'amp_user', 'amp_log_path', 'amp_use_container',
  'amp_data_root', 'amp_api_user', 'amp_api_pass', 'amp_api_port',
  'director_url',
  // Market bot enable is PER SERVER (the rest of the bot config is global/shared).
  'market_bot_enabled',
]
export const PER_SERVER_KEY_SET = new Set<string>(PER_SERVER_KEYS as string[])

// pickPerServer extracts the per-server fields from the unified editing config.
export const pickPerServer = (cfg: AppConfig): Partial<AppConfig> => {
  const out: Partial<AppConfig> = {}
  for (const k of PER_SERVER_KEYS) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(out as any)[k] = cfg[k]
  }
  return out
}

// pickGlobal extracts the global (non-per-server) fields from the editing config.
export const pickGlobal = (cfg: AppConfig): Partial<AppConfig> => {
  const out: Partial<AppConfig> = {}
  for (const k of Object.keys(cfg) as (keyof AppConfig)[]) {
    if (!PER_SERVER_KEY_SET.has(k)) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(out as any)[k] = cfg[k]
    }
  }
  return out
}
