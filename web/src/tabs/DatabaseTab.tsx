import { useState } from 'react'
import { Button, InputGroup, Label, Spinner, TextField, toast } from '@heroui/react'
import { api } from '../api/client'
import { DataTable, Icon, PageHeader, SideNav, type Column } from '../dune-ui'

type Section = 'tables' | 'describe' | 'sample' | 'search' | 'sql'

type TableData = { headers: string[], rows: string[][] }

const SECTIONS: { key: Section, label: string }[] = [
  { key: 'tables', label: 'Tables' },
  { key: 'describe', label: 'Describe' },
  { key: 'sample', label: 'Sample' },
  { key: 'search', label: 'Search Columns' },
  { key: 'sql', label: 'Run SQL' },
]

function ResultTable({ headers, rows }: TableData) {
  // Backend may return null for empty headers/rows — guard against it.
  const safeHeaders = headers ?? []
  const safeRows = rows ?? []
  if (safeRows.length === 0 || safeHeaders.length === 0) {
    return <p className="text-sm text-muted">No results.</p>
  }
  const columns: Column<string>[] = safeHeaders.map((h, i) => ({
    key: `c${i}`,
    label: h,
  }))
  type Row = { _id: string, values: string[] }
  const items: Row[] = safeRows.map((r, i) => ({ _id: String(i), values: r ?? [] }))
  return (
    <DataTable<Row, string>
      aria-label="Result"
      className="min-h-0 max-h-full"
      columns={columns}
      rows={items}
      rowId={(r) => r._id}
      initialSort={{ column: columns[0].key, direction: 'ascending' }}
      sortValue={(r, k) => {
        const idx = Number(k.slice(1))
        const v = r.values[idx] ?? ''
        const n = Number(v)
        return !isNaN(n) && v !== '' ? n : v
      }}
      renderCell={(r, k) => {
        const idx = Number(k.slice(1))
        return <span className="font-mono whitespace-nowrap">{r.values[idx] ?? ''}</span>
      }}
    />
  )
}

export default function DatabaseTab() {
  const [active, setActive] = useState<Section>('tables')
  const [tableInput, setTableInput] = useState('')
  const [limitInput, setLimitInput] = useState('20')
  const [searchInput, setSearchInput] = useState('')
  const [sqlInput, setSqlInput] = useState('')
  const [result, setResult] = useState<TableData | null>(null)
  const [sqlResult, setSqlResult] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const run = async () => {
    setLoading(true)
    setResult(null)
    setSqlResult(null)
    setError(null)
    try {
      if (active === 'tables') {
        const rows = await api.database.tables()
        setResult({
          headers: ['Table', 'Rows'],
          rows: rows.map((r) => [r.name, String(r.row_count)]),
        })
      }
      else if (active === 'describe') {
        if (!tableInput.trim()) {
          toast.warning('Enter a table name')
          return
        }
        const r = await api.database.describe(tableInput.trim())
        setResult({
          headers: ['Column', 'Type', 'Nullable'],
          rows: r.columns.map((c) => [c.name, c.data_type, c.nullable]),
        })
      }
      else if (active === 'sample') {
        if (!tableInput.trim()) {
          toast.warning('Enter a table name')
          return
        }
        const r = await api.database.sample(tableInput.trim(), Number(limitInput) || 20)
        setResult({ headers: r.headers, rows: r.rows })
      }
      else if (active === 'search') {
        if (!searchInput.trim()) {
          toast.warning('Enter a search term')
          return
        }
        const r = await api.database.search(searchInput.trim())
        setResult({ headers: r.headers, rows: r.rows })
      }
      else {
        if (!sqlInput.trim()) {
          toast.warning('Enter a SQL query')
          return
        }
        const r = await api.database.sql(sqlInput.trim())
        setSqlResult(r.result)
      }
    }
    catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setError(msg)
      toast.danger(`Failed: ${msg}`)
    }
    finally {
      setLoading(false)
    }
  }

  const activeLabel = SECTIONS.find((s) => s.key === active)?.label ?? ''

  return (
    <div className="flex gap-4 h-full min-h-0">
      <SideNav<Section>
        items={SECTIONS}
        active={active}
        onSelect={(k) => {
          setActive(k)
          setResult(null)
          setSqlResult(null)
          setError(null)
        }}
        title="Database"
        width="w-60"
      />

      <div className="flex-1 flex flex-col gap-3 min-h-0 overflow-hidden">
        <PageHeader title={activeLabel} />

        {/* Inputs per section */}
        {(active === 'describe' || active === 'sample') && (
          <div className="flex items-center gap-3 shrink-0">
            <TextField className="flex-1 max-w-md" aria-label="Table name">
              <InputGroup className="w-full">
                <InputGroup.Prefix>Table dune.</InputGroup.Prefix>
                <InputGroup.Input
                  className="flex-1 w-full pl-2"
                  value={tableInput}
                  onChange={(e) => setTableInput(e.target.value)}
                  placeholder="actors"
                  onKeyDown={(e) => e.key === 'Enter' && run()}
                />
              </InputGroup>
            </TextField>
            {active === 'sample' && (
              <TextField className="w-28" aria-label="Limit">
                <InputGroup>
                  <InputGroup.Prefix>Limit</InputGroup.Prefix>
                  <InputGroup.Input
                    className="pl-2"
                    type="number"
                    min={1}
                    max={1000}
                    value={limitInput}
                    onChange={(e) => setLimitInput(e.target.value)}
                  />
                </InputGroup>
              </TextField>
            )}
            <Button onPress={run} isDisabled={loading} size="sm">
              {loading ? <Spinner size="sm" color="current" /> : <Icon name="play" />}
              {' '}
              Run
            </Button>
          </div>
        )}

        {active === 'search' && (
          <div className="flex items-center gap-3 shrink-0">
            <TextField className="flex-1 max-w-md" aria-label="Column or table name">
              <InputGroup className="w-full">
                <InputGroup.Prefix>Search</InputGroup.Prefix>
                <InputGroup.Input
                  className="flex-1 w-full pl-2"
                  value={searchInput}
                  onChange={(e) => setSearchInput(e.target.value)}
                  placeholder="player_id, faction..."
                  onKeyDown={(e) => e.key === 'Enter' && run()}
                />
              </InputGroup>
            </TextField>
            <Button onPress={run} isDisabled={loading} size="sm">
              {loading ? <Spinner size="sm" color="current" /> : <Icon name="search" />}
              {' '}
              Search
            </Button>
          </div>
        )}

        {active === 'tables' && (
          <div className="shrink-0">
            <Button onPress={run} isDisabled={loading} size="sm" variant="outline">
              {loading ? <Spinner size="sm" color="current" /> : <Icon name="list" />}
              {' '}
              List Tables
            </Button>
          </div>
        )}

        {active === 'sql' && (
          <div className="flex flex-col gap-2 shrink-0">
            <Label>SQL Query</Label>
            <textarea
              value={sqlInput}
              onChange={(e) => setSqlInput(e.target.value)}
              placeholder="SELECT * FROM dune.actors LIMIT 10;"
              rows={5}
              className="rounded-[var(--radius)] px-3 py-2 text-sm font-mono w-full resize-y outline-none border"
              style={{
                background: 'var(--field-background)',
                color: 'var(--field-foreground)',
                borderColor: 'var(--field-border)',
              }}
              onKeyDown={(e) => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) run() }}
            />
            <div className="flex items-center gap-3">
              <Button onPress={run} isDisabled={loading} size="sm">
                {loading ? <Spinner size="sm" color="current" /> : <Icon name="play" />}
                {' '}
                Run Query
              </Button>
              <span className="text-xs text-muted">Cmd/Ctrl+Enter to run</span>
            </div>
          </div>
        )}

        {loading && (
          <div className="flex justify-center py-8 shrink-0">
            <Spinner size="lg" />
          </div>
        )}

        {error && !loading && (
          <div className="rounded-[var(--radius)] p-4 bg-danger/10 border border-danger/40 text-danger shrink-0">
            <strong>Error:</strong>
            {' '}
            {error}
          </div>
        )}

        {result && !loading && !error && (
          <div className="flex-1 min-h-0 flex flex-col">
            <ResultTable headers={result.headers} rows={result.rows} />
          </div>
        )}

        {sqlResult !== null && !loading && !error && (
          <pre className="rounded-[var(--radius)] p-4 overflow-auto flex-1 min-h-0 text-sm font-mono whitespace-pre-wrap m-0 border border-border/60 bg-background text-foreground">
            {sqlResult || '(empty result)'}
          </pre>
        )}
      </div>
    </div>
  )
}
