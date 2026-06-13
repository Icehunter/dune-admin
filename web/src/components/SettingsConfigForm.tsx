import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, CloseButton, Input, Select, ListBox, Spinner, Switch, toast } from '@heroui/react'
import { Segment } from '@heroui-pro/react'
import { api, MASKED } from '../api/client'
import type { AppConfig } from '../api/client'
import { Icon, NumberInput, Panel, SectionLabel } from '../dune-ui'
import { DiscordMemberPicker } from './DiscordMemberPicker'
import { AuthContext } from '../auth/context'
import type {
  FieldProps,
  TextInputProps,
  CheckboxFieldProps,
  GridRowProps,
  DiscordRole,
  RolePickerProps,
  SettingsConfigFormProps,
} from './types'

const FieldLabelContext = React.createContext('')

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
const pointerBoolFields = new Set<keyof AppConfig>(['amp_use_container', 'market_bot_enabled'])

const mergeConfig = (fetched: Record<string, unknown>): AppConfig => {
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

const FieldRow: React.FC<FieldProps> = ({ label, hint, children }) => {
  return (
    <FieldLabelContext.Provider value={label}>
      <div className="flex flex-col gap-1">
        <span className="text-xs text-muted font-medium">
          {label}
          {hint && (
            <span className="opacity-60 font-normal">
              {' '}
              (
              {hint}
              )
            </span>
          )}
        </span>
        {children}
      </div>
    </FieldLabelContext.Provider>
  )
}

const TextInput: React.FC<TextInputProps> = ({ value, onChange, placeholder, type = 'text', autoComplete }) => {
  const fieldLabel = React.useContext(FieldLabelContext)
  return (
    <Input
      className="font-mono"
      type={type}
      value={String(value)}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      aria-label={fieldLabel || placeholder || 'value'}
      autoComplete={autoComplete ?? (type === 'password' ? 'new-password' : 'off')}
    />
  )
}

const CheckboxField: React.FC<CheckboxFieldProps> = ({ label, checked, onChange, hint }) => {
  return (
    <div className="flex flex-col gap-1">
      {hint && <p className="text-xs text-muted">{hint}</p>}
      <div className="flex flex-1 items-center">
        <Switch isSelected={!!checked} onChange={onChange} size="sm">
          <Switch.Control><Switch.Thumb /></Switch.Control>
          <Switch.Content>{label}</Switch.Content>
        </Switch>
      </div>
    </div>
  )
}

const TwoColumnGrid: React.FC<GridRowProps> = ({ children }) => {
  return <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-1">{children}</div>
}

// ── RolePicker ────────────────────────────────────────────────────────────────

const RolePicker: React.FC<RolePickerProps> = ({ value, onChange, roles, label, hint }) => {
  const { t } = useTranslation()
  const [pickKey, setPickKey] = React.useState(0)

  const selectedIds = value ? value.split(',').map((s) => s.trim()).filter(Boolean) : []
  const nameOf = (id: string) => roles.find((r) => r.id === id)?.name ?? id
  const available = roles.filter((r) => !selectedIds.includes(r.id))

  const addRole = (id: string) => {
    if (id && !selectedIds.includes(id)) {
      onChange([...selectedIds, id].join(','))
    }
    setPickKey((k) => k + 1)
  }

  const removeRole = (id: string) => onChange(selectedIds.filter((s) => s !== id).join(','))

  return (
    <FieldRow label={label} hint={hint}>
      <div className="flex flex-col gap-1.5">
        {available.length > 0
          ? (
              <Select
                key={pickKey}
                selectedKey=""
                aria-label={t('settings.discord.addRole')}
                onSelectionChange={(k) => addRole(String(k))}
              >
                <Select.Trigger>
                  <span className="text-sm text-muted flex-1">{t('settings.discord.addRole')}</span>
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    {available.map((r) => (
                      <ListBox.Item key={r.id} id={r.id} textValue={r.name}>
                        {r.name}
                        <ListBox.ItemIndicator />
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
            )
          : (
              roles.length === 0 && selectedIds.length === 0 && (
                <p className="text-xs text-muted">{t('settings.discord.rolesNotLoaded')}</p>
              )
            )}
        {selectedIds.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {selectedIds.map((id) => (
              <span key={id} className="inline-flex items-center gap-1 rounded-full bg-accent/15 text-accent px-2 py-0.5 text-xs font-medium">
                {nameOf(id)}
                <CloseButton aria-label={`Remove ${nameOf(id)}`} className="size-4 opacity-60 hover:opacity-100" onPress={() => removeRole(id)} />
              </span>
            ))}
          </div>
        )}
      </div>
    </FieldRow>
  )
}

// ── main component ────────────────────────────────────────────────────────────

export const SettingsConfigForm: React.FC<SettingsConfigFormProps> = ({ saveRef, onSavingChange }) => {
  const { t } = useTranslation()
  const auth = React.useContext(AuthContext)
  const [cfg, setCfg] = React.useState<AppConfig>(EMPTY)
  const [loading, setLoading] = React.useState(true)
  const [tab, setTab] = React.useState('connection')
  const [backendUrl, setBackendUrl] = React.useState(() => localStorage.getItem('dune_admin_backend') || '')

  const [discordRoles, setDiscordRoles] = React.useState<DiscordRole[]>([])
  const [rolesLoading, setRolesLoading] = React.useState(false)

  React.useEffect(() => {
    api.config.get()
      .then((c) => setCfg(mergeConfig(c as Record<string, unknown>)))
      .catch((e) => toast.danger(t('settings.loadFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t])

  const loadDiscordRoles = React.useCallback(() => {
    setRolesLoading(true)
    api.discord.roles()
      .then(setDiscordRoles)
      .catch(() => setDiscordRoles([]))
      .finally(() => setRolesLoading(false))
  }, [])

  React.useEffect(() => {
    Promise.resolve().then(loadDiscordRoles)
  }, [loadDiscordRoles])

  React.useEffect(() => {
    if (tab === 'discord' || tab === 'auth') Promise.resolve().then(loadDiscordRoles)
  }, [tab, loadDiscordRoles])

  const set = (key: keyof AppConfig) => (v: string) =>
    setCfg((prev) => ({
      ...prev,
      [key]: key === 'db_port' || key === 'scrip_currency' || key === 'market_bot_max_buys' || key === 'amp_api_port' || key === 'discord_status_interval_seconds'
        ? (Number(v) || 0)
        : key === 'market_bot_buy_threshold'
          ? (parseFloat(v) || 0)
          : v,
    }))

  const setBool = (key: keyof AppConfig) => (v: boolean) =>
    setCfg((prev) => ({ ...prev, [key]: v }))

  const save = async () => {
    onSavingChange?.(true)
    const authToggled = auth.enabled !== cfg.auth_enabled
    try {
      await api.config.save(cfg)
      toast.success(t('settings.configSaved'))
      // Toggling authentication clears the session cookie server-side; reset
      // the route to the default tab and force a full reload so the SPA
      // re-bootstraps from a clean slate — the login page when enabling, the
      // Battlegroup tab when disabling — with no stale auth state or route.
      if (authToggled) {
        window.location.hash = '#/battlegroup'
        window.location.reload()
        return
      }
      // Non-toggle save: re-sync auth status (e.g. methods/owners changed).
      await auth.refresh()
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
  React.useEffect(() => {
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

  const TABS = [
    { id: 'connection', label: t('settings.tabs.connection') },
    { id: 'control', label: t('settings.tabs.control') },
    { id: 'broker', label: t('settings.tabs.broker') },
    { id: 'discord', label: t('settings.tabs.discord') },
    { id: 'auth', label: t('settings.tabs.auth') },
    { id: 'advanced', label: t('settings.tabs.advanced') },
  ]

  return (
    <form className="flex flex-col flex-1 min-h-0 gap-3" onSubmit={(e) => e.preventDefault()} autoComplete="off">
      {/* sr-only (not display:none) — Chrome's credential heuristic skips display:none elements */}
      <input type="text" autoComplete="username" aria-hidden="true" tabIndex={-1} readOnly className="sr-only" />
      <Segment
        selectedKey={tab}
        onSelectionChange={(k) => setTab(String(k))}
        size="sm"
        className="shrink-0 w-fit ml-auto"
      >
        {TABS.map(({ id, label }) => (
          <Segment.Item key={id} id={id}>
            <Segment.Separator />
            {label}
          </Segment.Item>
        ))}
      </Segment>

      {/* ── Connection ─────────────────────────────────────────────────── */}
      {tab === 'connection' && (
        <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <Panel>
            <SectionLabel>{t('settings.sections.database')}</SectionLabel>
            <TwoColumnGrid>
              <FieldRow label={t('settings.db.host')} hint={t('settings.db.hostHint')}>
                <TextInput value={cfg.db_host} onChange={set('db_host')} placeholder="127.0.0.1" />
              </FieldRow>
              <FieldRow label={t('settings.db.port')}>
                <NumberInput
                  ariaLabel={t('settings.db.port')}
                  value={Number(cfg.db_port) || 0}
                  onChange={(v) => set('db_port')(String(v))}
                  showButtons={false}
                  className="w-full"
                />
              </FieldRow>
              <FieldRow label={t('settings.db.user')}>
                <TextInput value={cfg.db_user} onChange={set('db_user')} placeholder="dune" />
              </FieldRow>
              <FieldRow label={t('settings.db.password')} hint={t('settings.db.passwordHint')}>
                <TextInput value={cfg.db_pass} onChange={set('db_pass')} type="password" placeholder={MASKED} />
              </FieldRow>
              <FieldRow label={t('settings.db.name')}>
                <TextInput value={cfg.db_name} onChange={set('db_name')} placeholder="dune" />
              </FieldRow>
              <FieldRow label={t('settings.db.schema')}>
                <TextInput value={cfg.db_schema} onChange={set('db_schema')} placeholder="dune" />
              </FieldRow>
            </TwoColumnGrid>
          </Panel>

          <Panel>
            <SectionLabel>{t('settings.sections.ssh')}</SectionLabel>
            <TwoColumnGrid>
              <FieldRow label={t('settings.ssh.hostPort')} hint={t('settings.ssh.hostPortHint')}>
                <TextInput value={cfg.ssh_host} onChange={set('ssh_host')} placeholder="192.168.0.72:22" />
              </FieldRow>
              <FieldRow label={t('settings.ssh.user')}>
                <TextInput value={cfg.ssh_user} onChange={set('ssh_user')} placeholder="dune" />
              </FieldRow>
              <FieldRow label={t('settings.ssh.privateKey')} hint={t('settings.ssh.privateKeyHint')}>
                <TextInput value={cfg.ssh_key} onChange={set('ssh_key')} placeholder="~/.ssh/id_ed25519" />
              </FieldRow>
            </TwoColumnGrid>
          </Panel>
        </div>
      )}

      {/* ── Control ────────────────────────────────────────────────────── */}
      {tab === 'control' && (
        <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <Panel>
            <SectionLabel>{t('settings.sections.controlPlane')}</SectionLabel>
            <div className="flex flex-col gap-1">
              <span className="text-xs text-muted font-medium">{t('settings.control.modeHint')}</span>
              <Select
                selectedKey={cfg.control || 'local'}
                onSelectionChange={(k) => setCfg((prev) => ({ ...prev, control: String(k) }))}
                className="w-64"
                aria-label={t('settings.sections.controlPlane')}
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
            </div>
          </Panel>

          {isKubectl && (
            <Panel>
              <SectionLabel>{t('settings.sections.kubernetes')}</SectionLabel>
              <TwoColumnGrid>
                <FieldRow label={t('settings.k8s.namespace')} hint={t('settings.k8s.namespaceHint')}>
                  <TextInput value={cfg.control_namespace} onChange={set('control_namespace')} placeholder="my-namespace" />
                </FieldRow>
              </TwoColumnGrid>
            </Panel>
          )}

          {isDocker && (
            <Panel>
              <SectionLabel>{t('settings.sections.dockerContainers')}</SectionLabel>
              <TwoColumnGrid>
                <FieldRow label={t('settings.docker.gameServer')}><TextInput value={cfg.docker_gameserver} onChange={set('docker_gameserver')} placeholder="dune-gameserver" /></FieldRow>
                <FieldRow label={t('settings.docker.brokerGame')}><TextInput value={cfg.docker_broker_game} onChange={set('docker_broker_game')} placeholder="dune-mq-game" /></FieldRow>
                <FieldRow label={t('settings.docker.brokerAdmin')}><TextInput value={cfg.docker_broker_admin} onChange={set('docker_broker_admin')} placeholder="dune-mq-admin" /></FieldRow>
                <FieldRow label={t('settings.docker.database')}><TextInput value={cfg.docker_db} onChange={set('docker_db')} placeholder="dune-postgres" /></FieldRow>
              </TwoColumnGrid>
            </Panel>
          )}

          {isLocal && (
            <Panel>
              <SectionLabel>{t('settings.sections.serverCommands')}</SectionLabel>
              <TwoColumnGrid>
                <FieldRow label={t('settings.cmd.start')}><TextInput value={cfg.cmd_start} onChange={set('cmd_start')} placeholder="service dune start" /></FieldRow>
                <FieldRow label={t('settings.cmd.stop')}><TextInput value={cfg.cmd_stop} onChange={set('cmd_stop')} placeholder="service dune stop" /></FieldRow>
                <FieldRow label={t('settings.cmd.restart')}><TextInput value={cfg.cmd_restart} onChange={set('cmd_restart')} placeholder="service dune restart" /></FieldRow>
                <FieldRow label={t('settings.cmd.status')}><TextInput value={cfg.cmd_status} onChange={set('cmd_status')} placeholder="service dune status" /></FieldRow>
              </TwoColumnGrid>
            </Panel>
          )}

          {isAmp && (
            <Panel>
              <SectionLabel>{t('settings.sections.amp')}</SectionLabel>
              <TwoColumnGrid>
                <FieldRow label={t('settings.amp.instanceName')}><TextInput value={cfg.amp_instance} onChange={set('amp_instance')} placeholder="DuneAwakening01" /></FieldRow>
                <FieldRow label={t('settings.amp.containerName')} hint={t('settings.amp.containerNameHint')}><TextInput value={cfg.amp_container} onChange={set('amp_container')} placeholder="AMP_DuneAwakening01" /></FieldRow>
                <FieldRow label={t('settings.amp.user')}><TextInput value={cfg.amp_user} onChange={set('amp_user')} placeholder="amp" /></FieldRow>
                <FieldRow label={t('settings.amp.logPath')}><TextInput value={cfg.amp_log_path} onChange={set('amp_log_path')} placeholder="/logs" /></FieldRow>
                <FieldRow label={t('settings.amp.dataRoot')}><TextInput value={cfg.amp_data_root} onChange={set('amp_data_root')} placeholder="/AMP/duneawakening" /></FieldRow>
                <CheckboxField
                  label={t('settings.amp.useContainer')}
                  checked={cfg.amp_use_container}
                  onChange={setBool('amp_use_container')}
                  hint={t('settings.amp.useContainerHint')}
                />
              </TwoColumnGrid>
              <p className="text-xs text-muted mt-3">{t('settings.amp.apiHint')}</p>
              <TwoColumnGrid>
                <FieldRow label={t('settings.amp.apiUser')}><TextInput value={cfg.amp_api_user} onChange={set('amp_api_user')} placeholder="admin" /></FieldRow>
                <FieldRow label={t('settings.amp.apiPassword')}><TextInput value={cfg.amp_api_pass} onChange={set('amp_api_pass')} type="password" placeholder={MASKED} /></FieldRow>
                <FieldRow label={t('settings.amp.apiPort')} hint="default 8081">
                  <NumberInput
                    ariaLabel={t('settings.amp.apiPort')}
                    value={Number(cfg.amp_api_port) || 0}
                    onChange={(v) => set('amp_api_port')(String(v))}
                    showButtons={false}
                    className="w-full"
                  />
                </FieldRow>
              </TwoColumnGrid>
            </Panel>
          )}

          {!isKubectl && !isDocker && !isLocal && !isAmp && (
            <p className="text-xs text-muted pt-2">{t('settings.control.selectMode')}</p>
          )}
        </div>
      )}

      {/* ── Broker ─────────────────────────────────────────────────────── */}
      {tab === 'broker' && (
        <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <Panel>
            <SectionLabel>{t('settings.sections.rabbitmq')}</SectionLabel>
            <p className="text-xs text-muted -mt-1">{t('settings.broker.optionalHint')}</p>
            <TwoColumnGrid>
              <FieldRow label={t('settings.broker.gameAddr')}><TextInput value={cfg.broker_game_addr} onChange={set('broker_game_addr')} placeholder="10.x.x.x:5672" /></FieldRow>
              <FieldRow label={t('settings.broker.adminAddr')}><TextInput value={cfg.broker_admin_addr} onChange={set('broker_admin_addr')} placeholder="10.x.x.x:5672" /></FieldRow>
              <FieldRow label={t('settings.broker.user')}><TextInput value={cfg.broker_user} onChange={set('broker_user')} placeholder="dune_cap" /></FieldRow>
              <FieldRow label={t('settings.broker.password')}><TextInput value={cfg.broker_pass} onChange={set('broker_pass')} type="password" placeholder={MASKED} /></FieldRow>
              <FieldRow label={t('settings.broker.jwtSecret')} hint={t('settings.broker.jwtSecretHint')}>
                <TextInput value={cfg.broker_jwt_secret} onChange={set('broker_jwt_secret')} type="password" placeholder={MASKED} />
              </FieldRow>
              <FieldRow label={t('settings.broker.execPrefix')} hint={t('settings.broker.execPrefixHint')}>
                <TextInput value={cfg.broker_exec_prefix} onChange={set('broker_exec_prefix')} placeholder="podman exec <container>" />
              </FieldRow>
              <div className="sm:col-span-2">
                <CheckboxField label={t('settings.broker.useTls')} checked={cfg.broker_tls} onChange={setBool('broker_tls')} />
              </div>
            </TwoColumnGrid>
          </Panel>
        </div>
      )}

      {/* ── Discord ────────────────────────────────────────────────────── */}
      {tab === 'discord' && (
        <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <input type="text" autoComplete="username" aria-hidden="true" tabIndex={-1} readOnly className="sr-only" />
          <Panel>
            <SectionLabel>{t('settings.sections.discordBot')}</SectionLabel>
            <div className="flex flex-col gap-1 -mt-1">
              <p className="text-sm text-muted">{t('settings.discord.hint')}</p>
              <p className="text-sm text-muted">{t('settings.discord.setupStep1')}</p>
              <p className="text-sm text-muted">{t('settings.discord.setupStep2')}</p>
              <p className="text-sm text-muted">{t('settings.discord.setupStep3')}</p>
            </div>
            <TwoColumnGrid>
              <div className="sm:col-span-2">
                <CheckboxField
                  label={t('settings.discord.enabled')}
                  checked={cfg.discord_bot_enabled}
                  onChange={setBool('discord_bot_enabled')}
                />
              </div>
              <FieldRow label={t('settings.discord.token')} hint={t('settings.discord.tokenHint')}>
                <TextInput value={cfg.discord_bot_token} onChange={set('discord_bot_token')} type="password" placeholder={MASKED} />
              </FieldRow>
              <FieldRow label={t('settings.discord.guildId')} hint={t('settings.discord.guildIdHint')}>
                <TextInput value={cfg.discord_guild_id} onChange={set('discord_guild_id')} placeholder="123456789012345678" />
              </FieldRow>
            </TwoColumnGrid>
          </Panel>

          <Panel>
            <div className="flex items-center justify-between">
              <SectionLabel>{t('settings.sections.discordRoles')}</SectionLabel>
              <Button size="sm" variant="ghost" onPress={loadDiscordRoles} isDisabled={rolesLoading}>
                {rolesLoading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
                {' '}
                {t('common.refresh')}
              </Button>
            </div>
            <div className="flex flex-col gap-1 -mt-1">
              <p className="text-xs text-muted">{t('settings.discord.rolesHint')}</p>
              <p className="text-sm text-muted">{t('settings.discord.rolesRefreshNote')}</p>
            </div>
            <TwoColumnGrid>
              <RolePicker
                label={t('settings.discord.rolesViewer')}
                hint={t('settings.discord.rolesViewerHint')}
                value={cfg.discord_roles_viewer}
                onChange={set('discord_roles_viewer')}
                roles={discordRoles}
              />
              <RolePicker
                label={t('settings.discord.rolesEconomy')}
                hint={t('settings.discord.rolesEconomyHint')}
                value={cfg.discord_roles_economy}
                onChange={set('discord_roles_economy')}
                roles={discordRoles}
              />
              <RolePicker
                label={t('settings.discord.rolesAdmin')}
                hint={t('settings.discord.rolesAdminHint')}
                value={cfg.discord_roles_admin}
                onChange={set('discord_roles_admin')}
                roles={discordRoles}
              />
              <FieldRow label={t('settings.discord.announceChannel')} hint={t('settings.discord.announceChannelHint')}>
                <TextInput value={cfg.discord_announce_channel_id} onChange={set('discord_announce_channel_id')} placeholder="444444444444444444" />
              </FieldRow>
            </TwoColumnGrid>
          </Panel>

          <Panel>
            <SectionLabel>{t('settings.sections.discordStatus')}</SectionLabel>
            <div className="flex flex-col gap-1 -mt-1">
              <p className="text-sm text-muted">{t('settings.discord.statusHint')}</p>
            </div>
            <TwoColumnGrid>
              <div className="sm:col-span-2">
                <CheckboxField
                  label={t('settings.discord.statusEnabled')}
                  hint={t('settings.discord.statusEnabledHint')}
                  checked={cfg.discord_status_enabled}
                  onChange={setBool('discord_status_enabled')}
                />
              </div>
              <FieldRow label={t('settings.discord.statusChannel')} hint={t('settings.discord.statusChannelHint')}>
                <TextInput value={cfg.discord_status_channel_id} onChange={set('discord_status_channel_id')} placeholder="555555555555555555" />
              </FieldRow>
              <FieldRow label={t('settings.discord.statusInterval')} hint={t('settings.discord.statusIntervalHint')}>
                <NumberInput
                  ariaLabel={t('settings.discord.statusInterval')}
                  value={Number(cfg.discord_status_interval_seconds) || 0}
                  onChange={(v) => set('discord_status_interval_seconds')(String(v))}
                  showButtons={false}
                  className="w-full"
                />
              </FieldRow>
            </TwoColumnGrid>
          </Panel>
        </div>
      )}

      {/* ── Authentication ─────────────────────────────────────────────── */}
      {tab === 'auth' && (
        <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <input type="text" autoComplete="username" aria-hidden="true" tabIndex={-1} readOnly className="sr-only" />
          <Panel>
            <SectionLabel>{t('settings.sections.authDashboard')}</SectionLabel>
            <div className="flex flex-col gap-1 -mt-1">
              <p className="text-sm text-muted">{t('settings.auth.hint')}</p>
            </div>
            <TwoColumnGrid>
              <div className="sm:col-span-2">
                <CheckboxField
                  label={t('settings.auth.enabled')}
                  checked={cfg.auth_enabled}
                  onChange={setBool('auth_enabled')}
                />
              </div>
              <div className="sm:col-span-2">
                <CheckboxField
                  label={t('settings.auth.guestEnabled')}
                  hint={t('settings.auth.guestEnabledHint')}
                  checked={cfg.auth_guest_enabled}
                  onChange={setBool('auth_guest_enabled')}
                />
              </div>
              <FieldRow label={t('settings.auth.localUsername')}>
                <TextInput value={cfg.auth_local_username} onChange={set('auth_local_username')} placeholder="admin" />
              </FieldRow>
              <FieldRow label={t('settings.auth.localPassword')} hint={t('settings.auth.localPasswordHint')}>
                <TextInput
                  value={cfg.auth_local_password_new ?? ''}
                  onChange={set('auth_local_password_new')}
                  type="password"
                  placeholder={cfg.auth_local_password_hash ? MASKED : ''}
                />
              </FieldRow>
              <FieldRow label={t('settings.auth.sessionTtl')} hint={t('settings.auth.sessionTtlHint')}>
                <NumberInput
                  ariaLabel={t('settings.auth.sessionTtl')}
                  value={Number(cfg.auth_session_ttl_hours) || 0}
                  onChange={(v) => set('auth_session_ttl_hours')(String(v))}
                  showButtons={false}
                  className="w-full"
                />
              </FieldRow>
              <FieldRow label={t('settings.auth.cookiePolicy')}>
                <Select
                  selectedKey={(cfg.auth_cookie_samesite || 'lax').toLowerCase()}
                  onSelectionChange={(k) => set('auth_cookie_samesite')(String(k))}
                  className="w-full"
                  aria-label={t('settings.auth.cookiePolicy')}
                >
                  <Select.Trigger>
                    <Select.Value />
                    <Select.Indicator />
                  </Select.Trigger>
                  <Select.Popover>
                    <ListBox>
                      <ListBox.Item id="lax" textValue={t('settings.auth.cookieLax')}>
                        {t('settings.auth.cookieLax')}
                        <ListBox.ItemIndicator />
                      </ListBox.Item>
                      <ListBox.Item id="strict" textValue={t('settings.auth.cookieStrict')}>
                        {t('settings.auth.cookieStrict')}
                        <ListBox.ItemIndicator />
                      </ListBox.Item>
                      <ListBox.Item id="none" textValue={t('settings.auth.cookieNone')}>
                        {t('settings.auth.cookieNone')}
                        <ListBox.ItemIndicator />
                      </ListBox.Item>
                    </ListBox>
                  </Select.Popover>
                </Select>
              </FieldRow>
              <div className="sm:col-span-2 flex flex-col gap-1 rounded-[var(--radius)] bg-surface-secondary/40 border border-border p-3 text-xs text-muted">
                <p>
                  <strong className="text-foreground">{t('settings.auth.cookieLax')}</strong>
                  {' — '}
                  {t('settings.auth.cookieLaxDesc')}
                </p>
                <p>
                  <strong className="text-foreground">{t('settings.auth.cookieStrict')}</strong>
                  {' — '}
                  {t('settings.auth.cookieStrictDesc')}
                </p>
                <p>
                  <strong className="text-foreground">{t('settings.auth.cookieNone')}</strong>
                  {' — '}
                  {t('settings.auth.cookieNoneDesc')}
                </p>
              </div>
            </TwoColumnGrid>
          </Panel>

          <Panel>
            <SectionLabel>{t('settings.sections.authDiscord')}</SectionLabel>
            <div className="flex flex-col gap-1 -mt-1">
              <p className="text-sm text-muted">{t('settings.auth.discordHint')}</p>
              <p className="text-sm text-muted">{t('settings.auth.discordStep1')}</p>
              <p className="text-sm text-muted">{t('settings.auth.discordStep2')}</p>
              <p className="text-sm text-muted">{t('settings.auth.discordStep3')}</p>
            </div>
            <TwoColumnGrid>
              <div className="sm:col-span-2">
                <CheckboxField
                  label={t('settings.auth.discordEnabled')}
                  checked={cfg.auth_discord_enabled}
                  onChange={setBool('auth_discord_enabled')}
                />
              </div>
              <FieldRow label={t('settings.auth.clientId')}>
                <TextInput value={cfg.auth_discord_client_id} onChange={set('auth_discord_client_id')} placeholder="123456789012345678" />
              </FieldRow>
              <FieldRow label={t('settings.auth.clientSecret')}>
                <TextInput value={cfg.auth_discord_client_secret} onChange={set('auth_discord_client_secret')} type="password" placeholder={MASKED} />
              </FieldRow>
              <div className="sm:col-span-2">
                <FieldRow label={t('settings.auth.redirectUrl')} hint={t('settings.auth.redirectUrlHint')}>
                  <TextInput value={cfg.auth_discord_redirect_url} onChange={set('auth_discord_redirect_url')} placeholder={`${window.location.origin}/api/v1/auth/discord/callback`} />
                </FieldRow>
              </div>
            </TwoColumnGrid>
          </Panel>

          <Panel>
            <div className="flex items-center justify-between">
              <SectionLabel>{t('settings.sections.authOwners')}</SectionLabel>
              <Button size="sm" variant="ghost" onPress={loadDiscordRoles} isDisabled={rolesLoading}>
                {rolesLoading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
                {' '}
                {t('settings.auth.refreshRoles')}
              </Button>
            </div>
            <div className="flex flex-col gap-1 -mt-1">
              <p className="text-sm text-muted">{t('settings.auth.ownersHint')}</p>
            </div>
            <TwoColumnGrid>
              <FieldRow label={t('settings.auth.ownerIds')} hint={t('settings.auth.ownerIdsHint')}>
                <DiscordMemberPicker
                  value={cfg.auth_owner_discord_ids}
                  onChange={set('auth_owner_discord_ids')}
                  ariaLabel={t('settings.auth.ownerIds')}
                />
              </FieldRow>
              <RolePicker
                label={t('settings.auth.ownerRoles')}
                hint={t('settings.auth.ownerRolesHint')}
                value={cfg.auth_owner_role_ids}
                onChange={set('auth_owner_role_ids')}
                roles={discordRoles}
              />
            </TwoColumnGrid>
          </Panel>
        </div>
      )}

      {/* ── Advanced ───────────────────────────────────────────────────── */}
      {tab === 'advanced' && (
        <div className="overflow-y-auto flex-1 pr-1 flex flex-col gap-4">
          <Panel>
            <SectionLabel>{t('settings.sections.server')}</SectionLabel>
            <TwoColumnGrid>
              <FieldRow label={t('settings.adv.listenAddr')} hint={t('settings.adv.listenAddrHint')}>
                <TextInput value={cfg.listen_addr} onChange={set('listen_addr')} placeholder=":8080" />
              </FieldRow>
              <FieldRow label={t('settings.adv.scripCurrency')}>
                <NumberInput
                  ariaLabel={t('settings.adv.scripCurrency')}
                  value={Number(cfg.scrip_currency) || 0}
                  onChange={(v) => set('scrip_currency')(String(v))}
                  showButtons={false}
                  className="w-full"
                />
              </FieldRow>
              <FieldRow label={t('settings.adv.directorUrl')} hint={t('settings.adv.directorUrlHint')}>
                <TextInput value={cfg.director_url} onChange={set('director_url')} placeholder="http://127.0.0.1:11717" />
              </FieldRow>
            </TwoColumnGrid>
          </Panel>

          <Panel>
            <SectionLabel>{t('settings.sections.paths')}</SectionLabel>
            <TwoColumnGrid>
              <FieldRow label={t('settings.adv.backupDir')}>
                <TextInput value={cfg.backup_dir} onChange={set('backup_dir')} placeholder="/path/to/backups" />
              </FieldRow>
              <FieldRow label={t('settings.adv.serverIniDir')} hint={t('settings.adv.serverIniDirHint')}>
                <TextInput value={cfg.server_ini_dir} onChange={set('server_ini_dir')} placeholder="/path/to/server/state" />
              </FieldRow>
              <FieldRow label={t('settings.adv.defaultIniDir')} hint={t('settings.adv.defaultIniDirHint')}>
                <TextInput value={cfg.default_ini_dir} onChange={set('default_ini_dir')} placeholder="/path/to/game/Config" />
              </FieldRow>
            </TwoColumnGrid>
          </Panel>

          <Panel>
            <SectionLabel>{t('settings.sections.backendUrlOverride')}</SectionLabel>
            <p className="text-xs text-muted -mt-1">
              {t('settings.adv.backendUrlHint')}
            </p>
            <TwoColumnGrid>
              <FieldRow label={t('settings.adv.url')} hint={t('settings.adv.urlHint')}>
                <TextInput
                  value={backendUrl}
                  onChange={(v) => {
                    setBackendUrl(v)
                    localStorage.setItem('dune_admin_backend', v)
                  }}
                  placeholder="http://host:port"
                />
              </FieldRow>
            </TwoColumnGrid>
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
        </div>
      )}
    </form>
  )
}
