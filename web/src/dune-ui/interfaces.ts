import * as React from 'react'

export interface FieldSelectProps {
  value: string
  onChange: (v: string) => void
  options: string[]
  className?: string
  ariaLabel?: string
  isDisabled?: boolean
}

export interface FieldInputProps {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  type?: 'text' | 'number' | 'password' | 'email' | 'url'
  className?: string
  ariaLabel?: string
  isDisabled?: boolean
}

export interface NumberInputProps {
  value: number
  onChange: (value: number) => void
  min?: number
  max?: number
  step?: number
  label?: string
  prefix?: string
  ariaLabel?: string
  isDisabled?: boolean
  className?: string
  showButtons?: boolean
  formatOptions?: Intl.NumberFormatOptions | undefined
}

export interface SectionLabelProps {
  children: React.ReactNode
}
