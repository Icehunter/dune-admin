import { Label, NumberField } from '@heroui/react'

interface NumberInputProps {
  value: number
  onChange: (value: number) => void
  min?: number
  max?: number
  step?: number
  label?: string
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
      className={className}
    >
      {label && <Label className="text-xs text-muted">{label}</Label>}
      <NumberField.Group>
        {showButtons && <NumberField.DecrementButton />}
        <NumberField.Input />
        {showButtons && <NumberField.IncrementButton />}
      </NumberField.Group>
    </NumberField>
  )
}
