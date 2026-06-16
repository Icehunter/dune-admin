import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@heroui-pro/react'
import { DataTable, Icon } from '../../dune-ui'
import { phaseColor } from './helpers'
import { formatUptime } from './uptime'
import { getServerColumns, type ServerRow, type ServerSortKey, type ServersTableProps } from './types'

export const ServersTable: React.FC<ServersTableProps> = ({ servers, isInitializing, loading, emptyMessage }) => {
  const { t } = useTranslation()
  return (
    <DataTable<ServerRow, ServerSortKey>
      aria-label={t('nav.battlegroup')}
      className="min-h-0 max-h-full"
      loading={loading}
      columns={getServerColumns(t)}
      rows={servers}
      rowId={(s) => `${s.map}-${s.dimension}-${s.partition}`}
      initialSort={{ column: 'map', direction: 'ascending' }}
      sortValue={(r, k) => {
        if (k === 'ready') return r.ready ? 1 : 0
        if (k === 'age') return r.ageSeconds ?? 0
        return r[k] as string | number
      }}
      emptyState={emptyMessage && (
        <EmptyState size="sm">
          <EmptyState.Header>
            <EmptyState.Title>{emptyMessage}</EmptyState.Title>
          </EmptyState.Header>
        </EmptyState>
      )}
      renderCell={(s, key) => {
        switch (key) {
          case 'map':
            return <span className="font-mono">{s.map}</span>
          case 'phase':
            return (
              <span className="font-semibold" style={{ color: phaseColor(s.phase) }}>
                {s.phase || '—'}
                {isInitializing && s.phase === 'Running' && (
                  <span className="ml-1 font-normal text-warning">{t('battlegroup.initializing')}</span>
                )}
              </span>
            )
          case 'players':
            return (
              <span className="font-semibold" style={{ color: s.players > 0 ? 'var(--success)' : 'var(--muted)' }}>
                {s.players}
                {s.playerHardCap > 0 && (
                  <span className="font-normal text-muted">{`/${s.playerHardCap}`}</span>
                )}
              </span>
            )
          case 'queue':
            return (
              <span style={{ color: s.queue > 0 ? 'var(--warning)' : 'var(--muted)' }}>
                {s.queue}
              </span>
            )
          case 'ready':
            return (
              <Icon
                name={s.ready ? 'check' : 'x'}
                className={`size-4 ${s.ready ? 'text-success' : 'text-danger'}`}
              />
            )
          case 'dimension': return <span className="text-muted">{s.dimension}</span>
          case 'partition': return <span className="text-muted">{s.partition}</span>
          case 'age': return <span className="font-mono text-muted">{formatUptime(s.ageSeconds)}</span>
        }
      }}
    />
  )
}
