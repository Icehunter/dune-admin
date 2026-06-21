import * as React from 'react'
import type { EventDefinition } from '../../../api/client'

export interface EventEditorModalProps {
  isOpen: boolean
  onClose: () => void
  editing: EventDefinition | null
  onSaved: () => void
}

export interface FormSectionProps {
  children: React.ReactNode
  className?: string
}

export interface TagPickerFieldProps {
  value: string
  onSelect: (tag: string) => void
  options: string[]
  ariaLabel: string
}
