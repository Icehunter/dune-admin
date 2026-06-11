import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useAtom } from 'jotai'
import { Button } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { DataTable, Icon, LoadingState, SectionLabel } from '../../../../../dune-ui'
import { DebouncedSearchField } from '../components/DebouncedSearchField'
import { AddTagsPanel } from '../components/AddTagsPanel'
import { api } from '../../../../../api/client'
import { busyAtom } from '../store'
import { useRun } from '../hooks/useActions'
import type { TagsSectionProps } from './types'

export const TagsSection: React.FC<TagsSectionProps> = ({ player }) => {
  const { t } = useTranslation()
  const [busy] = useAtom(busyAtom(player.id))
  const run = useRun(player.id)

  const [tags, setTags] = React.useState<string[]>([])
  const [tagsLoaded, setTagsLoaded] = React.useState(false)
  const [tagsLoading, setTagsLoading] = React.useState(false)
  const [pendingTags, setPendingTags] = React.useState<string[]>([])
  const [filterQuery, setFilterQuery] = React.useState('')

  React.useEffect(() => {
    Promise.resolve().then(() => {
      setTagsLoaded(false)
      setTags([])
      setPendingTags([])
    })
  }, [player.id])

  React.useEffect(() => {
    if (tagsLoaded) return
    Promise.resolve()
      .then(() => setTagsLoading(true))
      .then(() => api.players.tags(player.account_id))
      .then((t) => {
        setTags(t)
        setTagsLoaded(true)
      })
      .catch(() => {})
      .finally(() => setTagsLoading(false))
  }, [tagsLoaded, player.account_id])

  const filteredActiveTags = filterQuery
    ? tags.filter((t) => t.toLowerCase().includes(filterQuery.toLowerCase()))
    : tags

  const handleAddTags = () => {
    const toAdd = pendingTags
    run(
      () => api.players.updateTags(player.account_id, toAdd, []),
      `Added ${toAdd.length} tag${toAdd.length > 1 ? 's' : ''}`,
    ).then(() => {
      setTags((prev) => [...new Set([...prev, ...toAdd])].sort())
      setPendingTags([])
    })
  }

  const handleRemoveTag = (tag: string) => {
    setTags((prev) => prev.filter((s) => s !== tag))
    run(
      () => api.players.updateTags(player.account_id, [], [tag]),
      t('players.actions.tags.removedTag'),
    )
  }

  return (
    <div className="flex-1 min-h-0 flex flex-col gap-3 overflow-hidden">
      <div className="shrink-0 flex flex-col gap-2">
        <SectionLabel>{t('players.actions.tags.addTags')}</SectionLabel>
        <AddTagsPanel
          tags={tags}
          pendingTags={pendingTags}
          onAdd={(tag) => setPendingTags((prev) => [...prev, tag])}
        />
        {pendingTags.length > 0 && (
          <>
            <div className="flex flex-col gap-1 mt-1">
              {pendingTags.map((tag) => (
                <div
                  key={tag}
                  className="flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius)]
                    text-xs bg-surface border border-border"
                >
                  <span className="flex-1 font-mono">{tag}</span>
                  <Button
                    size="sm"
                    variant="danger-soft"
                    onPress={() => setPendingTags((prev) => prev.filter((t) => t !== tag))}
                    aria-label={`Unstage ${tag}`}
                  >
                    <Icon name="x" className="size-3" />
                  </Button>
                </div>
              ))}
            </div>
            <Button size="sm" isDisabled={busy} onPress={handleAddTags}>
              {t('players.actions.tags.addCount', { count: pendingTags.length })}
            </Button>
          </>
        )}
      </div>

      {tagsLoading
        ? (
            <LoadingState size="md" />
          )
        : (
            <div className="flex-1 min-h-0 flex flex-col gap-2 overflow-hidden">
              <div className="flex items-center gap-2 shrink-0 min-h-8">
                <SectionLabel>
                  {t('players.actions.tags.activeTags', { count: tags.length })}
                </SectionLabel>
                <DebouncedSearchField
                  className="flex-1"
                  placeholder={t('players.actions.tags.filterPlaceholder')}
                  onSearch={setFilterQuery}
                />
              </div>
              <DataTable<{ id: string }, 'tag' | 'actions'>
                aria-label={t('players.actions.tags.activeTagsLabel')}
                className="min-h-0 max-h-full"
                columns={[
                  {
                    key: 'tag',
                    label: t('players.actions.tags.tagColumn'),
                    isRowHeader: true,
                  },
                  { key: 'actions', label: ' ', sortable: false, width: 60 },
                ]}
                rows={filteredActiveTags.map((tag) => ({ id: tag }))}
                rowId={(r) => r.id}
                initialSort={{ column: 'tag', direction: 'ascending' }}
                sortValue={(r) => r.id}
                emptyState={(
                  <EmptyState size="sm">
                    <EmptyState.Header>
                      <EmptyState.Media variant="icon">
                        <IconifyIcon icon="gravity-ui:magnifier" className="size-5" />
                      </EmptyState.Media>
                      <EmptyState.Title>{t('players.actions.tags.noTags')}</EmptyState.Title>
                    </EmptyState.Header>
                  </EmptyState>
                )}
                renderCell={(r, key) => {
                  if (key === 'tag') return <span className="font-mono">{r.id}</span>
                  return (
                    <Button
                      size="sm"
                      variant="danger-soft"
                      aria-label={`Remove ${r.id}`}
                      onPress={() => handleRemoveTag(r.id)}
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
