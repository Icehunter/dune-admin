import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { SearchField } from '@heroui/react'
import { useAtom } from 'jotai'
import { gameplayTagsSyncAtom } from '../../../../../data/store'
import { useDebounce } from '../hooks/useDebounce'
import type { AddTagsPanelProps } from './interfaces'

const normalizeTag = (value: string): string => value.trim().replace(/\s+/g, ' ')

export const AddTagsPanel: React.FC<AddTagsPanelProps> = React.memo(({ tags, pendingTags, onAdd }) => {
  const { t } = useTranslation()
  const [query, setQuery] = React.useState('')
  const debouncedQuery = useDebounce(query)
  const [allTags] = useAtom(gameplayTagsSyncAtom)

  const matches = React.useMemo(() => {
    if (!debouncedQuery) return []
    const tagsSet = new Set(tags)
    const pendingSet = new Set(pendingTags)
    const q = debouncedQuery.toLowerCase()
    return (allTags ?? [])
      .filter((t) => !tagsSet.has(t) && !pendingSet.has(t) && t.toLowerCase().includes(q))
      .slice(0, 100)
  }, [debouncedQuery, tags, pendingTags, allTags])

  const custom = normalizeTag(debouncedQuery)
  const customLower = custom.toLowerCase()
  const isKnownTag = (allTags ?? []).some((tg) => tg.toLowerCase() === customLower)
  const isAlreadyApplied = tags.some((tg) => tg.toLowerCase() === customLower)
    || pendingTags.some((tg) => tg.toLowerCase() === customLower)
  const showCustom = custom !== '' && !isKnownTag && !isAlreadyApplied
  const showDropdown = query.length > 0 && (matches.length > 0 || showCustom)

  const selectOption = (value: string): void => {
    onAdd(value)
    setQuery('')
  }

  const renderOption = (value: string, isCustom: boolean): React.ReactNode => (
    <div
      key={isCustom ? `__custom__${value}` : value}
      className="px-3 py-1.5 text-xs cursor-pointer hover:bg-surface-hover"
      onMouseDown={(e) => {
        e.preventDefault()
        selectOption(value)
      }}
    >
      {isCustom
        ? <span className="text-accent">{t('players.actions.tags.addCustom', { tag: value })}</span>
        : <span className="font-mono">{value}</span>}
    </div>
  )

  const renderDropdown = (): React.ReactNode => (
    <div className="absolute z-50 w-full mt-1 max-h-52 overflow-y-auto rounded-[var(--radius)] border border-border bg-surface">
      {matches.map((m) => renderOption(m, false))}
      {showCustom ? renderOption(custom, true) : null}
    </div>
  )

  return (
    <div className="relative">
      <SearchField value={query} onChange={setQuery} variant="secondary" aria-label={t('players.actions.tags.searchPlaceholder')}>
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input placeholder={t('players.actions.tags.searchPlaceholder')} />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>
      {showDropdown ? renderDropdown() : null}
    </div>
  )
})
