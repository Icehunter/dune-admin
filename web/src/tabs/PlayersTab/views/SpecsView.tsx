import { useState, useMemo } from 'react'
import { SearchField } from '@heroui/react'
import type { SpecTrack } from '../../../api/client'
import { DataTable, PageHeader, type Column } from '../../../dune-ui'

type Key = 'player' | 'track' | 'xp' | 'level'

const COLUMNS: Column<Key>[] = [
  { key: 'player', label: 'Player', isRowHeader: true },
  { key: 'track',  label: 'Track' },
  { key: 'xp',     label: 'XP' },
  { key: 'level',  label: 'Level' },
]

interface Props {
  data: SpecTrack[]
  loading: boolean
  controllerToName: Map<number, string>
}

export function SpecsView({ data, loading, controllerToName }: Props) {
  const [search, setSearch] = useState('')

  const filtered = useMemo(() => {
    if (!search) return data
    const q = search.toLowerCase()
    return data.filter(r => {
      const name = controllerToName.get(r.player_id) ?? ''
      return name.toLowerCase().includes(q) || String(r.player_id).includes(q)
    })
  }, [data, search, controllerToName])

  return (
    <>
      <PageHeader title={`Specs / XP (${filtered.length}${search ? ` / ${data.length}` : ''})`}>
        <SearchField
          aria-label="Search specs"
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

      <DataTable<SpecTrack, Key>
        aria-label="Specs / XP"
        className="min-h-0 max-h-full"
        columns={COLUMNS}
        rows={filtered}
        rowId={r => `${r.player_id}-${r.track_type}`}
        initialSort={{ column: 'player', direction: 'ascending' }}
        sortValue={(r, k) => {
          if (k === 'player') return controllerToName.get(r.player_id) ?? `#${r.player_id}`
          if (k === 'track')  return r.track_type
          if (k === 'xp')     return r.xp
          return r.level
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
                  {controllerToName.get(r.player_id) && (
                    <span className="font-semibold">{controllerToName.get(r.player_id)}</span>
                  )}
                  <span className="font-mono text-muted text-[10px]">#{r.player_id}</span>
                </span>
              )
            case 'track': return <span className="font-semibold">{r.track_type}</span>
            case 'xp':    return <span className="text-muted">{r.xp.toLocaleString()}</span>
            case 'level': return <span className="text-muted">{r.level}</span>
          }
        }}
      />
    </>
  )
}
