import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Chip } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { DataTable, LoadingState, Panel, SectionLabel } from '../../../../../dune-ui'
import { api } from '../../../../../api/client'
import type { GameEvent, DungeonRecord } from '../../../../../api/client'
import type { HistorySectionProps, ChipColor } from './types'

const eventColor = (t: number): ChipColor => {
  if (t === 1) return 'success'
  if (t === 2) return 'danger'
  if (t === 3) return 'warning'
  return 'default'
}

const difficultyColor = (d: string): ChipColor => {
  if (d === 'Hard') return 'danger'
  if (d === 'Normal') return 'warning'
  return 'default'
}

export const HistorySection: React.FC<HistorySectionProps> = ({ player }) => {
  const { t } = useTranslation()

  const [events, setEvents] = React.useState<GameEvent[]>([])
  const [dungeons, setDungeons] = React.useState<DungeonRecord[]>([])
  const [loaded, setLoaded] = React.useState(false)
  const [loading, setLoading] = React.useState(false)

  React.useEffect(() => {
    Promise.resolve().then(() => {
      setLoaded(false)
      setEvents([])
      setDungeons([])
    })
  }, [player.id])

  React.useEffect(() => {
    if (loaded) return
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => Promise.all([api.players.events(player.id), api.players.dungeons(player.id)]))
      .then(([evts, dngns]) => {
        setEvents(evts)
        setDungeons(dngns)
        setLoaded(true)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [loaded, player.id])

  if (loading) return <LoadingState size="md" />

  const formatDuration = (ms: number) => {
    const secs = Math.floor(ms / 1000)
    return `${Math.floor(secs / 60)}:${String(secs % 60).padStart(2, '0')}`
  }

  const renderGameEventCell = (evt: GameEvent, key: string) => {
    switch (key) {
      case 'time':
        return <span className="font-mono text-muted">{evt.universe_time}</span>
      case 'map':
        return <span className="text-muted">{evt.map}</span>
      case 'event_type':
        return (
          <Chip size="sm" color={eventColor(evt.event_type)} variant="soft">
            {evt.event_type}
          </Chip>
        )
      case 'location':
        return (
          <span className="font-mono text-muted">
            {Math.round(evt.x)}
            ,
            {' '}
            {Math.round(evt.y)}
            ,
            {' '}
            {Math.round(evt.z)}
          </span>
        )
    }
  }

  const renderDungeonCell = (d: DungeonRecord, key: string) => {
    switch (key) {
      case 'dungeon':
        return <span className="font-mono">{d.dungeon_id}</span>
      case 'difficulty':
        return (
          <Chip size="sm" color={difficultyColor(d.difficulty)} variant="soft">
            {d.difficulty}
          </Chip>
        )
      case 'duration':
        return <span className="font-mono text-muted">{formatDuration(d.duration_ms)}</span>
      case 'party':
        return <span className="text-muted">{d.players_num}</span>
    }
  }

  return (
    <div className="flex-1 overflow-y-auto flex flex-col gap-3 pr-2">
      <Panel className="flex-1">
        <SectionLabel>{t('players.actions.history.gameEvents')}</SectionLabel>
        <DataTable<GameEvent, 'time' | 'map' | 'event_type' | 'location'>
          aria-label={t('players.actions.history.gameEventsLabel')}
          className="max-h-[40vh]"
          columns={[
            { key: 'time', label: t('players.actions.history.columns.time'), isRowHeader: true },
            { key: 'map', label: t('players.actions.history.columns.map') },
            { key: 'event_type', label: t('players.actions.history.columns.eventType') },
            { key: 'location', label: t('players.actions.history.columns.location'), sortable: false },
          ]}
          rows={events}
          rowId={(evt) => `${evt.actor_id}-${evt.universe_time}`}
          initialSort={{ column: 'time', direction: 'descending' }}
          sortValue={(evt, k) => {
            if (k === 'time') return evt.universe_time
            if (k === 'map') return evt.map
            if (k === 'event_type') return evt.event_type
            return ''
          }}
          emptyState={(
            <EmptyState size="sm">
              <EmptyState.Header>
                <EmptyState.Media variant="icon">
                  <IconifyIcon icon="gravity-ui:clock" className="size-5" />
                </EmptyState.Media>
                <EmptyState.Title>{t('players.actions.history.noEvents')}</EmptyState.Title>
              </EmptyState.Header>
            </EmptyState>
          )}
          renderCell={renderGameEventCell}
        />
      </Panel>
      <Panel className="flex-1">
        <SectionLabel>{t('players.actions.history.dungeonRecords')}</SectionLabel>
        <DataTable<DungeonRecord, 'dungeon' | 'difficulty' | 'duration' | 'party'>
          aria-label={t('players.actions.history.dungeonLabel')}
          className="max-h-[40vh]"
          columns={[
            { key: 'dungeon', label: t('players.actions.history.columns.dungeon'), isRowHeader: true },
            { key: 'difficulty', label: t('players.actions.history.columns.difficulty') },
            { key: 'duration', label: t('players.actions.history.columns.duration') },
            { key: 'party', label: t('players.actions.history.columns.partySize') },
          ]}
          rows={dungeons}
          rowId={(d) => `${d.dungeon_id}-${d.completion_id}`}
          initialSort={{ column: 'dungeon', direction: 'ascending' }}
          sortValue={(d, k) => {
            if (k === 'dungeon') return d.dungeon_id
            if (k === 'difficulty') return d.difficulty
            if (k === 'duration') return d.duration_ms
            return d.players_num
          }}
          emptyState={(
            <EmptyState size="sm">
              <EmptyState.Header>
                <EmptyState.Title>{t('players.actions.history.noDungeons')}</EmptyState.Title>
              </EmptyState.Header>
            </EmptyState>
          )}
          renderCell={renderDungeonCell}
        />
      </Panel>
    </div>
  )
}
