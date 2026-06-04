import { useTranslation } from 'react-i18next'
import { Button } from '@heroui/react'
import { DataTable, Icon, LoadingState, SectionLabel } from '../../../../../dune-ui'
import { DebouncedSearchField } from '../components/DebouncedSearchField'
import { AddTagsPanel } from '../components/AddTagsPanel'
import { api } from '../../../../../api/client'
import type { Player } from '../../../../../api/client'

interface TagsSectionProps {
  player: Player
  tags: string[]
  tagsLoading: boolean
  pendingTags: string[]
  setPendingTags: (v: string[]) => void
  filteredActiveTags: string[]
  onFilterChange: (q: string) => void
  run: (fn: () => Promise<unknown>, label: string) => Promise<void>
  onAddTag: (tag: string) => void
  onTagsUpdate: (tags: string[]) => void
}

export function TagsSection({
  player,
  tags,
  tagsLoading,
  pendingTags,
  setPendingTags,
  filteredActiveTags,
  onFilterChange,
  run,
  onAddTag,
  onTagsUpdate,
}: TagsSectionProps) {
  const { t } = useTranslation()

  return (
    <div className="flex-1 min-h-0 flex flex-col gap-3 overflow-hidden">
      <div className="shrink-0 flex flex-col gap-2">
        <SectionLabel>{t('players.actions.tags.addTags')}</SectionLabel>
        <AddTagsPanel tags={tags} pendingTags={pendingTags} onAdd={onAddTag} />
        {pendingTags.length > 0 && (
          <>
            <div className="flex flex-col gap-1 mt-1">
              {pendingTags.map((tag) => (
                <div
                  key={tag}
                  className="flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius)] text-xs bg-surface border border-border"
                >
                  <span className="flex-1 font-mono">{tag}</span>
                  <Button
                    size="sm"
                    variant="danger-soft"
                    onPress={() => setPendingTags(pendingTags.filter((t) => t !== tag))}
                    aria-label={`Unstage ${tag}`}
                  >
                    <Icon name="x" className="size-3" />
                  </Button>
                </div>
              ))}
            </div>
            <Button
              size="sm"
              onPress={() => {
                const toAdd = pendingTags
                run(
                  () => api.players.updateTags(player.account_id, toAdd, []),
                  `Added ${toAdd.length} tag${toAdd.length > 1 ? 's' : ''}`,
                ).then(() => {
                  onTagsUpdate([...new Set([...tags, ...toAdd])].sort())
                  setPendingTags([])
                })
              }}
            >
              {t('players.actions.tags.addCount', { count: pendingTags.length })}
            </Button>
          </>
        )}
      </div>

      {tagsLoading
        ? <LoadingState size="md" />
        : (
            <div className="flex-1 min-h-0 flex flex-col gap-2 overflow-hidden">
              <div className="flex items-center gap-2 shrink-0 min-h-8">
                <SectionLabel>
                  {t('players.actions.tags.activeTags', { count: tags.length })}
                </SectionLabel>
                <DebouncedSearchField
                  className="flex-1"
                  placeholder={t('players.actions.tags.filterPlaceholder')}
                  onSearch={onFilterChange}
                />
              </div>
              <DataTable<string, 'tag' | 'actions'>
                aria-label={t('players.actions.tags.activeTagsLabel')}
                className="min-h-0 max-h-full"
                columns={[
                  { key: 'tag', label: t('players.actions.tags.tagColumn'), isRowHeader: true },
                  { key: 'actions', label: ' ', sortable: false, width: 60 },
                ]}
                rows={filteredActiveTags}
                rowId={(tag) => tag}
                initialSort={{ column: 'tag', direction: 'ascending' }}
                sortValue={(tag) => tag}
                emptyState={<div className="py-8 text-center text-muted">{t('players.actions.tags.noTags')}</div>}
                renderCell={(tag, key) => {
                  if (key === 'tag') return <span className="font-mono">{tag}</span>
                  return (
                    <Button
                      size="sm"
                      variant="danger-soft"
                      onPress={() => {
                        onTagsUpdate(tags.filter((s) => s !== tag))
                        run(
                          () => api.players.updateTags(player.account_id, [], [tag]),
                          t('players.actions.tags.removedTag'),
                        )
                      }}
                      aria-label={`Remove ${tag}`}
                    >
                      <Icon name="x" className="size-3" />
                    </Button>
                  )
                }}
              />
            </div>
          )}
    </div>
  )
}
