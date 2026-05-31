import { Label, NumberField } from '@heroui/react'

interface NumberInputProps {
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
}

export function NumberInput({
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
}: NumberInputProps) {
  return (
    <NumberField
      value={value}
      onChange={(v) => onChange(v ?? min ?? 0)}
      minValue={min}
      maxValue={max}
      step={step}
      isDisabled={isDisabled}
      aria-label={ariaLabel ?? label}
      variant="secondary"
      fullWidth
      className={className}
    >
      {label && <Label className="text-xs text-muted">{label}</Label>}
      <NumberField.Group className="w-full">
        {prefix && (
          <span className="px-2 text-xs text-muted shrink-0 flex items-center border-r border-border">
            {prefix}
          </span>
        )}
        {showButtons && <NumberField.DecrementButton />}
        <NumberField.Input className="flex-1 min-w-0" style={{ textAlign: 'center' }} />
        {showButtons && <NumberField.IncrementButton />}
      </NumberField.Group>
    </NumberField>
  )
}
