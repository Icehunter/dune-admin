import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Spinner, toast } from '@heroui/react'
import { Segment } from '@heroui-pro/react'
import { api } from '../../../api/client'
import type { AppConfig, ServerConfig } from '../../../api/client'
import { AuthContext } from '../../../auth/context'
import { Panel } from '../../../dune-ui'
import { useActiveServer } from '../../../context/useActiveServer'
import { EMPTY, mergeConfig, pickPerServer } from '../config'
import { FieldRow } from '../fields/FieldRow'
import { TextInput } from '../fields/TextInput'
import { ControlPanel } from './ControlPanel'
import { SshPanel } from './SshPanel'
import { ConnectionPanel } from './ConnectionPanel'
import { ServerAdvancedPanel } from './ServerAdvancedPanel'
import { ServerDiscordPanel } from './ServerDiscordPanel'
import type { ServerAdvancedVariant } from './ServerAdvancedPanel'

export interface ServerSettingsFormProps {
  saveRef?: React.MutableRefObject<(() => Promise<void>) | null>
  onSavingChange?: (saving: boolean) => void
  /**
   * Manage-server page: target this server id for load/save instead of the
   * active server. Also enables the inline Name (rename) field.
   */
  serverId?: number
  /**
   * Add-server wizard: persist creates a NEW per-server entry via POST /servers
   * (not the flat config). Only per-server fields are sent.
   */
  addMode?: boolean
  /** Add-server wizard: the name entered for the new server. */
  addServerName?: string
  /**
   * Add-server wizard: called whenever the live form config changes so the
   * wizard can read current values (control plane + SSH) to drive discovery.
   */
  onConfigChange?: (cfg: AppConfig) => void
  /** Add-server wizard: discovered values to merge into the form config. */
  prefill?: Partial<AppConfig> | null
  /** When set, overrides the internal tab state (wizard mode). */
  activeTab?: string
  /** When true, hides the Segment tab bar (wizard drives navigation). */
  hideTabBar?: boolean
  /**
   * Settings-modal only: invoked from the per-server Advanced "Danger Zone" to
   * request deletion of the active server. When omitted, the Danger Zone is hidden.
   */
  onRequestDeleteServer?: () => void
  /** Initial tab to open on; still switchable. */
  initialTab?: string
}

export const ServerSettingsForm: React.FC<ServerSettingsFormProps> = ({
  saveRef, onSavingChange, serverId, addMode, addServerName, onConfigChange, prefill,
  activeTab, hideTabBar, onRequestDeleteServer, initialTab,
}) => {
  const { t } = useTranslation()
  const auth = React.useContext(AuthContext)
  const { activeID, servers, refresh: refreshServers } = useActiveServer()
  // Settings-modal mode (vs. wizard): the wizard hides the tab bar and drives
  // navigation via activeTab. Only the modal shows the rename field + Danger Zone.
  const settingsMode = !hideTabBar
  // The Manage-server page targets an explicit serverId; otherwise fall back to
  // the active server (single-server installs never set an explicit active id).
  const serverID = serverId ?? activeID ?? servers[0]?.id ?? 0
  const activeName = servers.find((s) => s.id === serverID)?.name ?? ''

  const [cfg, setCfg] = React.useState<AppConfig>(EMPTY)
  // Per-server display name (rename); loaded from the server's config.
  const [serverName, setServerName] = React.useState('')
  const [loading, setLoading] = React.useState(true)
  const [internalTab, setInternalTab] = React.useState(initialTab ?? 'control')
  const tab = activeTab ?? internalTab
  const [backendUrl, setBackendUrl] = React.useState(() => localStorage.getItem('dune_admin_backend') || '')

  // Raw per-server base kept so a save reconstructs the payload without
  // clobbering fields it doesn't own.
  const serverBaseRef = React.useRef<ServerConfig | null>(null)

  React.useEffect(() => {
    const onErr = (e: unknown) =>
      toast.danger(t('settings.loadFailed', { message: e instanceof Error ? e.message : String(e) }))

    // Wizard mode (first-run or add-server): edit a single flat config.
    if (!settingsMode) {
      api.config.get()
        .then((c) => setCfg(mergeConfig(c as Record<string, unknown>)))
        .catch(onErr)
        .finally(() => setLoading(false))
      return
    }

    // Settings / manage mode: load this server's per-server config.
    api.servers.getConfig(serverID)
      .then((server) => {
        serverBaseRef.current = server
        if (server?.name) setServerName(server.name)
        setCfg(mergeConfig(server as Record<string, unknown>))
      })
      .catch(onErr)
      .finally(() => setLoading(false))
  }, [t, settingsMode, serverID])

  const set = (key: keyof AppConfig) => (v: string) =>
    setCfg((prev) => ({
      ...prev,
      [key]: key === 'db_port' || key === 'amp_api_port'
        ? (Number(v) || 0)
        : v,
    }))

  const setBool = (key: keyof AppConfig) => (v: boolean) =>
    setCfg((prev) => ({ ...prev, [key]: v }))

  const setControl = (v: string) => setCfg((prev) => ({ ...prev, control: v }))

  const persist = async () => {
    // Add-server wizard: create a NEW per-server entry. Only per-server fields
    // are sent; the backend assigns the numeric id.
    if (addMode) {
      const name = (addServerName ?? '').trim()
      if (!name) throw new Error(t('setup.nameRequired', 'Server name is required'))
      const payload = { ...pickPerServer(cfg), id: 0, name } as ServerConfig
      await api.servers.add(payload)
      return
    }
    // First-run wizard: a single flat save creates/edits the default server.
    if (!settingsMode) {
      await api.config.save(cfg)
      return
    }
    // Manage-server (per-server) scope: save only this server's config + name.
    const serverPayload = {
      ...(serverBaseRef.current ?? {}),
      ...pickPerServer(cfg),
      id: serverID,
      name: serverName.trim() || activeName,
    } as ServerConfig
    await api.servers.saveConfig(serverID, serverPayload)
  }

  const save = async () => {
    onSavingChange?.(true)
    try {
      await persist()
      toast.success(t('settings.configSaved'))
      // Re-fetch the server list so a rename (or add) reflects immediately in
      // the navbar dropdown and the dashboard cards.
      await refreshServers()
      await auth.refresh()
    }
    catch (e: unknown) {
      toast.danger(t('settings.saveFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      onSavingChange?.(false)
    }
  }

  // Expose save to the parent footer button only after config has loaded. Clear
  // the ref on unmount so a stale closure cannot fire after the form is gone.
  React.useEffect(() => {
    if (saveRef && !loading) {
      saveRef.current = save
      return () => {
        saveRef.current = null
      }
    }
  })

  // Add-server wizard plumbing: notify the wizard of config changes (so it can
  // drive discovery from the entered settings) and merge discovered values in.
  React.useEffect(() => {
    onConfigChange?.(cfg)
  }, [cfg, onConfigChange])
  React.useEffect(() => {
    if (prefill) void Promise.resolve().then(() => setCfg((prev) => ({ ...prev, ...prefill })))
  }, [prefill])

  if (loading) {
    return (
      <div className="flex items-center justify-center flex-1 gap-2 text-muted">
        <Spinner size="sm" color="current" />
        <span className="text-sm">{t('settings.loadingConfig')}</span>
      </div>
    )
  }

  // The Discord (guild-link) tab edits cross-server guild data, so it only makes
  // sense for a persisted server (id > 0) in the Settings/Manage view — never the
  // add-server wizard or a not-yet-created server.
  const showDiscord = settingsMode && !addMode && serverID > 0
  const SERVER_TABS = [
    { id: 'control', label: t('settings.tabs.control') },
    { id: 'ssh', label: t('settings.tabs.ssh') },
    { id: 'server', label: t('settings.tabs.server') },
    ...(showDiscord ? [{ id: 'discord', label: t('settings.tabs.discord') }] : []),
    { id: 'server-advanced', label: t('settings.tabs.advanced') },
  ]
  // Inline rename: shown in the per-server Settings/Manage view (not the wizard).
  const showNameField = settingsMode

  // Determine the variant for the advanced surface.
  const advancedVariant: ServerAdvancedVariant = addMode ? 'add' : (!settingsMode ? 'first-run' : 'manage')

  return (
    <form className="flex flex-col flex-1 min-h-0 gap-3" onSubmit={(e) => e.preventDefault()} autoComplete="off">
      {/* sr-only (not display:none) — Chrome's credential heuristic skips display:none elements */}
      <input type="text" autoComplete="username" aria-hidden="true" tabIndex={-1} readOnly className="sr-only" />
      {showNameField && (
        <Panel className="shrink-0">
          <FieldRow label={t('manage.serverName', 'Server name')}>
            <TextInput value={serverName} onChange={setServerName} placeholder={activeName || 'Production'} />
          </FieldRow>
        </Panel>
      )}
      {!hideTabBar && (
        <div className="shrink-0 flex flex-wrap items-center justify-end gap-2">
          <Segment
            selectedKey={tab}
            onSelectionChange={(k) => setInternalTab(String(k))}
            size="sm"
            className="w-fit"
          >
            {SERVER_TABS.map(({ id, label }) => (
              <Segment.Item key={id} id={id}>
                <Segment.Separator />
                {label}
              </Segment.Item>
            ))}
          </Segment>
        </div>
      )}

      {tab === 'control' && (
        <ControlPanel cfg={cfg} set={set} setBool={setBool} setControl={setControl} />
      )}

      {tab === 'ssh' && (
        <SshPanel cfg={cfg} set={set} />
      )}

      {/* Combined server tab: DB + broker. */}
      {tab === 'server' && (
        <ConnectionPanel cfg={cfg} set={set} setBool={setBool} showDb showBroker />
      )}

      {/* Standalone wizard 'db' step: database only. */}
      {tab === 'db' && (
        <ConnectionPanel cfg={cfg} set={set} setBool={setBool} showDb showBroker={false} />
      )}

      {/* Standalone wizard 'broker' step: broker only. */}
      {tab === 'broker' && (
        <ConnectionPanel cfg={cfg} set={set} setBool={setBool} showDb={false} showBroker />
      )}

      {tab === 'discord' && showDiscord && (
        <ServerDiscordPanel serverId={serverID} />
      )}

      {(tab === 'advanced' || tab === 'server-advanced') && (
        <ServerAdvancedPanel
          variant={advancedVariant}
          cfg={cfg}
          set={set}
          setBool={setBool}
          backendUrl={backendUrl}
          setBackendUrl={setBackendUrl}
          activeName={activeName}
          onRequestDeleteServer={onRequestDeleteServer}
        />
      )}
    </form>
  )
}
