import * as React from 'react'
import { SearchField } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import type { TimezoneSelectProps } from './interfaces'

// IANA timezone names from the browser when available (Chrome 99+/modern), with
// a small fallback for older runtimes. Computed once at module load.
const tzList = (): string[] => {
  const fn = (Intl as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf
  try {
    if (typeof fn === 'function') return fn('timeZone')
  }
  catch { /* fall through to fallback */ }
  return [
    'UTC', 'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
    'America/Sao_Paulo', 'Europe/London', 'Europe/Berlin', 'Europe/Paris', 'Europe/Moscow',
    'Asia/Tokyo', 'Asia/Shanghai', 'Asia/Kolkata', 'Australia/Sydney',
  ]
}

const ZONES = tzList()
const MAX_VISIBLE = 60

// When closed, displayValue is derived from value prop — no local state needed.
// When open, query drives the filter and the SearchField input.
export const TimezoneSelect: React.FC<TimezoneSelectProps> = ({ value, onChange, className }) => {
  const { t } = useTranslation()
  const hostLabel = t('common.tzHostLocal')

  const allOptions = [{ key: '', label: hostLabel }, ...ZONES.map((z) => ({ key: z, label: z }))]

  const [query, setQuery] = React.useState('')
  const [open, setOpen] = React.useState(false)

  // While closed, show the settled value; while open, show what the user is typing.
  const displayValue = open ? query : (value === '' ? hostLabel : value)

  const _tzq = query.trim().toLowerCase()
  const filtered = !_tzq
    ? allOptions.slice(0, MAX_VISIBLE)
    : allOptions.filter(({ label }) => label.toLowerCase().includes(_tzq)).slice(0, MAX_VISIBLE)

  const pick = (key: string) => {
    onChange(key)
    setOpen(false)
  }

  const handleFocus = () => {
    setQuery(value === '' ? hostLabel : value)
    setOpen(true)
  }

  const handleChange = (v: string) => {
    setQuery(v)
    setOpen(true)
  }

  return (
    <div className={`relative ${className ?? ''}`}>
      <SearchField
        className="w-full"
        value={displayValue}
        onChange={handleChange}
        onFocus={handleFocus}
        onBlur={() => setTimeout(() => setOpen(false), 150)}
        aria-label={t('common.timezone')}
      >
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input placeholder={hostLabel} />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>
      {open && filtered.length > 0 && (
        <div className="absolute z-50 w-full mt-1 rounded-[var(--radius)] border border-border bg-surface shadow-lg overflow-y-auto max-h-52">
          {filtered.map(({ key, label }) => (
            <div
              key={key || '__host__'}
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => pick(key)}
              className={`px-3 py-1.5 text-xs cursor-pointer hover:bg-surface-hover${key === value ? ' text-accent font-medium' : ''}`}
            >
              {label}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
