import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Spinner, toast } from '@heroui/react'
import { Segment } from '@heroui-pro/react'
import { api } from '../../../api/client'
import type { AppConfig } from '../../../api/client'
import { AuthContext } from '../../../auth/context'
import { EMPTY, mergeConfig, pickGlobal } from '../config'
import { AuthPanel } from './AuthPanel'
import { DiscordPanel } from './DiscordPanel'
import { GuildsPanel } from './GuildsPanel'
import { AdminAdvancedPanel } from './AdminAdvancedPanel'
import type { DiscordRole } from '../../interfaces'
import type { GlobalSettingsFormProps } from './interfaces'

// GlobalSettingsForm edits the dune-admin (global) configuration: auth, the
// Discord bot, and admin-level advanced settings. Loads via api.config.get();
// saves only the global fields via api.config.save(..., true).
export const GlobalSettingsForm: React.FC<GlobalSettingsFormProps> = ({ saveRef, onSavingChange, initialTab }) => {
  const { t } = useTranslation()
  const auth = React.useContext(AuthContext)

  const [cfg, setCfg] = React.useState<AppConfig>(EMPTY)
  const [loading, setLoading] = React.useState(true)
  const [tab, setTab] = React.useState(initialTab ?? 'auth')
  const [backendUrl, setBackendUrl] = React.useState(() => localStorage.getItem('dune_admin_backend') || '')

  // Raw global base kept so the save reconstructs the payload without clobbering
  // the flat per-server fields the global config endpoint returned.
  const globalBaseRef = React.useRef<Partial<AppConfig>>({})

  const [discordRoles, setDiscordRoles] = React.useState<DiscordRole[]>([])
  const [rolesLoading, setRolesLoading] = React.useState(false)

  React.useEffect(() => {
    const onErr = (e: unknown) =>
      toast.danger(t('settings.loadFailed', { message: e instanceof Error ? e.message : String(e) }))
    api.config.get()
      .then((c) => {
        globalBaseRef.current = c as Partial<AppConfig>
        setCfg(mergeConfig(c as Record<string, unknown>))
      })
      .catch(onErr)
      .finally(() => setLoading(false))
  }, [t])

  const loadDiscordRoles = (): void => {
    setRolesLoading(true)
    api.discord.roles()
      .then(setDiscordRoles)
      .catch(() => setDiscordRoles([]))
      .finally(() => setRolesLoading(false))
  }

  React.useEffect(() => {
    Promise.resolve().then(loadDiscordRoles)
  }, [])

  React.useEffect(() => {
    if (tab === 'auth') Promise.resolve().then(loadDiscordRoles)
  }, [tab])

  const set = (key: keyof AppConfig) => (v: string) =>
    setCfg((prev) => ({
      ...prev,
      [key]: key === 'scrip_currency' || key === 'discord_status_interval_seconds' || key === 'auth_session_ttl_hours'
        ? (Number(v) || 0)
        : v,
    }))

  const setBool = (key: keyof AppConfig) => (v: boolean) =>
    setCfg((prev) => ({ ...prev, [key]: v }))

  const save = async () => {
    onSavingChange?.(true)
    const authToggled = auth.enabled !== cfg.auth_enabled
    try {
      await api.config.save({ ...globalBaseRef.current, ...pickGlobal(cfg) } as AppConfig, true)
      toast.success(t('settings.configSaved'))
      // Toggling authentication clears the session cookie server-side; reset the
      // route to the Dashboard and force a full reload so the SPA re-bootstraps
      // from a clean slate.
      if (authToggled) {
        window.location.hash = '#/dashboard'
        window.location.reload()
        return
      }
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

  if (loading) {
    return (
      <div className="flex items-center justify-center flex-1 gap-2 text-muted">
        <Spinner size="sm" color="current" />
        <span className="text-sm">{t('settings.loadingConfig')}</span>
      </div>
    )
  }

  const ADMIN_TABS = [
    { id: 'auth', label: t('settings.tabs.auth') },
    { id: 'discord', label: t('settings.tabs.discord') },
    { id: 'guilds', label: t('settings.tabs.guilds') },
    { id: 'admin-advanced', label: t('settings.tabs.advanced') },
  ]

  return (
    <form className="flex flex-col flex-1 min-h-0 gap-3" onSubmit={(e) => e.preventDefault()} autoComplete="off">
      {/* sr-only (not display:none) — Chrome's credential heuristic skips display:none elements */}
      <input type="text" autoComplete="username" aria-hidden="true" tabIndex={-1} readOnly className="sr-only" />
      <div className="shrink-0 flex flex-wrap items-center justify-end gap-2">
        <Segment
          selectedKey={tab}
          onSelectionChange={(k) => setTab(String(k))}
          size="sm"
          className="w-fit"
        >
          {ADMIN_TABS.map(({ id, label }) => (
            <Segment.Item key={id} id={id}>
              <Segment.Separator />
              {label}
            </Segment.Item>
          ))}
        </Segment>
      </div>

      {tab === 'auth' && (
        <AuthPanel
          cfg={cfg}
          set={set}
          setBool={setBool}
          discordRoles={discordRoles}
          rolesLoading={rolesLoading}
          loadDiscordRoles={loadDiscordRoles}
        />
      )}
      {tab === 'discord' && (
        <DiscordPanel
          cfg={cfg}
          set={set}
          setBool={setBool}
        />
      )}
      {tab === 'guilds' && (
        <GuildsPanel />
      )}
      {tab === 'admin-advanced' && (
        <AdminAdvancedPanel
          cfg={cfg}
          set={set}
          backendUrl={backendUrl}
          setBackendUrl={setBackendUrl}
        />
      )}
    </form>
  )
}
