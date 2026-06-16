import * as React from 'react'

export type TabId
  = | 'dashboard'
    | 'battlegroup'
    | 'players'
    | 'database'
    | 'logs'
    | 'blueprints'
    | 'bases'
    | 'guilds'
    | 'landsraad'
    | 'storage'
    | 'livemap'
    | 'server'
    | 'director'
    | 'market'
    | 'welcome'
    | 'events'
    | 'battlepass'
    | 'permissions'
    | 'diagnostics'

export type DbSection = 'backups' | 'tables' | 'describe' | 'sample' | 'search' | 'sql'
export type WelcomeSection = 'config' | 'packages' | 'grants'

export interface AppCoreProps {
  isSignedIn: boolean
}

export interface TabPaneProps {
  active: boolean
  children: React.ReactNode
}

export interface ConnectionBadgeProps {
  label: string
  connected: boolean
}
