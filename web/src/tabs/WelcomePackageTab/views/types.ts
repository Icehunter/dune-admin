import type { WelcomeSharedProps, WelcomePackageItem } from '../types'

export type GrantKey = 'character' | 'fls' | 'version' | 'status' | 'attempts' | 'updated' | 'error' | 'actions'

export type GrantsViewProps = Pick<WelcomeSharedProps, 'grants' | 'retry' | 'revoke' | 'load' | 'loading'>

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
  | 'save' | 'saving'
  | 'runNow' | 'running'
  | 'load' | 'loading'
  | 'configDiff'
>

export type PackagesViewProps = Pick<
  WelcomeSharedProps,
  'packages' | 'setPackages' | 'activeVersions' | 'templates' | 'save' | 'saving' | 'load' | 'loading' | 'configDiff'
>

export type KeyedItem = WelcomePackageItem & { _key: string }
