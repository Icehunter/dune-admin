import * as React from 'react'
import { Input } from '@heroui/react'
import { FieldLabelContext } from './FieldRow'
import type { TextInputProps } from '../../interfaces'

export const TextInput: React.FC<TextInputProps> = ({ value, onChange, placeholder, type = 'text', autoComplete }) => {
  const fieldLabel = React.useContext(FieldLabelContext)
  return (
    <Input
      className="font-mono"
      type={type}
      value={String(value)}
      onChange={(e) => onChange(e.target.value)}
      {...(placeholder !== undefined ? { placeholder } : {})}
      aria-label={fieldLabel || placeholder || 'value'}
      autoComplete={autoComplete ?? (type === 'password' ? 'new-password' : 'off')}
    />
  )
}
