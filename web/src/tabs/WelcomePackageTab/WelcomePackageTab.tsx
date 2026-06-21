import * as React from 'react'
import { toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { Segment } from '@heroui-pro/react'
import { api } from '../../api/client'
import type { WelcomePackage, WelcomePackageConfig, WelcomeGrantRecord } from '../../api/client'
import type { WelcomeConfigDiff } from './interfaces'
import type { WelcomeSection, WelcomePackageTabProps } from './types'
import { ConfigView } from './views/ConfigView'
import { PackagesView } from './views/PackagesView'
import { GrantsView } from './views/GrantsView'

export const WelcomePackageTab: React.FC<WelcomePackageTabProps> = ({ section: initialSection = 'config' }: WelcomePackageTabProps) => {
  const { t } = useTranslation()

  const [section, setSection] = React.useState<WelcomeSection>(initialSection)

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
  const [regionJoinEnabled, setRegionJoinEnabled] = React.useState(false)
  const [regionLeaveEnabled, setRegionLeaveEnabled] = React.useState(false)
  const [regionJoinTemplate, setRegionJoinTemplate] = React.useState('')
  const [regionLeaveTemplate, setRegionLeaveTemplate] = React.useState('')
  const [regionChatChannel, setRegionChatChannel] = React.useState('whisper')
  const [templates, setTemplates] = React.useState<{ id: string, name: string }[]>([])

  // Snapshot of what's persisted on the server; null until first load completes.
  const [savedConfig, setSavedConfig] = React.useState<WelcomePackageConfig | null>(null)

  const applyConfig = (c: WelcomePackageConfig): void => {
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
    setRegionJoinEnabled(c.region_join_enabled ?? false)
    setRegionLeaveEnabled(c.region_leave_enabled ?? false)
    setRegionJoinTemplate(c.region_join_template ?? '')
    setRegionLeaveTemplate(c.region_leave_template ?? '')
    setRegionChatChannel(c.region_chat_channel ?? 'whisper')
  }

  const load = (): void => {
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
  }

  React.useEffect(() => {
    load()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

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
        region_join_enabled: regionJoinEnabled,
        region_leave_enabled: regionLeaveEnabled,
        region_join_template: regionJoinTemplate,
        region_leave_template: regionLeaveTemplate,
        region_chat_channel: regionChatChannel,
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

  const override = async (accountId: number, packageVersion: string) => {
    const r = await api.welcomePackage.override(accountId, packageVersion)
    toast.success(t('welcome.overrideGranted', { name: r.character_name || `#${r.account_id}`, version: packageVersion }))
    setGrants(await api.welcomePackage.grants(100))
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

  let configDiff: WelcomeConfigDiff
  if (!savedConfig) {
    configDiff = { packageAdded: 0, packageRemoved: 0, packageUpdated: 0, settingsChanged: false, isDirty: false }
  }
  else {
    const _savedPkgs = savedConfig.packages ?? []
    const _savedPkgMap = new Map(_savedPkgs.map((p) => [p.version, p]))
    const _currentPkgIds = new Set(packages.map((p) => p.version))

    const _packageAdded = packages.filter((p) => !_savedPkgMap.has(p.version)).length
    const _packageRemoved = _savedPkgs.filter((p) => !_currentPkgIds.has(p.version)).length
    const _packageUpdated = packages.filter((p) => {
      if (!_savedPkgMap.has(p.version)) return false
      return JSON.stringify(p) !== JSON.stringify(_savedPkgMap.get(p.version))
    }).length

    const _savedVersions = [...(savedConfig.active_versions ?? [])].sort()
    const _settingsChanged
      = enabled !== savedConfig.enabled
        || scanSecs !== savedConfig.scan_interval_secs
        || JSON.stringify([...activeVersions].sort()) !== JSON.stringify(_savedVersions)
        || welcomeMessageEnabled !== (savedConfig.welcome_message_enabled ?? false)
        || welcomeMessage !== (savedConfig.welcome_message ?? '')
        || welcomeWhisperSourcePlayer !== (savedConfig.welcome_whisper_source_player ?? '')
        || motdEnabled !== (savedConfig.motd_enabled ?? false)
        || motdMessage !== (savedConfig.motd_message ?? '')
        || motdSourcePlayer !== (savedConfig.motd_source_player ?? '')
        || regionJoinEnabled !== (savedConfig.region_join_enabled ?? false)
        || regionLeaveEnabled !== (savedConfig.region_leave_enabled ?? false)
        || regionJoinTemplate !== (savedConfig.region_join_template ?? '')
        || regionLeaveTemplate !== (savedConfig.region_leave_template ?? '')
        || regionChatChannel !== (savedConfig.region_chat_channel ?? 'whisper')

    const _isDirty = _packageAdded + _packageRemoved + _packageUpdated > 0 || _settingsChanged
    configDiff = {
      packageAdded: _packageAdded,
      packageRemoved: _packageRemoved,
      packageUpdated: _packageUpdated,
      settingsChanged: _settingsChanged,
      isDirty: _isDirty,
    }
  }

  const sectionNav = (
    <Segment
      selectedKey={section}
      onSelectionChange={(k) => setSection(k as WelcomeSection)}
      size="sm"
      aria-label={t('welcome.title')}
    >
      {SECTIONS.map((s) => (
        <Segment.Item key={s.key} id={s.key}>
          <Segment.Separator />
          {s.label}
        </Segment.Item>
      ))}
    </Segment>
  )

  const activeView = () => {
    switch (section) {
      case 'config':
        return (
          <ConfigView
            nav={sectionNav}
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
            regionJoinEnabled={regionJoinEnabled}
            setRegionJoinEnabled={setRegionJoinEnabled}
            regionLeaveEnabled={regionLeaveEnabled}
            setRegionLeaveEnabled={setRegionLeaveEnabled}
            regionJoinTemplate={regionJoinTemplate}
            setRegionJoinTemplate={setRegionJoinTemplate}
            regionLeaveTemplate={regionLeaveTemplate}
            setRegionLeaveTemplate={setRegionLeaveTemplate}
            regionChatChannel={regionChatChannel}
            setRegionChatChannel={setRegionChatChannel}
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
            nav={sectionNav}
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
        return (
          <GrantsView
            nav={sectionNav}
            grants={grants}
            retry={retry}
            revoke={revoke}
            override={override}
            packages={packages}
            activeVersions={activeVersions}
            load={load}
            loading={loading}
          />
        )
    }
  }

  return (
    <div className="flex flex-col h-full min-h-0">
      {activeView()}
    </div>
  )
}
