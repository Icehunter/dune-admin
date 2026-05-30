import { useState, useMemo } from 'react'
import { SearchField } from '@heroui/react'
import type { CurrencyRow } from '../../../api/client'
import { DataTable, PageHeader, type Column } from '../../../dune-ui'

type Key = 'player' | 'currency' | 'balance'

const COLUMNS: Column<Key>[] = [
  { key: 'player', label: 'Player', isRowHeader: true },
  { key: 'currency', label: 'Currency' },
  { key: 'balance', label: 'Balance' },
]

interface Props {
  data: CurrencyRow[]
  loading: boolean
  controllerToName: Map<number, string>
}

export function CurrencyView({ data, loading, controllerToName }: Props) {
  const [search, setSearch] = useState('')

  const filtered = useMemo(() => {
    if (!search) return data
    const q = search.toLowerCase()
    return data.filter((r) => {
      const name = controllerToName.get(r.player_id) ?? ''
      return name.toLowerCase().includes(q) || String(r.player_id).includes(q)
    })
  }, [data, search, controllerToName])

  return (
    <>
      <PageHeader title={`Currency (${filtered.length}${search ? ` / ${data.length}` : ''})`}>
        <SearchField
          aria-label="Search currency"
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

      <DataTable<CurrencyRow, Key>
        aria-label="Currency"
        className="min-h-0 max-h-full"
        columns={COLUMNS}
        rows={filtered}
        rowId={(r) => `${r.player_id}-${r.currency_id}`}
        initialSort={{ column: 'player', direction: 'ascending' }}
        sortValue={(r, k) => {
          if (k === 'player') return controllerToName.get(r.player_id) ?? `#${r.player_id}`
          if (k === 'currency') return r.currency_id
          return r.balance
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
                  <span className="font-mono text-muted text-[10px]">
                    #
                    {r.player_id}
                  </span>
                </span>
              )
            case 'currency': return <span className="font-mono text-muted">{r.currency_id}</span>
            case 'balance': return <span className="font-semibold">{r.balance.toLocaleString()}</span>
          }
        }}
      />
    </>
  )
}
