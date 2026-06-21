import * as React from 'react'
import { Label, NumberField } from '@heroui/react'
import type { NumberInputProps } from './interfaces'

export const NumberInput: React.FC<NumberInputProps> = ({
  value,
  onChange,
  min,
  max,
  step = 1,
  label,
  prefix,
  ariaLabel,
  isDisabled,
  className,
  showButtons = true,
  formatOptions,
}) => {
  const fieldClassName = prefix ? 'flex-1 min-w-0' : className
  const field = (
    <NumberField
      value={value}
      onChange={(v) => onChange(v ?? min ?? 0)}
      {...(min !== undefined ? { minValue: min } : {})}
      {...(max !== undefined ? { maxValue: max } : {})}
      step={step}
      {...(isDisabled !== undefined ? { isDisabled } : {})}
      aria-label={ariaLabel ?? label ?? prefix ?? ''}
      variant="secondary"
      {...(fieldClassName !== undefined ? { className: fieldClassName } : {})}
      {...(formatOptions !== undefined ? { formatOptions } : {})}
    >
      {label && <Label className="text-xs text-muted">{label}</Label>}
      <NumberField.Group
        className="w-full"
        style={prefix
          ? { width: '100%', display: 'flex', alignItems: 'center', borderTopLeftRadius: 0, borderBottomLeftRadius: 0, borderLeft: 'none' }
          : { width: '100%', display: 'flex', alignItems: 'center' }}
      >
        {showButtons && <NumberField.DecrementButton />}
        <NumberField.Input className="flex-1" style={{ flexGrow: 1, minWidth: 40 }} />
        {showButtons && <NumberField.IncrementButton />}
      </NumberField.Group>
    </NumberField>
  )

  if (!prefix) return field

  return (
    <div className={`flex items-stretch ${className ?? ''}`}>
      <span className="px-2 text-xs text-muted shrink-0 flex items-center border border-r-0 border-border rounded-l-[var(--radius)] bg-surface-secondary">
        {prefix}
      </span>
      {field}
    </div>
  )
}
