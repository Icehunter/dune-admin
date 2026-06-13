import type { WelcomePackage, WelcomeGrantRecord, WelcomePackageItem } from '../../api/client'
import type * as React from 'react'

export type WelcomeSection = 'config' | 'packages' | 'grants'

export type WelcomePackageTabProps = { section?: WelcomeSection }

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
  // Region join/leave broadcast (whisper to everyone in the joined/left region)
  regionJoinEnabled: boolean
  setRegionJoinEnabled: (v: boolean) => void
  regionLeaveEnabled: boolean
  setRegionLeaveEnabled: (v: boolean) => void
  regionJoinTemplate: string
  setRegionJoinTemplate: (v: string) => void
  regionLeaveTemplate: string
  setRegionLeaveTemplate: (v: string) => void
  regionChatChannel: string
  setRegionChatChannel: (v: string) => void
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
  // manual grant override — force-grant a package to a chosen player
  override: (accountId: number, packageVersion: string) => Promise<void>
  // templates (packages view)
  templates: { id: string, name: string }[]
  // unsaved-changes diff
  configDiff: WelcomeConfigDiff
  // section nav rendered into each view's PageHeader (Segment)
  nav?: React.ReactNode
}

export type { WelcomePackageItem }
