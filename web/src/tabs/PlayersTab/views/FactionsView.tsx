import { useState, useMemo } from 'react'
import { SearchField } from '@heroui/react'
import type { FactionRep } from '../../../api/client'
import { DataTable, PageHeader, type Column } from '../../../dune-ui'

type Key = 'player' | 'faction' | 'reputation' | 'scrips'

const COLUMNS: Column<Key>[] = [
  { key: 'player',     label: 'Player', isRowHeader: true },
  { key: 'faction',    label: 'Faction' },
  { key: 'reputation', label: 'Reputation' },
  { key: 'scrips',     label: 'Scrips' },
]

interface Props {
  data: FactionRep[]
  loading: boolean
  controllerToName: Map<number, string>
}

export function FactionsView({ data, loading, controllerToName }: Props) {
  const [search, setSearch] = useState('')

  const filtered = useMemo(() => {
    if (!search) return data
    const q = search.toLowerCase()
    return data.filter(r => {
      const name = controllerToName.get(r.actor_id) ?? ''
      return name.toLowerCase().includes(q) || String(r.actor_id).includes(q)
    })
  }, [data, search, controllerToName])

  return (
    <>
      <PageHeader title={`Factions (${filtered.length}${search ? ` / ${data.length}` : ''})`}>
        <SearchField
          aria-label="Search factions"
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

      <DataTable<FactionRep, Key>
        aria-label="Factions"
        className="min-h-0 max-h-full"
        columns={COLUMNS}
        rows={filtered}
        rowId={r => `${r.actor_id}-${r.faction_id}`}
        initialSort={{ column: 'player', direction: 'ascending' }}
        sortValue={(r, k) => {
          if (k === 'player')     return controllerToName.get(r.actor_id) ?? `#${r.actor_id}`
          if (k === 'faction')    return r.faction_name
          if (k === 'reputation') return r.reputation
          return r.scrips
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
                  {controllerToName.get(r.actor_id) && (
                    <span className="font-semibold">{controllerToName.get(r.actor_id)}</span>
                  )}
                  <span className="font-mono text-muted text-[10px]">#{r.actor_id}</span>
                </span>
              )
            case 'faction':    return <span className="font-semibold">{r.faction_name}</span>
            case 'reputation': return <span className="text-muted">{r.reputation.toLocaleString()}</span>
            case 'scrips':     return <span className="text-muted">{r.scrips.toLocaleString()}</span>
          }
        }}
      />
    </>
  )
}
