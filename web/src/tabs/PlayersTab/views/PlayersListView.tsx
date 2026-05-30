import { useState, useMemo } from 'react'
import { Button, SearchField, Spinner } from '@heroui/react'
import type { Player } from '../../../api/client'
import { DataTable, Icon, PageHeader, type Column } from '../../../dune-ui'
import { PLAYER_COLUMNS, type PlayerSortKey } from '../types'
import { StatusDot } from '../components/StatusDot'

type ColKey = PlayerSortKey | 'actions'

const COLUMNS: Column<ColKey>[] = [
  ...PLAYER_COLUMNS,
  { key: 'actions', label: 'Actions', sortable: false },
]

interface Props {
  players: Player[]
  loading: boolean
  onRefresh: () => void
  onAction: (player: Player, action: 'inventory' | 'give' | 'actions') => void
}

export function PlayersListView({ players, loading, onRefresh, onAction }: Props) {
  const [search, setSearch] = useState('')

  const filtered = useMemo(() => {
    const q = search.toLowerCase()
    if (!q) return players
    return players.filter((p) =>
      p.name.toLowerCase().includes(q)
      || p.class.toLowerCase().includes(q)
      || p.map.toLowerCase().includes(q)
      || String(p.id).includes(q),
    )
  }, [players, search])

  return (
    <>
      <PageHeader title={`Players (${filtered.length}${search ? ` / ${players.length}` : ''})`}>
        <SearchField
          aria-label="Search players"
          className="w-72"
          value={search}
          onChange={setSearch}
        >
          <SearchField.Group>
            <SearchField.SearchIcon />
            <SearchField.Input placeholder="Search..." />
            <SearchField.ClearButton />
          </SearchField.Group>
        </SearchField>
        <Button size="sm" variant="ghost" onPress={onRefresh} isDisabled={loading}>
          {loading
            ? <Spinner size="sm" color="current" />
            : (
                <>
                  <Icon name="refresh-cw" />
                  {' '}
                  Refresh
                </>
              )}
        </Button>
      </PageHeader>

      {loading
        ? (
            <div className="flex justify-center py-12"><Spinner size="lg" /></div>
          )
        : (
            <DataTable<Player, ColKey>
              aria-label="Players"
              className="min-h-0 max-h-full"
              columns={COLUMNS}
              rows={filtered}
              rowId={(p) => String(p.id)}
              initialSort={{ column: 'id', direction: 'ascending' }}
              sortValue={(p, k) => k === 'actions' ? '' : (p as unknown as Record<string, string | number>)[k]}
              emptyState={<div className="py-8 text-center text-muted">{search ? 'No matches' : 'No players'}</div>}
              renderCell={(p, key) => {
                switch (key) {
                  case 'id':
                    return <span className="font-mono text-muted">{p.id}</span>
                  case 'name':
                    return (
                      <span className="inline-flex items-center font-semibold">
                        <StatusDot status={p.online_status} />
                        {p.name}
                      </span>
                    )
                  case 'class': return <span className="text-muted">{p.class}</span>
                  case 'map': return <span className="text-muted">{p.map}</span>
                  case 'faction_id': return <span className="text-muted">{p.faction_id || '—'}</span>
                  default:
                    return (
                      <div className="flex gap-1">
                        <Button size="sm" variant="ghost" onPress={() => onAction(p, 'inventory')}>
                          <Icon name="package" />
                          {' '}
                          Inventory
                        </Button>
                        <Button size="sm" variant="ghost" onPress={() => onAction(p, 'give')}>
                          <Icon name="gift" />
                          {' '}
                          Give
                        </Button>
                        <Button size="sm" variant="ghost" onPress={() => onAction(p, 'actions')}>
                          <Icon name="settings" />
                          {' '}
                          Actions
                        </Button>
                      </div>
                    )
                }
              }}
            />
          )}
    </>
  )
}
