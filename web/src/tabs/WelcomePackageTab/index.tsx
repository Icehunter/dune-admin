import * as React from 'react'
import { toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../api/client'
import type { WelcomePackage, WelcomePackageConfig, WelcomeGrantRecord } from '../../api/client'
import { SideNav } from '../../dune-ui'
import type { WelcomeSection, WelcomeConfigDiff, WelcomePackageTabProps } from './types'
import { ConfigView } from './views/ConfigView'
import { PackagesView } from './views/PackagesView'
import { GrantsView } from './views/GrantsView'

export const WelcomePackageTab: React.FC<WelcomePackageTabProps> = ({ showSubnav, section = 'config', onSectionChange }: WelcomePackageTabProps) => {
  const { t } = useTranslation()

  const SECTIONS: { key: WelcomeSection, label: string }[] = [
    { key: 'config', label: t('welcome.sections.config') },
    { key: 'packages', label: t('welcome.sections.packages') },
    { key: 'grants', label: t('welcome.sections.grants') },
  ]

  const [grants, setGrants] = React.useState<WelcomeGrantRecord[]>([])
  const [loading, setLoading] = React.useState(false)
  const [saving, setSaving] = React.useState(false)
  const [running, setRunning] = React.useState(false)

  const [enabled, setEnabled] = React.useState(false)
  const [scanSecs, setScanSecs] = React.useState(30)
  const [packages, setPackages] = React.useState<WelcomePackage[]>([])
  const [activeVersions, setActiveVersions] = React.useState<string[]>([])
  const [welcomeMessageEnabled, setWelcomeMessageEnabled] = React.useState(false)
  const [welcomeMessage, setWelcomeMessage] = React.useState('')
  const [welcomeWhisperSourcePlayer, setWelcomeWhisperSourcePlayer] = React.useState('')
  const [motdEnabled, setMotdEnabled] = React.useState(false)
  const [motdMessage, setMotdMessage] = React.useState('')
  const [motdSourcePlayer, setMotdSourcePlayer] = React.useState('')
  const [templates, setTemplates] = React.useState<{ id: string, name: string }[]>([])

  // Snapshot of what's persisted on the server; null until first load completes.
  const [savedConfig, setSavedConfig] = React.useState<WelcomePackageConfig | null>(null)

  const applyConfig = React.useCallback((c: WelcomePackageConfig) => {
    setEnabled(c.enabled)
    setScanSecs(c.scan_interval_secs)
    setPackages(c.packages ?? [])
    const avs = c.active_versions?.length
      ? c.active_versions
      : c.active_version ? [c.active_version] : []
    setActiveVersions(avs)
    setWelcomeMessageEnabled(c.welcome_message_enabled ?? false)
    setWelcomeMessage(c.welcome_message ?? '')
    setWelcomeWhisperSourcePlayer(c.welcome_whisper_source_player ?? '')
    setMotdEnabled(c.motd_enabled ?? false)
    setMotdMessage(c.motd_message ?? '')
    setMotdSourcePlayer(c.motd_source_player ?? '')
  }, [])

  const load = React.useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.welcomePackage.config())
      .then((c) => {
        applyConfig(c)
        setSavedConfig(c)
      })
      .then(() => api.welcomePackage.grants(100))
      .then(setGrants)
      .catch((e: unknown) => {
        const msg = e instanceof Error ? e.message : String(e)
        toast.danger(t('welcome.failedToLoad', { message: msg }))
      })
      .finally(() => setLoading(false))
  }, [t, applyConfig])

  React.useEffect(() => {
    load()
  }, [load])

  React.useEffect(() => {
    api.players.templates().then(setTemplates).catch(() => {})
  }, [])

  const save = async () => {
    setSaving(true)
    try {
      const cfg: WelcomePackageConfig = {
        enabled,
        scan_interval_secs: scanSecs,
        active_version: activeVersions[0] ?? '',
        active_versions: activeVersions,
        packages,
        welcome_message_enabled: welcomeMessageEnabled,
        welcome_message: welcomeMessage,
        welcome_whisper_source_player: welcomeWhisperSourcePlayer,
        motd_enabled: motdEnabled,
        motd_message: motdMessage,
        motd_source_player: motdSourcePlayer,
      }
      const saved = await api.welcomePackage.saveConfig(cfg)
      applyConfig(saved)
      setSavedConfig(saved)
      toast.success(enabled
        ? t('welcome.savedEnabled', { version: activeVersions.join(', ') })
        : t('welcome.savedDisabled'))
    }
    catch (e) {
      toast.danger(t('welcome.saveFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setSaving(false)
    }
  }

  const runNow = async () => {
    setRunning(true)
    try {
      const r = await api.welcomePackage.run()
      toast.success(t('welcome.scanComplete', { granted: r.granted, failed: r.failed, skipped: r.skipped }))
      setGrants(await api.welcomePackage.grants(100))
    }
    catch (e) {
      toast.danger(t('welcome.runFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setRunning(false)
    }
  }

  const retry = async (g: WelcomeGrantRecord) => {
    try {
      await api.welcomePackage.retry(g.fls_id, g.package_version, g.account_id)
      toast.success(t('welcome.retryCleared'))
      setGrants(await api.welcomePackage.grants(100))
    }
    catch (e) {
      toast.danger(t('welcome.retryFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
  }

  const revoke = async (g: WelcomeGrantRecord) => {
    try {
      await api.welcomePackage.revoke(g.fls_id, g.package_version, g.account_id)
      toast.success(t('welcome.revoked'))
      setGrants(await api.welcomePackage.grants(100))
    }
    catch (e) {
      toast.danger(t('welcome.revokeFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
  }

  const configDiff = React.useMemo((): WelcomeConfigDiff => {
    if (!savedConfig) {
      return { packageAdded: 0, packageRemoved: 0, packageUpdated: 0, settingsChanged: false, isDirty: false }
    }
    const savedPkgs = savedConfig.packages ?? []
    const savedPkgMap = new Map(savedPkgs.map((p) => [p.version, p]))
    const currentPkgIds = new Set(packages.map((p) => p.version))

    const packageAdded = packages.filter((p) => !savedPkgMap.has(p.version)).length
    const packageRemoved = savedPkgs.filter((p) => !currentPkgIds.has(p.version)).length
    const packageUpdated = packages.filter((p) => {
      if (!savedPkgMap.has(p.version)) return false
      return JSON.stringify(p) !== JSON.stringify(savedPkgMap.get(p.version))
    }).length

    const savedVersions = [...(savedConfig.active_versions ?? [])].sort()
    const settingsChanged
      = enabled !== savedConfig.enabled
        || scanSecs !== savedConfig.scan_interval_secs
        || JSON.stringify([...activeVersions].sort()) !== JSON.stringify(savedVersions)
        || welcomeMessageEnabled !== (savedConfig.welcome_message_enabled ?? false)
        || welcomeMessage !== (savedConfig.welcome_message ?? '')
        || welcomeWhisperSourcePlayer !== (savedConfig.welcome_whisper_source_player ?? '')
        || motdEnabled !== (savedConfig.motd_enabled ?? false)
        || motdMessage !== (savedConfig.motd_message ?? '')
        || motdSourcePlayer !== (savedConfig.motd_source_player ?? '')

    const isDirty = packageAdded + packageRemoved + packageUpdated > 0 || settingsChanged
    return { packageAdded, packageRemoved, packageUpdated, settingsChanged, isDirty }
  }, [
    packages,
    enabled,
    scanSecs,
    activeVersions,
    welcomeMessageEnabled,
    welcomeMessage,
    welcomeWhisperSourcePlayer,
    motdEnabled,
    motdMessage,
    motdSourcePlayer,
    savedConfig,
  ])

  const activeView = () => {
    switch (section) {
      case 'config':
        return (
          <ConfigView
            enabled={enabled}
            setEnabled={setEnabled}
            scanSecs={scanSecs}
            setScanSecs={setScanSecs}
            packages={packages}
            activeVersions={activeVersions}
            setActiveVersions={setActiveVersions}
            welcomeMessageEnabled={welcomeMessageEnabled}
            setWelcomeMessageEnabled={setWelcomeMessageEnabled}
            welcomeMessage={welcomeMessage}
            setWelcomeMessage={setWelcomeMessage}
            welcomeWhisperSourcePlayer={welcomeWhisperSourcePlayer}
            setWelcomeWhisperSourcePlayer={setWelcomeWhisperSourcePlayer}
            motdEnabled={motdEnabled}
            setMotdEnabled={setMotdEnabled}
            motdMessage={motdMessage}
            setMotdMessage={setMotdMessage}
            motdSourcePlayer={motdSourcePlayer}
            setMotdSourcePlayer={setMotdSourcePlayer}
            save={save}
            saving={saving}
            runNow={runNow}
            running={running}
            load={load}
            loading={loading}
            configDiff={configDiff}
          />
        )
      case 'packages':
        return (
          <PackagesView
            packages={packages}
            setPackages={setPackages}
            activeVersions={activeVersions}
            templates={templates}
            save={save}
            saving={saving}
            load={load}
            loading={loading}
            configDiff={configDiff}
          />
        )
      case 'grants':
        return <GrantsView grants={grants} retry={retry} revoke={revoke} load={load} loading={loading} />
    }
  }

  if (showSubnav) {
    return (
      <div className="h-full min-h-0 flex gap-3">
        <SideNav
          title={t('welcome.title')}
          items={SECTIONS}
          active={section}
          onSelect={(k) => onSectionChange?.(k as WelcomeSection)}
        />
        <div className="flex-1 min-h-0 flex flex-col">
          {activeView()}
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full min-h-0">
      {activeView()}
    </div>
  )
}
