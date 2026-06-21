import type { WelcomeSharedProps } from '../interfaces'
import type { WelcomePackageItem } from '../types'

export type GrantKey = 'character' | 'fls' | 'version' | 'status' | 'attempts' | 'updated' | 'error' | 'actions'

export type GrantsViewProps = Pick<
  WelcomeSharedProps,
  'grants' | 'retry' | 'revoke' | 'override' | 'packages' | 'activeVersions' | 'load' | 'loading' | 'nav'
>

export type ConfigViewProps = Pick<
  WelcomeSharedProps,
  | 'enabled' | 'setEnabled'
  | 'scanSecs' | 'setScanSecs'
  | 'packages'
  | 'activeVersions' | 'setActiveVersions'
  | 'welcomeMessageEnabled' | 'setWelcomeMessageEnabled'
  | 'welcomeMessage' | 'setWelcomeMessage'
  | 'welcomeWhisperSourcePlayer' | 'setWelcomeWhisperSourcePlayer'
  | 'motdEnabled' | 'setMotdEnabled'
  | 'motdMessage' | 'setMotdMessage'
  | 'motdSourcePlayer' | 'setMotdSourcePlayer'
  | 'regionJoinEnabled' | 'setRegionJoinEnabled'
  | 'regionLeaveEnabled' | 'setRegionLeaveEnabled'
  | 'regionJoinTemplate' | 'setRegionJoinTemplate'
  | 'regionLeaveTemplate' | 'setRegionLeaveTemplate'
  | 'regionChatChannel' | 'setRegionChatChannel'
  | 'save' | 'saving'
  | 'runNow' | 'running'
  | 'load' | 'loading'
  | 'configDiff'
  | 'nav'
>

export type PackagesViewProps = Pick<
  WelcomeSharedProps,
  'packages' | 'setPackages' | 'activeVersions' | 'templates' | 'save' | 'saving' | 'load' | 'loading' | 'configDiff' | 'nav'
>

export type KeyedItem = WelcomePackageItem & { _key: string }
