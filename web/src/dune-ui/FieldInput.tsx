import * as React from 'react'
import { Input } from '@heroui/react'
import type { FieldInputProps } from './interfaces'

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
    {...(placeholder !== undefined ? { placeholder } : {})}
    {...(ariaLabel !== undefined ? { 'aria-label': ariaLabel } : {})}
    {...(isDisabled !== undefined ? { disabled: isDisabled } : {})}
    {...(className !== undefined ? { className } : {})}
  />
)
