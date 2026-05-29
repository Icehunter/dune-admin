import { useState, useEffect } from 'react'
import { Button, SearchField, Select, ListBox, Spinner, toast } from '@heroui/react'
import { api, MASKED } from '../api/client'
import type { AppConfig } from '../api/client'
import { Panel, SectionDivider } from '../dune-ui'
import { Icon } from '../dune-ui'

// ── defaults ──────────────────────────────────────────────────────────────────

const EMPTY: AppConfig = {
  control: 'local',
  ssh_host: '', ssh_user: '', ssh_key: '',
  db_host: '127.0.0.1', db_port: 15432, db_user: 'dune',
  db_pass: '', db_name: 'dune', db_schema: 'dune',
  control_namespace: '',
  docker_gameserver: '', docker_broker_game: '', docker_broker_admin: '', docker_db: '',
  cmd_start: '', cmd_stop: '', cmd_restart: '', cmd_status: '',
  broker_game_addr: '', broker_admin_addr: '', broker_tls: false,
  broker_user: '', broker_pass: '', broker_jwt_secret: '', broker_exec_prefix: '',
  backup_dir: '', server_ini_dir: '', default_ini_dir: '',
  amp_instance: '', amp_container: '', amp_user: 'amp', amp_log_path: '',
  amp_use_container: true, amp_data_root: '',
  director_url: '',
  market_bot_enabled: false,
  market_bot_cache_db: '', market_bot_item_data: '', market_bot_state: '',
  market_bot_buy_interval: '', market_bot_list_interval: '',
  market_bot_buy_threshold: 0, market_bot_max_buys: 0,
  market_bot_remote_url: '', market_bot_remote_token: '',
  listen_addr: ':8080', scrip_currency: 1,
}

// ── search helper ─────────────────────────────────────────────────────────────

function matches(query: string, ...terms: string[]): boolean {
  if (!query) return true
  const q = query.toLowerCase()
  return terms.some(t => t.toLowerCase().includes(q))
}

// ── field components ──────────────────────────────────────────────────────────

function Field({
  label, value, onChange, placeholder, type = 'text', hint, hidden,
}: {
  label: string
  value: string | number
  onChange: (v: string) => void
  placeholder?: string
  type?: string
  hint?: string
  hidden?: boolean
}) {
  if (hidden) return null
  return (
    <div className="flex flex-col gap-1">
      <label className="text-xs text-muted font-medium uppercase tracking-wide">{label}</label>
      <input
        type={type}
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
        className="bg-surface border border-border rounded px-3 py-1.5 text-sm text-foreground placeholder:text-muted/50 focus:outline-none focus:border-accent/60 font-mono"
      />
      {hint && <span className="text-xs text-muted">{hint}</span>}
    </div>
  )
}

function CheckField({
  label, checked, onChange, hint, hidden,
}: {
  label: string
  checked: boolean
  onChange: (v: boolean) => void
  hint?: string
  hidden?: boolean
}) {
  if (hidden) return null
  return (
    <div className="flex flex-col gap-1">
      <label className="flex items-center gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={checked}
          onChange={e => onChange(e.target.checked)}
          className="accent-[var(--color-accent)] w-4 h-4"
        />
        <span className="text-sm text-foreground">{label}</span>
      </label>
      {hint && <span className="text-xs text-muted">{hint}</span>}
    </div>
  )
}

function Section({ title, hidden, children }: {
  title: string
  hidden?: boolean
  children: React.ReactNode
}) {
  if (hidden) return null
  return (
    <div className="flex flex-col gap-3">
      <SectionDivider title={title} />
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {children}
      </div>
    </div>
  )
}

// ── main component ────────────────────────────────────────────────────────────

export default function SettingsConfigForm() {
  const [cfg, setCfg] = useState<AppConfig>(EMPTY)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [query, setQuery] = useState('')

  useEffect(() => {
    api.config.get()
      .then(c => setCfg({ ...EMPTY, ...c }))
      .catch(() => toast.danger('Could not load config'))
      .finally(() => setLoading(false))
  }, [])

  const set = (key: keyof AppConfig) => (v: string) =>
    setCfg(prev => ({
      ...prev,
      [key]: (key === 'db_port' || key === 'scrip_currency' || key === 'market_bot_max_buys')
        ? (Number(v) || 0)
        : key === 'market_bot_buy_threshold'
          ? (parseFloat(v) || 0)
          : v,
    }))

  const save = async () => {
    setSaving(true)
    try {
      await api.config.save(cfg)
      toast.success('Config saved — reconnecting…')
    } catch (e: unknown) {
      toast.danger(`Save failed: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8 gap-2 text-muted">
        <Spinner size="sm" color="current" />
        <span className="text-sm">Loading config…</span>
      </div>
    )
  }

  const q = query
  const isKubectl = cfg.control === 'kubectl'
  const isDocker  = cfg.control === 'docker'
  const isLocal   = cfg.control === 'local'
  const isAmp     = cfg.control === 'amp'

  const showDB = matches(q,
    'Host', 'db_host', 'PostgreSQL host',
    'Port', 'db_port', 'PostgreSQL port',
    'User', 'db_user', 'Database user',
    'Password', 'db_pass', 'Database password',
    'Database name', 'db_name',
    'Schema', 'db_schema', 'schema prefix',
    'database', 'postgres',
  )
  const showSSH = matches(q,
    'SSH', 'ssh_host', 'Host', 'tunnelling',
    'ssh_user', 'ssh_key', 'Private key',
    'Leave blank for local',
  )
  const showControl = matches(q,
    'Control', 'control', 'kubectl', 'docker', 'local', 'amp',
    'Kubernetes', 'Namespace', 'control_namespace',
    'docker_gameserver', 'Game server container',
    'docker_broker_game', 'docker_broker_admin', 'docker_db',
    'cmd_start', 'cmd_stop', 'cmd_restart', 'cmd_status',
    'Start', 'Stop', 'Restart', 'Status', 'shell command',
    'amp_instance', 'amp_container', 'amp_user', 'amp_log_path',
    'amp_use_container', 'amp_data_root',
    'ampinstmgr', 'podman', 'AMP', 'CubeCoders',
  )
  const showBroker = matches(q,
    'Broker', 'RabbitMQ', 'broker_game_addr', 'broker_admin_addr', 'broker_tls',
    'broker_user', 'broker_pass', 'broker_jwt_secret', 'broker_exec_prefix',
    'Game addr', 'Admin addr', 'TLS', 'JWT', 'Exec prefix',
    'capture', 'notification',
  )
  const showMarketBot = matches(q,
    'Market', 'Bot', 'market_bot',
    'Buy interval', 'List interval', 'Threshold', 'Max buys',
    'Cache DB', 'Remote URL', 'Remote token', 'in-process',
  )
  const showAdvanced = matches(q,
    'Advanced', 'Listen', 'listen_addr', 'Backup', 'backup_dir',
    'server_ini_dir', 'Server INI', 'default_ini_dir', 'Default INI',
    'director_url', 'Director', 'scrip_currency', 'Scrip',
    'restart to take effect',
  )

  return (
    <div className="flex flex-col gap-4">
      {/* Search */}
      <SearchField value={query} onChange={setQuery} aria-label="Search settings">
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input placeholder="Search fields…" />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>

      <Panel>
        <div className="flex flex-col gap-5 p-4">

          {/* Database */}
          <Section title="Database" hidden={!showDB}>
            <Field label="Host" value={cfg.db_host} onChange={set('db_host')}
              placeholder="127.0.0.1"
              hint="PostgreSQL host the game database is running on"
              hidden={!matches(q, 'Host', 'db_host', 'PostgreSQL host')} />
            <Field label="Port" value={cfg.db_port} onChange={set('db_port')}
              type="number" placeholder="15432"
              hint="PostgreSQL port (15432 for AMP module, 5432 otherwise)"
              hidden={!matches(q, 'Port', 'db_port', 'PostgreSQL port')} />
            <Field label="User" value={cfg.db_user} onChange={set('db_user')}
              placeholder="dune"
              hint="Database user"
              hidden={!matches(q, 'User', 'db_user', 'Database user')} />
            <Field label="Password" value={cfg.db_pass} onChange={set('db_pass')}
              type="password" placeholder={MASKED}
              hint="Send the placeholder to keep the existing value unchanged"
              hidden={!matches(q, 'Password', 'db_pass', 'Database password')} />
            <Field label="Database name" value={cfg.db_name} onChange={set('db_name')}
              placeholder="dune"
              hint="PostgreSQL database name"
              hidden={!matches(q, 'Database name', 'db_name')} />
            <Field label="Schema" value={cfg.db_schema} onChange={set('db_schema')}
              placeholder="dune"
              hint="Postgres schema prefix — all game tables live here (typically dune)"
              hidden={!matches(q, 'Schema', 'db_schema', 'schema prefix')} />
          </Section>

          {/* SSH */}
          <Section title="SSH" hidden={!showSSH}>
            <Field label="Host : port" value={cfg.ssh_host} onChange={set('ssh_host')}
              placeholder="192.168.0.72:22"
              hint="SSH host (and optional :port). Leave blank for local operation — filling this tunnels all DB connections and executor commands through SSH"
              hidden={!matches(q, 'Host', 'ssh_host', 'SSH host', 'tunnelling')} />
            <Field label="User" value={cfg.ssh_user} onChange={set('ssh_user')}
              placeholder="dune"
              hint="SSH user on the remote host"
              hidden={!matches(q, 'User', 'ssh_user', 'SSH user')} />
            <Field label="Private key path" value={cfg.ssh_key} onChange={set('ssh_key')}
              placeholder="~/.ssh/id_ed25519"
              hint="Absolute path to the SSH private key file. Leave blank for auto-detection next to the binary"
              hidden={!matches(q, 'Private key', 'ssh_key', 'key path')} />
          </Section>

          {/* Control Plane */}
          <Section title="Control Plane" hidden={!showControl}>
            {matches(q, 'Control', 'control', 'kubectl', 'docker', 'local', 'amp', 'CubeCoders') && (
              <div className="flex flex-col gap-1 sm:col-span-2">
                <label className="text-xs text-muted font-medium uppercase tracking-wide">Control plane</label>
                <Select
                  selectedKey={cfg.control || 'local'}
                  onSelectionChange={k => setCfg(prev => ({ ...prev, control: String(k) }))}
                  className="w-64"
                >
                  <Select.Trigger><Select.Value /><Select.Indicator /></Select.Trigger>
                  <Select.Popover>
                    <ListBox>
                      <ListBox.Item id="kubectl" textValue="kubectl">kubectl — Kubernetes / k3s<ListBox.ItemIndicator /></ListBox.Item>
                      <ListBox.Item id="docker" textValue="docker">docker — Docker containers<ListBox.ItemIndicator /></ListBox.Item>
                      <ListBox.Item id="local" textValue="local">local — bare metal / LGSM / shell commands<ListBox.ItemIndicator /></ListBox.Item>
                      <ListBox.Item id="amp" textValue="amp">amp — CubeCoders AMP<ListBox.ItemIndicator /></ListBox.Item>
                    </ListBox>
                  </Select.Popover>
                </Select>
                <span className="text-xs text-muted">How dune-admin manages the game server process and broker</span>
              </div>
            )}

            {/* kubectl */}
            {isKubectl && (
              <Field label="Namespace" value={cfg.control_namespace} onChange={set('control_namespace')}
                placeholder="auto-discovered"
                hint="Kubernetes namespace where the Dune workloads run. Leave blank to auto-discover"
                hidden={!matches(q, 'Namespace', 'control_namespace', 'Kubernetes')} />
            )}

            {/* docker */}
            {isDocker && (<>
              <Field label="Game server container" value={cfg.docker_gameserver} onChange={set('docker_gameserver')}
                placeholder="dune-gameserver"
                hint="Container name for the game server process"
                hidden={!matches(q, 'Game server', 'docker_gameserver', 'container')} />
              <Field label="Broker (game) container" value={cfg.docker_broker_game} onChange={set('docker_broker_game')}
                placeholder="dune-mq-game"
                hint="Container name for the game-vhost RabbitMQ broker"
                hidden={!matches(q, 'Broker', 'docker_broker_game', 'container')} />
              <Field label="Broker (admin) container" value={cfg.docker_broker_admin} onChange={set('docker_broker_admin')}
                placeholder="dune-mq-admin"
                hint="Container name for the admin-vhost RabbitMQ broker"
                hidden={!matches(q, 'Broker', 'docker_broker_admin', 'container')} />
              <Field label="Database container" value={cfg.docker_db} onChange={set('docker_db')}
                placeholder="dune-postgres"
                hint="Container name for the PostgreSQL instance"
                hidden={!matches(q, 'Database', 'docker_db', 'postgres', 'container')} />
            </>)}

            {/* local shell commands */}
            {isLocal && (<>
              <Field label="Start command" value={cfg.cmd_start} onChange={set('cmd_start')}
                placeholder="ampinstmgr start DuneAwakening01"
                hint="Shell command to start the game server"
                hidden={!matches(q, 'Start', 'cmd_start', 'shell command')} />
              <Field label="Stop command" value={cfg.cmd_stop} onChange={set('cmd_stop')}
                placeholder="ampinstmgr stop DuneAwakening01"
                hint="Shell command to stop the game server"
                hidden={!matches(q, 'Stop', 'cmd_stop', 'shell command')} />
              <Field label="Restart command" value={cfg.cmd_restart} onChange={set('cmd_restart')}
                placeholder="ampinstmgr restart DuneAwakening01"
                hint="Shell command to restart the game server"
                hidden={!matches(q, 'Restart', 'cmd_restart', 'shell command')} />
              <Field label="Status command" value={cfg.cmd_status} onChange={set('cmd_status')}
                placeholder="ampinstmgr status DuneAwakening01"
                hint="Shell command to query game server status"
                hidden={!matches(q, 'Status', 'cmd_status', 'shell command')} />
            </>)}

            {/* AMP */}
            {isAmp && (<>
              <Field label="Instance name" value={cfg.amp_instance} onChange={set('amp_instance')}
                placeholder="DuneAwakening01"
                hint="AMP instance name used with ampinstmgr, e.g. DuneAwakening01"
                hidden={!matches(q, 'Instance', 'amp_instance', 'ampinstmgr')} />
              <Field label="Container name" value={cfg.amp_container} onChange={set('amp_container')}
                placeholder="AMP_DuneAwakening01"
                hint="Podman container name (default: AMP_<instance>)"
                hidden={!matches(q, 'Container', 'amp_container', 'podman')} />
              <Field label="AMP user" value={cfg.amp_user} onChange={set('amp_user')}
                placeholder="amp"
                hint="OS user that runs AMP — used for sudo elevation and podman exec"
                hidden={!matches(q, 'AMP user', 'amp_user')} />
              <Field label="Log path" value={cfg.amp_log_path} onChange={set('amp_log_path')}
                placeholder="/AMP/duneawakening/logs"
                hint="In-container log directory (used for log streaming)"
                hidden={!matches(q, 'Log path', 'amp_log_path')} />
              <CheckField label="Use container (podman exec)" checked={cfg.amp_use_container}
                onChange={v => setCfg(prev => ({ ...prev, amp_use_container: v }))}
                hint="Enabled: wraps commands in podman exec. Disabled: runs as the AMP user on the host directly"
                hidden={!matches(q, 'Use container', 'amp_use_container', 'podman exec')} />
              <Field label="Data root" value={cfg.amp_data_root} onChange={set('amp_data_root')}
                placeholder="/AMP/duneawakening"
                hint="Per-game data root inside the container (default /AMP/duneawakening — the CubeCoders convention)"
                hidden={!matches(q, 'Data root', 'amp_data_root')} />
            </>)}
          </Section>

          {/* Broker */}
          <Section title="RabbitMQ Broker (optional)" hidden={!showBroker}>
            <Field label="Game addr" value={cfg.broker_game_addr} onChange={set('broker_game_addr')}
              placeholder="10.43.48.246:5672"
              hint="RabbitMQ management address for the game vhost — enables capture and notifications"
              hidden={!matches(q, 'Game addr', 'broker_game_addr', 'RabbitMQ')} />
            <Field label="Admin addr" value={cfg.broker_admin_addr} onChange={set('broker_admin_addr')}
              placeholder="10.43.189.193:5672"
              hint="RabbitMQ management address for the admin vhost"
              hidden={!matches(q, 'Admin addr', 'broker_admin_addr', 'RabbitMQ')} />
            <Field label="User" value={cfg.broker_user} onChange={set('broker_user')}
              placeholder="dune_cap"
              hint="RabbitMQ user for both vhosts"
              hidden={!matches(q, 'broker_user', 'RabbitMQ user', 'Broker user')} />
            <Field label="Password" value={cfg.broker_pass} onChange={set('broker_pass')}
              type="password" placeholder={MASKED}
              hint="RabbitMQ password — send the placeholder to keep the existing value"
              hidden={!matches(q, 'broker_pass', 'Password', 'RabbitMQ password')} />
            <Field label="JWT secret" value={cfg.broker_jwt_secret} onChange={set('broker_jwt_secret')}
              type="password" placeholder={MASKED}
              hint="Base64-encoded HMAC key for re-signing ServiceAuthTokens. Leave blank to use the built-in default"
              hidden={!matches(q, 'JWT', 'broker_jwt_secret', 'HMAC', 'ServiceAuthToken')} />
            <CheckField label="Use TLS (amqps://)" checked={cfg.broker_tls}
              onChange={v => setCfg(prev => ({ ...prev, broker_tls: v }))}
              hint="Enable TLS for all broker connections"
              hidden={!matches(q, 'TLS', 'broker_tls', 'amqps')} />
            <Field label="Exec prefix" value={cfg.broker_exec_prefix} onChange={set('broker_exec_prefix')}
              placeholder="podman exec AMP_DuneAwakening01"
              hint="Prepended to all rabbitmqctl calls — use when the broker runs inside a container not managed by the docker control plane"
              hidden={!matches(q, 'Exec prefix', 'broker_exec_prefix', 'rabbitmqctl')} />
          </Section>

          {/* Market Bot */}
          <Section title="Market Bot" hidden={!showMarketBot}>
            <CheckField label="Enable embedded bot" checked={cfg.market_bot_enabled}
              onChange={v => setCfg(prev => ({ ...prev, market_bot_enabled: v }))}
              hint="Run the market bot in-process alongside dune-admin (restart required to take effect)"
              hidden={!matches(q, 'market_bot_enabled', 'Enable', 'embedded bot')} />
            <Field label="Remote URL" value={cfg.market_bot_remote_url} onChange={set('market_bot_remote_url')}
              placeholder="http://192.168.0.10:9191"
              hint="Forward market bot API calls to a standalone bot at this URL instead of running one in-process"
              hidden={!matches(q, 'Remote URL', 'market_bot_remote_url', 'standalone')} />
            <Field label="Remote token" value={cfg.market_bot_remote_token} onChange={set('market_bot_remote_token')}
              type="password" placeholder={MASKED}
              hint="Bearer token for authenticating with the remote bot"
              hidden={!matches(q, 'Remote token', 'market_bot_remote_token', 'bearer')} />
            <Field label="Cache DB path" value={cfg.market_bot_cache_db} onChange={set('market_bot_cache_db')}
              placeholder="~/.dune-admin/market-bot-cache.db"
              hint="Path to the SQLite cache database used by the embedded bot"
              hidden={!matches(q, 'Cache DB', 'market_bot_cache_db', 'SQLite')} />
            <Field label="Item data path" value={cfg.market_bot_item_data} onChange={set('market_bot_item_data')}
              placeholder="item-data.json"
              hint="Path to item-data.json — the bot uses this for price lookups"
              hidden={!matches(q, 'Item data', 'market_bot_item_data')} />
            <Field label="State path" value={cfg.market_bot_state} onChange={set('market_bot_state')}
              placeholder="~/.dune-admin/market-bot-state.json"
              hint="Path to the JSON file where the bot persists its runtime state across restarts"
              hidden={!matches(q, 'State path', 'market_bot_state')} />
            <Field label="Buy interval" value={cfg.market_bot_buy_interval} onChange={set('market_bot_buy_interval')}
              placeholder="5m"
              hint="How often the bot checks for buy opportunities (e.g. 5m, 30s)"
              hidden={!matches(q, 'Buy interval', 'market_bot_buy_interval')} />
            <Field label="List interval" value={cfg.market_bot_list_interval} onChange={set('market_bot_list_interval')}
              placeholder="10m"
              hint="How often the bot refreshes its listings"
              hidden={!matches(q, 'List interval', 'market_bot_list_interval')} />
            <Field label="Buy threshold" value={cfg.market_bot_buy_threshold} onChange={set('market_bot_buy_threshold')}
              type="number" placeholder="0.8"
              hint="Minimum discount ratio (0–1) before the bot buys — e.g. 0.8 = buy at 80% of market price or lower"
              hidden={!matches(q, 'Buy threshold', 'market_bot_buy_threshold', 'discount')} />
            <Field label="Max buys" value={cfg.market_bot_max_buys} onChange={set('market_bot_max_buys')}
              type="number" placeholder="10"
              hint="Maximum concurrent buy orders the bot will place"
              hidden={!matches(q, 'Max buys', 'market_bot_max_buys')} />
          </Section>

          {/* Advanced */}
          <Section title="Advanced" hidden={!showAdvanced}>
            <Field label="Listen address ⚠" value={cfg.listen_addr} onChange={set('listen_addr')}
              placeholder=":8080"
              hint="HTTP listen address — changing this requires a full server restart to take effect"
              hidden={!matches(q, 'Listen', 'listen_addr', 'HTTP', 'restart to take effect')} />
            <Field label="Backup directory" value={cfg.backup_dir} onChange={set('backup_dir')}
              placeholder="/funcom/artifacts/database-dumps/mybg"
              hint="Path the executor accesses for game backup files (read over SSH when in SSH mode)"
              hidden={!matches(q, 'Backup', 'backup_dir')} />
            <Field label="Server INI directory" value={cfg.server_ini_dir} onChange={set('server_ini_dir')}
              placeholder="/home/amp/.ampdata/instances/DuneAwakening01/duneawakening/server/state"
              hint="Directory containing UserGame.ini and UserOverrides.ini — the Server Settings tab writes here"
              hidden={!matches(q, 'Server INI', 'server_ini_dir', 'UserGame.ini')} />
            <Field label="Default INI directory" value={cfg.default_ini_dir} onChange={set('default_ini_dir')}
              placeholder="/path/to/game/Config"
              hint="Path to DefaultGame.ini / DefaultEngine.ini — the base layer read by the Server Settings tab"
              hidden={!matches(q, 'Default INI', 'default_ini_dir', 'DefaultGame.ini')} />
            <Field label="Director URL" value={cfg.director_url} onChange={set('director_url')}
              placeholder="http://127.0.0.1:11717"
              hint="Optional Battlegroup Director URL — when set, dune-admin proxies /director/ to it"
              hidden={!matches(q, 'Director', 'director_url', 'Battlegroup')} />
            <Field label="Scrip currency ID" value={cfg.scrip_currency} onChange={set('scrip_currency')}
              type="number" placeholder="1"
              hint="Item ID used as the scrip currency in the game economy"
              hidden={!matches(q, 'Scrip', 'scrip_currency', 'currency')} />
          </Section>

        </div>
      </Panel>

      {/* Save */}
      <div className="flex justify-end">
        <Button onPress={save} isDisabled={saving} size="sm">
          {saving
            ? <><Spinner size="sm" color="current" /> Reconnecting…</>
            : <><Icon name="save" /> Save &amp; Reconnect</>}
        </Button>
      </div>
    </div>
  )
}
