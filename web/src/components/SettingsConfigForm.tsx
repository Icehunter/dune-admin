import type React from 'react'
import { useState, useEffect, type MutableRefObject } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Input, Select, ListBox, Spinner, Switch, Tabs, toast } from '@heroui/react'
import { api, MASKED } from '../api/client'
import type { AppConfig } from '../api/client'
import { NumberInput, Panel, SectionLabel } from '../dune-ui'

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
  amp_api_user: '', amp_api_pass: '', amp_api_port: 0,
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

interface FieldProps {
  label: string
  hint?: string
  children: React.ReactNode
}

function F({ label, hint, children }: FieldProps) {
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

interface TextInputProps {
  value: string | number
  onChange: (v: string) => void
  placeholder?: string
  type?: string
}

function TI({ value, onChange, placeholder, type = 'text' }: TextInputProps) {
  return (
    <Input
      className="font-mono"
      type={type}
      value={String(value)}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      aria-label={placeholder ?? 'value'}
    />
  )
}

interface CheckboxFieldProps {
  label: string
  checked: boolean
  onChange: (v: boolean) => void
  hint?: string
}

function CB({ label, checked, onChange, hint }: CheckboxFieldProps) {
  return (
    <div className="flex flex-col gap-0.5">
      <Switch isSelected={!!checked} onChange={onChange} size="sm">
        <Switch.Control><Switch.Thumb /></Switch.Control>
        <Switch.Content>{label}</Switch.Content>
      </Switch>
      {hint && <p className="text-xs text-muted ml-6">{hint}</p>}
    </div>
  )
}

interface GridRowProps {
  children: React.ReactNode
}

function G2({ children }: GridRowProps) {
  return <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-1">{children}</div>
}

// ── main component ────────────────────────────────────────────────────────────

interface SettingsConfigFormProps {
  saveRef?: MutableRefObject<(() => Promise<void>) | null>
  onSavingChange?: (saving: boolean) => void
}

export const SettingsConfigForm: React.FC<SettingsConfigFormProps> = ({ saveRef, onSavingChange }) => {
  const { t } = useTranslation()
  const [cfg, setCfg] = useState<AppConfig>(EMPTY)
  const [loading, setLoading] = useState(true)
  const [tab, setTab] = useState('connection')
  const [backendUrl, setBackendUrl] = useState(() => localStorage.getItem('dune_admin_backend') || '')

  useEffect(() => {
    api.config.get()
      .then((c) => setCfg(mergeConfig(c as Record<string, unknown>)))
      .catch((e) => toast.danger(t('settings.loadFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t])

  const set = (key: keyof AppConfig) => (v: string) =>
    setCfg((prev) => ({
      ...prev,
      [key]: key === 'db_port' || key === 'scrip_currency' || key === 'market_bot_max_buys' || key === 'amp_api_port'
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
      toast.success(t('settings.configSaved'))
    }
    catch (e: unknown) {
      toast.danger(t('settings.saveFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      onSavingChange?.(false)
    }
  }

  // Expose save to the parent footer button only after config has loaded.
  // Clear the ref on unmount so a stale closure from a previous modal open
  // cannot fire after the form has been removed from the tree.
  useEffect(() => {
    if (saveRef && !loading) {
      saveRef.current = save
      return () => {
        saveRef.current = null
      }
    }
  })

  if (loading) {
    return (
      <div className="flex items-center justify-center flex-1 gap-2 text-muted">
        <Spinner size="sm" color="current" />
        <span className="text-sm">{t('settings.loadingConfig')}</span>
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
              {t('settings.tabs.connection')}
              <Tabs.Indicator />
            </Tabs.Tab>
            <Tabs.Tab id="control">
              {t('settings.tabs.control')}
              <Tabs.Indicator />
            </Tabs.Tab>
            <Tabs.Tab id="broker">
              {t('settings.tabs.broker')}
              <Tabs.Indicator />
            </Tabs.Tab>
            <Tabs.Tab id="advanced">
              {t('settings.tabs.advanced')}
              <Tabs.Indicator />
            </Tabs.Tab>
          </Tabs.List>
        </Tabs.ListContainer>

        {/* ── Connection ─────────────────────────────────────────────────── */}
        <Tabs.Panel id="connection" className="pt-4 overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <Panel>
            <SectionLabel>{t('settings.sections.database')}</SectionLabel>
            <G2>
              <F label={t('settings.db.host')} hint={t('settings.db.hostHint')}>
                <TI value={cfg.db_host} onChange={set('db_host')} placeholder="127.0.0.1" />
              </F>
              <F label={t('settings.db.port')}>
                <NumberInput
                  ariaLabel={t('settings.db.port')}
                  value={Number(cfg.db_port) || 0}
                  onChange={(v) => set('db_port')(String(v))}
                  showButtons={false}
                  className="w-full"
                />
              </F>
              <F label={t('settings.db.user')}>
                <TI value={cfg.db_user} onChange={set('db_user')} placeholder="dune" />
              </F>
              <F label={t('settings.db.password')} hint={t('settings.db.passwordHint')}>
                <TI value={cfg.db_pass} onChange={set('db_pass')} type="password" placeholder={MASKED} />
              </F>
              <F label={t('settings.db.name')}>
                <TI value={cfg.db_name} onChange={set('db_name')} placeholder="dune" />
              </F>
              <F label={t('settings.db.schema')}>
                <TI value={cfg.db_schema} onChange={set('db_schema')} placeholder="dune" />
              </F>
            </G2>
          </Panel>

          <Panel>
            <SectionLabel>{t('settings.sections.ssh')}</SectionLabel>
            <G2>
              <F label={t('settings.ssh.hostPort')} hint={t('settings.ssh.hostPortHint')}>
                <TI value={cfg.ssh_host} onChange={set('ssh_host')} placeholder="192.168.0.72:22" />
              </F>
              <F label={t('settings.ssh.user')}>
                <TI value={cfg.ssh_user} onChange={set('ssh_user')} placeholder="dune" />
              </F>
              <F label={t('settings.ssh.privateKey')} hint={t('settings.ssh.privateKeyHint')}>
                <TI value={cfg.ssh_key} onChange={set('ssh_key')} placeholder="~/.ssh/id_ed25519" />
              </F>
            </G2>
          </Panel>
        </Tabs.Panel>

        {/* ── Control ────────────────────────────────────────────────────── */}
        <Tabs.Panel id="control" className="pt-4 overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <Panel>
            <SectionLabel>{t('settings.sections.controlPlane')}</SectionLabel>
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
                      {t('settings.control.kubectl')}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                    <ListBox.Item id="docker" textValue="docker">
                      {t('settings.control.docker')}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                    <ListBox.Item id="local" textValue="local">
                      {t('settings.control.local')}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                    <ListBox.Item id="amp" textValue="amp">
                      {t('settings.control.amp')}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  </ListBox>
                </Select.Popover>
              </Select>
              <p className="text-xs text-muted">{t('settings.control.modeHint')}</p>
            </div>
          </Panel>

          {isKubectl && (
            <Panel>
              <SectionLabel>{t('settings.sections.kubernetes')}</SectionLabel>
              <G2>
                <F label={t('settings.k8s.namespace')} hint={t('settings.k8s.namespaceHint')}>
                  <TI value={cfg.control_namespace} onChange={set('control_namespace')} placeholder="my-namespace" />
                </F>
              </G2>
            </Panel>
          )}

          {isDocker && (
            <Panel>
              <SectionLabel>{t('settings.sections.dockerContainers')}</SectionLabel>
              <G2>
                <F label={t('settings.docker.gameServer')}><TI value={cfg.docker_gameserver} onChange={set('docker_gameserver')} placeholder="dune-gameserver" /></F>
                <F label={t('settings.docker.brokerGame')}><TI value={cfg.docker_broker_game} onChange={set('docker_broker_game')} placeholder="dune-mq-game" /></F>
                <F label={t('settings.docker.brokerAdmin')}><TI value={cfg.docker_broker_admin} onChange={set('docker_broker_admin')} placeholder="dune-mq-admin" /></F>
                <F label={t('settings.docker.database')}><TI value={cfg.docker_db} onChange={set('docker_db')} placeholder="dune-postgres" /></F>
              </G2>
            </Panel>
          )}

          {isLocal && (
            <Panel>
              <SectionLabel>{t('settings.sections.serverCommands')}</SectionLabel>
              <G2>
                <F label={t('settings.cmd.start')}><TI value={cfg.cmd_start} onChange={set('cmd_start')} placeholder="service dune start" /></F>
                <F label={t('settings.cmd.stop')}><TI value={cfg.cmd_stop} onChange={set('cmd_stop')} placeholder="service dune stop" /></F>
                <F label={t('settings.cmd.restart')}><TI value={cfg.cmd_restart} onChange={set('cmd_restart')} placeholder="service dune restart" /></F>
                <F label={t('settings.cmd.status')}><TI value={cfg.cmd_status} onChange={set('cmd_status')} placeholder="service dune status" /></F>
              </G2>
            </Panel>
          )}

          {isAmp && (
            <Panel>
              <SectionLabel>{t('settings.sections.amp')}</SectionLabel>
              <G2>
                <F label={t('settings.amp.instanceName')}><TI value={cfg.amp_instance} onChange={set('amp_instance')} placeholder="DuneAwakening01" /></F>
                <F label={t('settings.amp.containerName')} hint={t('settings.amp.containerNameHint')}><TI value={cfg.amp_container} onChange={set('amp_container')} placeholder="AMP_DuneAwakening01" /></F>
                <F label={t('settings.amp.user')}><TI value={cfg.amp_user} onChange={set('amp_user')} placeholder="amp" /></F>
                <F label={t('settings.amp.logPath')}><TI value={cfg.amp_log_path} onChange={set('amp_log_path')} placeholder="/logs" /></F>
                <F label={t('settings.amp.dataRoot')}><TI value={cfg.amp_data_root} onChange={set('amp_data_root')} placeholder="/AMP/duneawakening" /></F>
                <CB
                  label={t('settings.amp.useContainer')}
                  checked={cfg.amp_use_container}
                  onChange={setBool('amp_use_container')}
                  hint={t('settings.amp.useContainerHint')}
                />
              </G2>
              <p className="text-xs text-muted mt-3">{t('settings.amp.apiHint')}</p>
              <G2>
                <F label={t('settings.amp.apiUser')}><TI value={cfg.amp_api_user} onChange={set('amp_api_user')} placeholder="admin" /></F>
                <F label={t('settings.amp.apiPassword')}><TI value={cfg.amp_api_pass} onChange={set('amp_api_pass')} type="password" placeholder={MASKED} /></F>
                <F label={t('settings.amp.apiPort')} hint="default 8081">
                  <NumberInput
                    ariaLabel={t('settings.amp.apiPort')}
                    value={Number(cfg.amp_api_port) || 0}
                    onChange={(v) => set('amp_api_port')(String(v))}
                    showButtons={false}
                    className="w-full"
                  />
                </F>
              </G2>
            </Panel>
          )}

          {!isKubectl && !isDocker && !isLocal && !isAmp && (
            <p className="text-xs text-muted pt-2">{t('settings.control.selectMode')}</p>
          )}
        </Tabs.Panel>

        {/* ── Broker ─────────────────────────────────────────────────────── */}
        <Tabs.Panel id="broker" className="pt-4 overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <Panel>
            <SectionLabel>{t('settings.sections.rabbitmq')}</SectionLabel>
            <p className="text-xs text-muted -mt-1">{t('settings.broker.optionalHint')}</p>
            <G2>
              <F label={t('settings.broker.gameAddr')}><TI value={cfg.broker_game_addr} onChange={set('broker_game_addr')} placeholder="10.x.x.x:5672" /></F>
              <F label={t('settings.broker.adminAddr')}><TI value={cfg.broker_admin_addr} onChange={set('broker_admin_addr')} placeholder="10.x.x.x:5672" /></F>
              <F label={t('settings.broker.user')}><TI value={cfg.broker_user} onChange={set('broker_user')} placeholder="dune_cap" /></F>
              <F label={t('settings.broker.password')}><TI value={cfg.broker_pass} onChange={set('broker_pass')} type="password" placeholder={MASKED} /></F>
              <F label={t('settings.broker.jwtSecret')} hint={t('settings.broker.jwtSecretHint')}>
                <TI value={cfg.broker_jwt_secret} onChange={set('broker_jwt_secret')} type="password" placeholder={MASKED} />
              </F>
              <F label={t('settings.broker.execPrefix')} hint={t('settings.broker.execPrefixHint')}>
                <TI value={cfg.broker_exec_prefix} onChange={set('broker_exec_prefix')} placeholder="podman exec <container>" />
              </F>
              <div className="sm:col-span-2">
                <CB label={t('settings.broker.useTls')} checked={cfg.broker_tls} onChange={setBool('broker_tls')} />
              </div>
            </G2>
          </Panel>
        </Tabs.Panel>

        {/* ── Advanced ───────────────────────────────────────────────────── */}
        <Tabs.Panel id="advanced" className="pt-4 overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <Panel>
            <SectionLabel>{t('settings.sections.server')}</SectionLabel>
            <G2>
              <F label={t('settings.adv.listenAddr')} hint={t('settings.adv.listenAddrHint')}>
                <TI value={cfg.listen_addr} onChange={set('listen_addr')} placeholder=":8080" />
              </F>
              <F label={t('settings.adv.scripCurrency')}>
                <NumberInput
                  ariaLabel={t('settings.adv.scripCurrency')}
                  value={Number(cfg.scrip_currency) || 0}
                  onChange={(v) => set('scrip_currency')(String(v))}
                  showButtons={false}
                  className="w-full"
                />
              </F>
              <F label={t('settings.adv.directorUrl')} hint={t('settings.adv.directorUrlHint')}>
                <TI value={cfg.director_url} onChange={set('director_url')} placeholder="http://127.0.0.1:11717" />
              </F>
            </G2>
          </Panel>

          <Panel>
            <SectionLabel>{t('settings.sections.paths')}</SectionLabel>
            <G2>
              <F label={t('settings.adv.backupDir')}>
                <TI value={cfg.backup_dir} onChange={set('backup_dir')} placeholder="/path/to/backups" />
              </F>
              <F label={t('settings.adv.serverIniDir')} hint={t('settings.adv.serverIniDirHint')}>
                <TI value={cfg.server_ini_dir} onChange={set('server_ini_dir')} placeholder="/path/to/server/state" />
              </F>
              <F label={t('settings.adv.defaultIniDir')} hint={t('settings.adv.defaultIniDirHint')}>
                <TI value={cfg.default_ini_dir} onChange={set('default_ini_dir')} placeholder="/path/to/game/Config" />
              </F>
            </G2>
          </Panel>

          <Panel>
            <SectionLabel>{t('settings.sections.backendUrlOverride')}</SectionLabel>
            <p className="text-xs text-muted -mt-1">
              {t('settings.adv.backendUrlHint')}
            </p>
            <G2>
              <F label={t('settings.adv.url')} hint={t('settings.adv.urlHint')}>
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
              <Button size="sm" onPress={() => window.location.reload()}>{t('settings.adv.applyReload')}</Button>
              <Button
                size="sm"
                variant="outline"
                onPress={() => {
                  setBackendUrl('')
                  localStorage.removeItem('dune_admin_backend')
                  window.location.reload()
                }}
              >
                {t('settings.adv.reset')}
              </Button>
            </div>
          </Panel>
        </Tabs.Panel>
      </Tabs>

    </div>
  )
}
