import * as React from 'react'
import { SearchField } from '@heroui/react'
import { FieldLabelContext } from './FieldRow'
import type { SearchableSelectProps } from './interfaces'

const MAX_VISIBLE = 60

// SearchableSelect is a type-to-filter dropdown for picking a single id from a
// known list (a guild, a channel) by its human name. It uses the same SearchField
// chrome as the app's other lookups — leading search icon + clear button — for
// visual consistency. If the current value isn't among the options (still
// loading, or access lost) its raw id is shown so a save never silently drops it.
export const SearchableSelect: React.FC<SearchableSelectProps> = ({
  value, onChange, options, placeholder, isDisabled, ariaLabel,
}) => {
  const fieldLabel = React.useContext(FieldLabelContext)
  const label = ariaLabel || fieldLabel || placeholder || 'select'
  const [query, setQuery] = React.useState('')
  const [open, setOpen] = React.useState(false)

  const selectedLabel = !value ? '' : (options.find((o) => o.id === value)?.label ?? value)

  // While closed, show the settled selection; while open, show the typed query.
  const displayValue = open ? query : selectedLabel

  const _q = query.trim().toLowerCase()
  const _base = _q
    ? options.filter((o) => o.label.toLowerCase().includes(_q) || o.id.includes(_q))
    : options
  const filtered = _base.slice(0, MAX_VISIBLE)

  const pick = (id: string) => {
    onChange(id)
    setQuery('')
    setOpen(false)
  }

  return (
    <div className="relative w-full">
      <SearchField
        className="w-full"
        value={displayValue}
        // Clearing the field (X button, or erasing the text) clears the selection.
        onChange={(v) => {
          setQuery(v)
          setOpen(true)
          if (v === '') onChange('')
        }}
        onFocus={() => {
          setQuery('')
          setOpen(true)
        }}
        onBlur={() => setTimeout(() => setOpen(false), 150)}
        {...(isDisabled !== undefined ? { isDisabled } : {})}
        aria-label={label}
      >
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input {...(placeholder !== undefined ? { placeholder } : {})} aria-label={label} />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>
      {open && filtered.length > 0 && (
        <div className="absolute z-50 w-full mt-1 rounded-[var(--radius)] border border-border bg-surface shadow-lg overflow-y-auto max-h-52">
          {filtered.map((o) => (
            <div
              key={o.id}
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => pick(o.id)}
              className={`px-3 py-1.5 text-sm cursor-pointer hover:bg-surface-hover${o.id === value ? ' text-accent font-medium' : ''}`}
            >
              {o.label}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
