import { memo, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SearchField } from '@heroui/react'
import allGameplayTags from '../../../../../data/gameplayTags.json'
import { useDebounce } from '../hooks/useDebounce'

interface AddTagsPanelProps {
  tags: string[]
  pendingTags: string[]
  onAdd: (tag: string) => void
}

export const AddTagsPanel = memo(function AddTagsPanel({
  tags,
  pendingTags,
  onAdd,
}: AddTagsPanelProps) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const debouncedQuery = useDebounce(query)

  const matches = useMemo(() => {
    if (!debouncedQuery) return []
    const tagsSet = new Set(tags)
    const pendingSet = new Set(pendingTags)
    const q = debouncedQuery.toLowerCase()
    return (allGameplayTags as string[])
      .filter((t) => !tagsSet.has(t) && !pendingSet.has(t) && t.toLowerCase().includes(q))
      .slice(0, 100)
  }, [debouncedQuery, tags, pendingTags])

  return (
    <div className="relative">
      <SearchField value={query} onChange={setQuery} variant="secondary">
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input placeholder={t('players.actions.tags.searchPlaceholder')} />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>
      {query && matches.length > 0 && (
        <div className="absolute z-50 w-full mt-1 max-h-52 overflow-y-auto rounded-[var(--radius)] border border-border bg-surface">
          {matches.map((t) => (
            <div
              key={t}
              className="px-3 py-1.5 text-xs font-mono cursor-pointer hover:bg-surface-hover"
              onMouseDown={(e) => {
                e.preventDefault()
                onAdd(t)
                setQuery('')
              }}
            >
              {t}
            </div>
          ))}
        </div>
      )}
    </div>
  )
})
