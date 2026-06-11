import type { WelcomePackage, WelcomeGrantRecord, WelcomePackageItem } from '../../api/client'

export type WelcomeSection = 'config' | 'packages' | 'grants'

export type WelcomePackageTabProps
  = | { showSubnav?: false, section?: WelcomeSection, onSectionChange?: never }
    | { showSubnav: true, section?: WelcomeSection, onSectionChange: (s: WelcomeSection) => void }

export interface WelcomeConfigDiff {
  packageAdded: number
  packageRemoved: number
  packageUpdated: number
  settingsChanged: boolean
  isDirty: boolean
}

export interface WelcomeSharedProps {
  // config state
  enabled: boolean
  setEnabled: (v: boolean) => void
  scanSecs: number
  setScanSecs: (v: number) => void
  packages: WelcomePackage[]
  setPackages: (ps: WelcomePackage[]) => void
  activeVersions: string[]
  setActiveVersions: (avs: string[] | ((prev: string[]) => string[])) => void
  // message state
  welcomeMessageEnabled: boolean
  setWelcomeMessageEnabled: (v: boolean) => void
  welcomeMessage: string
  setWelcomeMessage: (v: string) => void
  welcomeWhisperSourcePlayer: string
  setWelcomeWhisperSourcePlayer: (v: string) => void
  // MOTD state (per-join message, independent of the package)
  motdEnabled: boolean
  setMotdEnabled: (v: boolean) => void
  motdMessage: string
  setMotdMessage: (v: string) => void
  motdSourcePlayer: string
  setMotdSourcePlayer: (v: string) => void
  // actions
  save: () => Promise<void>
  runNow: () => Promise<void>
  saving: boolean
  running: boolean
  load: () => void
  loading: boolean
  // grants
  grants: WelcomeGrantRecord[]
  retry: (g: WelcomeGrantRecord) => Promise<void>
  revoke: (g: WelcomeGrantRecord) => Promise<void>
  // templates (packages view)
  templates: { id: string, name: string }[]
  // unsaved-changes diff
  configDiff: WelcomeConfigDiff
}

export type { WelcomePackageItem }
