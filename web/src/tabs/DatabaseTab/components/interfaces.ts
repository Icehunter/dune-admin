import type * as React from 'react'

export interface TableSearchInputProps {
  value: string
  onChange: (v: string) => void
  onRun: () => void
  tableNames: string[]
  ariaLabel: string
  placeholder: string
}

export interface BackupsViewProps {
  onRegisterRefresh?: (fn: () => void) => void
  headerContent?: React.ReactNode | undefined
}
