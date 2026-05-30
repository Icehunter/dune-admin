import { useState, useEffect, type MutableRefObject } from 'react'
import { Button, Select, ListBox, Spinner, Tabs, toast } from '@heroui/react'
import { api, MASKED } from '../api/client'
import type { AppConfig } from '../api/client'
import { Panel, SectionLabel } from '../dune-ui'

// ── defaults (all empty — never show fake values) ─────────────────────────────

const EMPTY: AppConfig = {
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
  director_url: '',
  market_bot_enabled: false,
  market_bot_cache_db: '', market_bot_item_data: '', market_bot_state: '',
  market_bot_buy_interval: '', market_bot_list_interval: '',
  market_bot_buy_threshold: 0, market_bot_max_buys: 0,
  market_bot_remote_url: '', market_bot_remote_token: '',
  listen_addr: '', scrip_currency: 0,
}

// Pointer-backed boolean fields in the Go config: null means "use server
// default" (effectively true). If the API returns null for these, coerce to
// true so the checkbox reflects the real server default rather than silently
// inheriting EMPTY's false and overwriting the default-on value on save.
const pointerBoolFields = new Set<keyof AppConfig>(['amp_use_container', 'market_bot_enabled'])

function mergeConfig(fetched: Record<string, unknown>): AppConfig {
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

// ── field primitives matching BotConfigEditor ─────────────────────────────────

const inputCls = 'bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full font-mono placeholder:text-muted/50 focus:outline-none focus:border-accent/60'

function F({ label, hint, children }: { label: string, hint?: string, children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <label className="text-xs text-muted font-medium">
        {label}
        {hint && (
          <span className="opacity-60 font-normal">
            {' '}
            (
            {hint}
            )
          </span>
        )}
      </label>
      {children}
    </div>
  )
}

interface TIProps {
  value: string | number
  onChange: (v: string) => void
  placeholder?: string
  type?: string
}

function TI({ value, onChange, placeholder, type = 'text' }: TIProps) {
  return (
    <input
      type={type}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className={inputCls}
    />
  )
}

interface CBProps {
  label: string
  checked: boolean
  onChange: (v: boolean) => void
  hint?: string
}

function CB({ label, checked, onChange, hint }: CBProps) {
  return (
    <div className="flex flex-col gap-0.5">
      <label className="flex items-center gap-2 cursor-pointer select-none text-sm text-foreground">
        <input
          type="checkbox"
          checked={!!checked}
          onChange={(e) => onChange(e.target.checked)}
          className="accent-[var(--color-accent)] w-4 h-4 cursor-pointer"
        />
        {label}
      </label>
      {hint && <p className="text-xs text-muted ml-6">{hint}</p>}
    </div>
  )
}

function G2({ children }: { children: React.ReactNode }) {
  return <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-1">{children}</div>
}

// ── main component ────────────────────────────────────────────────────────────

interface Props {
  saveRef?: MutableRefObject<(() => Promise<void>) | null>
  onSavingChange?: (saving: boolean) => void
}

export default function SettingsConfigForm({ saveRef, onSavingChange }: Props) {
  const [cfg, setCfg] = useState<AppConfig>(EMPTY)
  const [loading, setLoading] = useState(true)
  const [tab, setTab] = useState('connection')
  const [backendUrl, setBackendUrl] = useState(() => localStorage.getItem('dune_admin_backend') || '')

  useEffect(() => {
    api.config.get()
      .then((c) => setCfg(mergeConfig(c as Record<string, unknown>)))
      .catch((e) => toast.danger(`Could not load config: ${e instanceof Error ? e.message : String(e)}`))
      .finally(() => setLoading(false))
  }, [])

  const set = (key: keyof AppConfig) => (v: string) =>
    setCfg((prev) => ({
      ...prev,
      [key]: key === 'db_port' || key === 'scrip_currency' || key === 'market_bot_max_buys'
        ? (Number(v) || 0)
        : key === 'market_bot_buy_threshold'
          ? (parseFloat(v) || 0)
          : v,
    }))

  const setBool = (key: keyof AppConfig) => (v: boolean) =>
    setCfg((prev) => ({ ...prev, [key]: v }))

  const save = async () => {
    onSavingChange?.(true)
    try {
      await api.config.save(cfg)
      toast.success('Config saved — applying settings…')
    }
    catch (e: unknown) {
      toast.danger(`Save failed: ${e instanceof Error ? e.message : String(e)}`)
    }
    finally {
      onSavingChange?.(false)
    }
  }

  // Keep the ref current so App.tsx footer button always calls the latest closure.
  useEffect(() => {
    if (saveRef) saveRef.current = save
  })

  if (loading) {
    return (
      <div className="flex items-center justify-center flex-1 gap-2 text-muted">
        <Spinner size="sm" color="current" />
        <span className="text-sm">Loading config…</span>
      </div>
    )
  }

  const isKubectl = cfg.control === 'kubectl'
  const isDocker = cfg.control === 'docker'
  const isLocal = cfg.control === 'local'
  const isAmp = cfg.control === 'amp'

  return (
    // Outer flex col: tabs + single save bar below
    <div className="flex flex-col flex-1 min-h-0 gap-0">
      <Tabs
        selectedKey={tab}
        onSelectionChange={(k) => setTab(String(k))}
        className="flex flex-col flex-1 min-h-0"
      >
        {/* Tab bar — never scrolls */}
        <Tabs.ListContainer className="shrink-0">
          <Tabs.List aria-label="Config sections" className="gap-1">
            <Tabs.Tab id="connection">
              Connection
              <Tabs.Indicator />
            </Tabs.Tab>
            <Tabs.Tab id="control">
              Control
              <Tabs.Indicator />
            </Tabs.Tab>
            <Tabs.Tab id="broker">
              Broker
              <Tabs.Indicator />
            </Tabs.Tab>
            <Tabs.Tab id="marketbot">
              Market Bot
              <Tabs.Indicator />
            </Tabs.Tab>
            <Tabs.Tab id="advanced">
              Advanced
              <Tabs.Indicator />
            </Tabs.Tab>
          </Tabs.List>
        </Tabs.ListContainer>

        {/* ── Connection ─────────────────────────────────────────────────── */}
        <Tabs.Panel id="connection" className="pt-4 overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <Panel>
            <SectionLabel>Database</SectionLabel>
            <G2>
              <F label="Host" hint="127.0.0.1 over SSH tunnel">
                <TI value={cfg.db_host} onChange={set('db_host')} placeholder="127.0.0.1" />
              </F>
              <F label="Port">
                <TI value={cfg.db_port || ''} onChange={set('db_port')} type="number" placeholder="15432" />
              </F>
              <F label="User">
                <TI value={cfg.db_user} onChange={set('db_user')} placeholder="dune" />
              </F>
              <F label="Password" hint="send placeholder to keep existing">
                <TI value={cfg.db_pass} onChange={set('db_pass')} type="password" placeholder={MASKED} />
              </F>
              <F label="Database name">
                <TI value={cfg.db_name} onChange={set('db_name')} placeholder="dune" />
              </F>
              <F label="Schema">
                <TI value={cfg.db_schema} onChange={set('db_schema')} placeholder="dune" />
              </F>
            </G2>
          </Panel>

          <Panel>
            <SectionLabel>SSH (leave blank for local)</SectionLabel>
            <G2>
              <F label="Host : port" hint="enables SSH tunnel for all DB + exec">
                <TI value={cfg.ssh_host} onChange={set('ssh_host')} placeholder="192.168.0.72:22" />
              </F>
              <F label="User">
                <TI value={cfg.ssh_user} onChange={set('ssh_user')} placeholder="dune" />
              </F>
              <F label="Private key path" hint="absolute path, blank = auto-detect">
                <TI value={cfg.ssh_key} onChange={set('ssh_key')} placeholder="~/.ssh/id_ed25519" />
              </F>
            </G2>
          </Panel>
        </Tabs.Panel>

        {/* ── Control ────────────────────────────────────────────────────── */}
        <Tabs.Panel id="control" className="pt-4 overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <Panel>
            <SectionLabel>Control Plane</SectionLabel>
            <div className="mt-1 flex flex-col gap-1">
              <Select
                selectedKey={cfg.control || 'local'}
                onSelectionChange={(k) => setCfg((prev) => ({ ...prev, control: String(k) }))}
                className="w-64"
              >
                <Select.Trigger>
                  <Select.Value />
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    <ListBox.Item id="kubectl" textValue="kubectl">
                      kubectl — Kubernetes / k3s
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                    <ListBox.Item id="docker" textValue="docker">
                      docker — Docker containers
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                    <ListBox.Item id="local" textValue="local">
                      local — bare metal / LGSM / shell
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                    <ListBox.Item id="amp" textValue="amp">
                      amp — CubeCoders AMP
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  </ListBox>
                </Select.Popover>
              </Select>
              <p className="text-xs text-muted">How dune-admin manages the game server</p>
            </div>
          </Panel>

          {isKubectl && (
            <Panel>
              <SectionLabel>Kubernetes</SectionLabel>
              <G2>
                <F label="Namespace" hint="blank = auto-discover">
                  <TI value={cfg.control_namespace} onChange={set('control_namespace')} placeholder="my-namespace" />
                </F>
              </G2>
            </Panel>
          )}

          {isDocker && (
            <Panel>
              <SectionLabel>Docker containers</SectionLabel>
              <G2>
                <F label="Game server"><TI value={cfg.docker_gameserver} onChange={set('docker_gameserver')} placeholder="dune-gameserver" /></F>
                <F label="Broker (game)"><TI value={cfg.docker_broker_game} onChange={set('docker_broker_game')} placeholder="dune-mq-game" /></F>
                <F label="Broker (admin)"><TI value={cfg.docker_broker_admin} onChange={set('docker_broker_admin')} placeholder="dune-mq-admin" /></F>
                <F label="Database"><TI value={cfg.docker_db} onChange={set('docker_db')} placeholder="dune-postgres" /></F>
              </G2>
            </Panel>
          )}

          {isLocal && (
            <Panel>
              <SectionLabel>Server commands</SectionLabel>
              <G2>
                <F label="Start"><TI value={cfg.cmd_start} onChange={set('cmd_start')} placeholder="service dune start" /></F>
                <F label="Stop"><TI value={cfg.cmd_stop} onChange={set('cmd_stop')} placeholder="service dune stop" /></F>
                <F label="Restart"><TI value={cfg.cmd_restart} onChange={set('cmd_restart')} placeholder="service dune restart" /></F>
                <F label="Status"><TI value={cfg.cmd_status} onChange={set('cmd_status')} placeholder="service dune status" /></F>
              </G2>
            </Panel>
          )}

          {isAmp && (
            <Panel>
              <SectionLabel>CubeCoders AMP</SectionLabel>
              <G2>
                <F label="Instance name"><TI value={cfg.amp_instance} onChange={set('amp_instance')} placeholder="DuneAwakening01" /></F>
                <F label="Container name" hint="default: AMP_<instance>"><TI value={cfg.amp_container} onChange={set('amp_container')} placeholder="AMP_DuneAwakening01" /></F>
                <F label="AMP user"><TI value={cfg.amp_user} onChange={set('amp_user')} placeholder="amp" /></F>
                <F label="Log path"><TI value={cfg.amp_log_path} onChange={set('amp_log_path')} placeholder="/logs" /></F>
                <F label="Data root"><TI value={cfg.amp_data_root} onChange={set('amp_data_root')} placeholder="/AMP/duneawakening" /></F>
                <CB
                  label="Use container (podman exec)"
                  checked={cfg.amp_use_container}
                  onChange={setBool('amp_use_container')}
                  hint="Disable to run on host as the AMP user directly."
                />
              </G2>
            </Panel>
          )}

          {!isKubectl && !isDocker && !isLocal && !isAmp && (
            <p className="text-xs text-muted pt-2">Select a control mode above to see mode-specific settings.</p>
          )}
        </Tabs.Panel>

        {/* ── Broker ─────────────────────────────────────────────────────── */}
        <Tabs.Panel id="broker" className="pt-4 overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <Panel>
            <SectionLabel>RabbitMQ (optional)</SectionLabel>
            <p className="text-xs text-muted -mt-1">Enables capture and notifications when configured.</p>
            <G2>
              <F label="Game addr"><TI value={cfg.broker_game_addr} onChange={set('broker_game_addr')} placeholder="10.x.x.x:5672" /></F>
              <F label="Admin addr"><TI value={cfg.broker_admin_addr} onChange={set('broker_admin_addr')} placeholder="10.x.x.x:5672" /></F>
              <F label="User"><TI value={cfg.broker_user} onChange={set('broker_user')} placeholder="dune_cap" /></F>
              <F label="Password"><TI value={cfg.broker_pass} onChange={set('broker_pass')} type="password" placeholder={MASKED} /></F>
              <F label="JWT secret" hint="blank = built-in default">
                <TI value={cfg.broker_jwt_secret} onChange={set('broker_jwt_secret')} type="password" placeholder={MASKED} />
              </F>
              <F label="Exec prefix" hint="when broker is inside a container">
                <TI value={cfg.broker_exec_prefix} onChange={set('broker_exec_prefix')} placeholder="podman exec <container>" />
              </F>
              <div className="sm:col-span-2">
                <CB label="Use TLS (amqps://)" checked={cfg.broker_tls} onChange={setBool('broker_tls')} />
              </div>
            </G2>
          </Panel>
        </Tabs.Panel>

        {/* ── Market Bot ─────────────────────────────────────────────────── */}
        <Tabs.Panel id="marketbot" className="pt-4 overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <Panel>
            <SectionLabel>Mode</SectionLabel>
            <div className="mt-1">
              <CB
                label="Enable embedded bot"
                checked={cfg.market_bot_enabled}
                onChange={setBool('market_bot_enabled')}
                hint="Runs in-process alongside dune-admin. Toggling off stops it immediately on save."
              />
            </div>
            <G2>
              <F label="Remote URL" hint="forward to standalone bot instead">
                <TI value={cfg.market_bot_remote_url} onChange={set('market_bot_remote_url')} placeholder="http://host:9191" />
              </F>
              <F label="Remote token">
                <TI value={cfg.market_bot_remote_token} onChange={set('market_bot_remote_token')} type="password" placeholder={MASKED} />
              </F>
            </G2>
          </Panel>

          <Panel>
            <SectionLabel>Paths (embedded bot)</SectionLabel>
            <p className="text-xs text-muted -mt-1">Tuning (intervals, threshold) is managed in the Bot Control panel.</p>
            <G2>
              <F label="Cache DB"><TI value={cfg.market_bot_cache_db} onChange={set('market_bot_cache_db')} placeholder="~/.dune-admin/market-bot-cache.db" /></F>
              <F label="Item data"><TI value={cfg.market_bot_item_data} onChange={set('market_bot_item_data')} placeholder="item-data.json" /></F>
              <F label="State path"><TI value={cfg.market_bot_state} onChange={set('market_bot_state')} placeholder="~/.dune-admin/market-bot-state.json" /></F>
            </G2>
          </Panel>
        </Tabs.Panel>

        {/* ── Advanced ───────────────────────────────────────────────────── */}
        <Tabs.Panel id="advanced" className="pt-4 overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <Panel>
            <SectionLabel>Server</SectionLabel>
            <G2>
              <F label="Listen address" hint="⚠ restart required">
                <TI value={cfg.listen_addr} onChange={set('listen_addr')} placeholder=":8080" />
              </F>
              <F label="Scrip currency ID">
                <TI value={cfg.scrip_currency || ''} onChange={set('scrip_currency')} type="number" placeholder="1" />
              </F>
              <F label="Director URL" hint="proxied at /director/">
                <TI value={cfg.director_url} onChange={set('director_url')} placeholder="http://127.0.0.1:11717" />
              </F>
            </G2>
          </Panel>

          <Panel>
            <SectionLabel>Paths</SectionLabel>
            <G2>
              <F label="Backup directory">
                <TI value={cfg.backup_dir} onChange={set('backup_dir')} placeholder="/path/to/backups" />
              </F>
              <F label="Server INI directory" hint="UserGame.ini location">
                <TI value={cfg.server_ini_dir} onChange={set('server_ini_dir')} placeholder="/path/to/server/state" />
              </F>
              <F label="Default INI directory" hint="DefaultGame.ini base layer">
                <TI value={cfg.default_ini_dir} onChange={set('default_ini_dir')} placeholder="/path/to/game/Config" />
              </F>
            </G2>
          </Panel>

          <Panel>
            <SectionLabel>Backend URL Override</SectionLabel>
            <p className="text-xs text-muted -mt-1">
              Only needed when the UI is served from a different host (SSH tunnel, CDN).
              Leave blank for single-binary setup.
            </p>
            <G2>
              <F label="URL" hint="stored in browser, not server">
                <TI
                  value={backendUrl}
                  onChange={(v) => {
                    setBackendUrl(v)
                    localStorage.setItem('dune_admin_backend', v)
                  }}
                  placeholder="http://host:port"
                />
              </F>
            </G2>
            <div className="flex gap-2 mt-1">
              <Button size="sm" onPress={() => window.location.reload()}>Apply & Reload</Button>
              <Button
                size="sm"
                variant="outline"
                onPress={() => {
                  setBackendUrl('')
                  localStorage.removeItem('dune_admin_backend')
                  window.location.reload()
                }}
              >
                Reset
              </Button>
            </div>
          </Panel>
        </Tabs.Panel>
      </Tabs>

    </div>
  )
}
