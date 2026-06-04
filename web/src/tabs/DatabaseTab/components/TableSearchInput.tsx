import { useMemo, useState } from 'react'
import { SearchField } from '@heroui/react'

interface TableSearchInputProps {
  value: string
  onChange: (v: string) => void
  onRun: () => void
  tableNames: string[]
  ariaLabel: string
  placeholder: string
}

export function TableSearchInput(
  { value, onChange, onRun, tableNames, ariaLabel, placeholder }: TableSearchInputProps,
) {
  const [open, setOpen] = useState(false)

  const filtered = useMemo(() => {
    const q = value.toLowerCase().trim()
    if (!q) return tableNames.slice(0, 40)
    return tableNames.filter((n) => n.toLowerCase().includes(q))
  }, [value, tableNames])

  const pick = (name: string) => {
    onChange(name)
    setOpen(false)
  }

  return (
    <div
      className="relative flex-1 max-w-md"
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
          setOpen(false)
        }
      }}
    >
      <SearchField
        className="w-full"
        value={value}
        onChange={(v) => {
          onChange(v)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        aria-label={ariaLabel}
      >
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input
            placeholder={placeholder}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                setOpen(false)
                onRun()
              }
              if (e.key === 'Escape') setOpen(false)
              if (e.key === 'ArrowDown') setOpen(true)
            }}
          />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>
      {open && filtered.length > 0 && (
        <div className="absolute z-50 w-full mt-1 rounded-[var(--radius)] border border-border bg-surface overflow-y-auto max-h-52 shadow-lg">
          {filtered.map((n) => (
            <button
              key={n}
              type="button"
              className="w-full text-left px-3 py-1.5 text-xs cursor-pointer hover:bg-surface-hover"
              onMouseDown={(e) => {
                e.preventDefault()
                pick(n)
              }}
            >
              <span className="text-muted mr-0.5">dune.</span>
              <span className="font-mono">{n}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
