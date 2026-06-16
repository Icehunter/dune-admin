import * as React from 'react'

export interface TimezoneSelectProps {
  value: string
  onChange: (v: string) => void
  className?: string
}

export interface BackendUnreachableProps {
  onRetry: () => void
}

export interface FieldProps {
  label: string
  hint?: string
  children: React.ReactNode
}

export interface TextInputProps {
  value: string | number
  onChange: (v: string) => void
  placeholder?: string
  type?: string
  autoComplete?: string
}

export interface CheckboxFieldProps {
  label: string
  checked: boolean
  onChange: (v: boolean) => void
  hint?: string
}

export interface GridRowProps {
  children: React.ReactNode
}

export interface DiscordRole { id: string, name: string }

export interface RolePickerProps {
  value: string
  onChange: (v: string) => void
  roles: DiscordRole[]
  label: string
  hint?: string
}
