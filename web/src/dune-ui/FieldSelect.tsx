import * as React from 'react'
import { Select, ListBox } from '@heroui/react'
import type { FieldSelectProps } from './interfaces'

// FieldSelect wraps HeroUI Select + ListBox for small, fixed option sets.
// For large lists (e.g. 400 IANA timezones), keep native <select> for type-to-search.
export const FieldSelect: React.FC<FieldSelectProps> = ({
  value,
  onChange,
  options,
  className,
  ariaLabel,
  isDisabled,
}) => (
  <Select
    selectedKey={value}
    onSelectionChange={(k) => onChange(String(k))}
    {...(ariaLabel !== undefined ? { 'aria-label': ariaLabel } : {})}
    {...(isDisabled !== undefined ? { isDisabled } : {})}
    {...(className !== undefined ? { className } : {})}
  >
    <Select.Trigger>
      <Select.Value />
      <Select.Indicator />
    </Select.Trigger>
    <Select.Popover>
      <ListBox>
        {options.map((opt) => (
          <ListBox.Item key={opt} id={opt} textValue={opt}>
            {opt}
            <ListBox.ItemIndicator />
          </ListBox.Item>
        ))}
      </ListBox>
    </Select.Popover>
  </Select>
)
