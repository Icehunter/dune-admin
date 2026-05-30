import { useState, useMemo } from 'react'
import { SearchField } from '@heroui/react'
import type { OnlineRow } from '../../../api/client'
import { DataTable, PageHeader, type Column } from '../../../dune-ui'
import { OnlineBadge } from '../components/OnlineBadge'

type Key = 'player' | 'status' | 'last_seen' | 'map'

const COLUMNS: Column<Key>[] = [
  { key: 'player', label: 'Player', isRowHeader: true },
  { key: 'status', label: 'Status', sortable: false },
  { key: 'last_seen', label: 'Last Seen' },
  { key: 'map', label: 'Map' },
]

interface Props {
  data: OnlineRow[]
  loading: boolean
}

export function OnlineView({ data, loading }: Props) {
  const [search, setSearch] = useState('')

  const filtered = useMemo(() => {
    if (!search) return data
    const q = search.toLowerCase()
    return data.filter((r) =>
      r.name.toLowerCase().includes(q) || String(r.player_id).includes(q),
    )
  }, [data, search])

  return (
    <>
      <PageHeader title={`Online State (${filtered.length}${search ? ` / ${data.length}` : ''})`}>
        <SearchField
          aria-label="Search online"
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
      </PageHeader>

      <DataTable<OnlineRow, Key>
        aria-label="Online state"
        className="min-h-0 max-h-full"
        columns={COLUMNS}
        rows={filtered}
        rowId={(r) => String(r.player_id)}
        initialSort={{ column: 'player', direction: 'ascending' }}
        sortValue={(r, k) => {
          if (k === 'player') return r.name
          if (k === 'status') return r.status
          return (r as unknown as Record<string, string>)[k]
        }}
        emptyState={
          loading
            ? <div className="py-8 text-center text-muted">Loading…</div>
            : <div className="py-8 text-center text-muted">{search ? 'No matches' : 'No data'}</div>
        }
        renderCell={(r, key) => {
          switch (key) {
            case 'player':
              return (
                <span className="inline-flex flex-col">
                  <span className="font-semibold">{r.name}</span>
                  <span className="font-mono text-muted text-[10px]">
                    #
                    {r.player_id}
                  </span>
                </span>
              )
            case 'status': return <OnlineBadge status={r.status} />
            case 'last_seen': return <span className="font-mono text-muted">{r.last_seen}</span>
            case 'map': return <span className="text-muted">{r.map}</span>
          }
        }}
      />
    </>
  )
}
