import * as React from 'react'
import { SearchField } from '@heroui/react'
import type { TagPickerFieldProps } from './interfaces'

export const TagPickerField: React.FC<TagPickerFieldProps> = ({ value, onSelect, options, ariaLabel }) => {
  const [query, setQuery] = React.useState('')

  const _tpq = query.toLowerCase()
  const filtered = !query ? [] : options.filter((t) => t.toLowerCase().includes(_tpq)).slice(0, 100)

  const handleSelect = (tag: string) => {
    onSelect(tag)
    setQuery('')
  }

  return (
    <React.Fragment>
      {value && (
        <div className="mb-1 flex items-center gap-1">
          <span className="font-mono text-xs text-foreground">{value}</span>
          <button
            type="button"
            className="text-xs text-muted hover:text-foreground ml-1"
            onClick={() => {
              onSelect('')
              setQuery('')
            }}
          >
            ×
          </button>
        </div>
      )}
      {/* Inline, absolutely-positioned dropdown anchored to the search field —
          mirrors the reward-template picker below. A portal to document.body
          lands outside the React Aria modal subtree, where the underlay blocks
          pointer selection and scroll chains to the whole dialog. The dropdown
          carries the `tag-dropdown` marker so the containing FormSection can
          raise its stacking context (each .dune-lift is isolation:isolate, so a
          plain z-index can't paint over the next sibling panel). */}
      <div className="relative w-full">
        <SearchField value={query} onChange={setQuery} aria-label={ariaLabel} className="w-full">
          <SearchField.Group>
            <SearchField.SearchIcon />
            <SearchField.Input placeholder="Search gameplay tags…" />
            <SearchField.ClearButton />
          </SearchField.Group>
        </SearchField>
        {filtered.length > 0 && (
          <div className="tag-dropdown absolute z-[200] w-full mt-1 max-h-52 overflow-y-auto overscroll-contain rounded-[var(--radius)] border border-border bg-surface shadow-lg">
            {filtered.map((tag) => (
              <div
                key={tag}
                className="px-3 py-1.5 text-xs font-mono cursor-pointer hover:bg-surface-hover"
                onClick={() => handleSelect(tag)}
              >
                {tag}
              </div>
            ))}
          </div>
        )}
      </div>
    </React.Fragment>
  )
}
