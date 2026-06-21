import type { WelcomePackageItem } from '../../api/client'

export type WelcomeSection = 'config' | 'packages' | 'grants'

export type WelcomePackageTabProps = { section?: WelcomeSection }

export type { WelcomePackageItem }
