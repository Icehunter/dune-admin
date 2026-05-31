import { useTranslation } from 'react-i18next'
import { DataTable } from '../../dune-ui'
import { phaseColor } from './helpers'
import { getServerColumns, type ServerRow, type ServerSortKey } from './types'

type Props = {
  servers: ServerRow[]
  isInitializing: boolean
  emptyMessage?: string
}

export function ServersTable({ servers, isInitializing, emptyMessage }: Props) {
  const { t } = useTranslation()
  return (
    <DataTable<ServerRow, ServerSortKey>
      aria-label={t('nav.battlegroup')}
      className="min-h-0 max-h-full"
      columns={getServerColumns(t)}
      rows={servers}
      rowId={(s) => `${s.map}-${s.dimension}-${s.partition}`}
      initialSort={{ column: 'map', direction: 'ascending' }}
      sortValue={(r, k) => k === 'ready' ? (r.ready ? 1 : 0) : (r[k] as string | number)}
      emptyState={emptyMessage && <div className="py-8 text-center text-muted">{emptyMessage}</div>}
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
              </span>
            )
          case 'ready':
            return (
              <span style={{ color: s.ready ? 'var(--success)' : 'var(--danger)' }}>
                {s.ready ? '✓' : '✗'}
              </span>
            )
          case 'dimension': return <span className="text-muted">{s.dimension}</span>
          case 'partition': return <span className="text-muted">{s.partition}</span>
        }
      }}
    />
  )
}
