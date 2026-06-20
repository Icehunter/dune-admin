import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Checkbox } from '@heroui/react'
import { Icon } from '../../../dune-ui'
import { CAT_COLOR } from '../constants'
import { TypeRow } from './TypeRow'
import type { CategorySectionProps } from './types'

export const CategorySection: React.FC<CategorySectionProps> = ({
  group, typesByCategory, expanded, setExpanded, search, filter, onToggle,
}): React.ReactElement | null => {
  const { t } = useTranslation()
  const items = typesByCategory[group.id]
  if (!items?.size) return null

  const isExpanded = expanded[group.id] ?? false
  const allOn = [...items.keys()].every((k) => filter[k] ?? false)
  const anyOn = [...items.keys()].some((k) => filter[k] ?? false)
  const q = search.toLowerCase()
  const filteredItems = q
    ? [...items.entries()].filter(([k, v]) => v.label.toLowerCase().includes(q) || k.toLowerCase().includes(q))
    : [...items.entries()]

  if (q && filteredItems.length === 0) return null

  const open = isExpanded || !!q
  const totalCount = [...items.values()].reduce((s, v) => s + v.count, 0)

  return (
    <div className="rounded-[var(--radius)] border border-border bg-surface">
      <div className="flex items-center gap-2 px-3 py-2">
        <Checkbox
          isSelected={allOn}
          isIndeterminate={!allOn && anyOn}
          onChange={(v) => { [...items.keys()].forEach((k) => onToggle(k, !v)) }}
        >
          <Checkbox.Control><Checkbox.Indicator /></Checkbox.Control>
        </Checkbox>
        <button
          type="button"
          className="flex-1 flex items-center gap-1.5 text-left min-w-0 focus:outline-none"
          onClick={() => { if (!q) setExpanded((e) => ({ ...e, [group.id]: !isExpanded })) }}
        >
          <span style={{ color: CAT_COLOR[group.id] }} className="text-xs shrink-0">●</span>
          <span className="text-xs font-medium text-muted uppercase tracking-wide">{t(group.labelKey as never)}</span>
          <span className="text-xs text-muted/60 ml-1">{totalCount.toLocaleString()}</span>
          {!q && (
            <Icon
              name={open ? 'chevron-up' : 'chevron-down'}
              className="ml-auto size-3 text-muted shrink-0"
            />
          )}
        </button>
      </div>
      {open && (
        <div className="border-t border-border px-1 py-1.5">
          {filteredItems.map(([key, { label, count }]) => (
            <TypeRow
              key={key}
              typeKey={key}
              label={label}
              count={count}
              category={group.id}
              filter={filter}
              onToggle={onToggle}
            />
          ))}
        </div>
      )}
    </div>
  )
}
