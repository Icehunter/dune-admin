import * as React from 'react'
import { Input } from '@heroui/react'
import type { FieldInputProps } from './types'

export const FieldInput: React.FC<FieldInputProps> = ({
  value,
  onChange,
  placeholder,
  type = 'text',
  className,
  ariaLabel,
  isDisabled,
}) => (
  <Input
    type={type}
    value={value}
    onChange={(e) => onChange(e.target.value)}
    placeholder={placeholder}
    aria-label={ariaLabel}
    disabled={isDisabled}
    className={className}
  />
)
