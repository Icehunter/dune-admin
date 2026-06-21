import * as React from 'react'

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
