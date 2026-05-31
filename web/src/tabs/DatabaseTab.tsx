import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import CodeMirror from '@uiw/react-codemirror'
import { createTheme } from '@uiw/codemirror-themes'
import { sql as sqlLang, PostgreSQL } from '@codemirror/lang-sql'
import { keymap } from '@codemirror/view'
import { Prec } from '@codemirror/state'
import { acceptCompletion } from '@codemirror/autocomplete'
import { tags as hlTags } from '@lezer/highlight'
import { Button, SearchField, Spinner, toast } from '@heroui/react'
import { api } from '../api/client'
import { DataTable, Icon, NumberInput, PageHeader, type Column } from '../dune-ui'

// ── CodeMirror theme ────────────────────────────────────────────────────────

const duneTheme = createTheme({
  theme: 'dark',
  settings: {
    background: 'var(--field-background)',
    foreground: 'var(--field-foreground)',
    caret: 'var(--accent)',
    selection: 'rgba(201,130,10,0.25)',
    selectionMatch: 'rgba(201,130,10,0.12)',
    lineHighlight: 'var(--surface)',
    gutterBackground: 'var(--surface)',
    gutterForeground: 'var(--muted)',
    gutterBorder: 'transparent',
    gutterActiveForeground: 'var(--accent)',
  },
  styles: [
    { tag: hlTags.comment, color: 'var(--muted)', fontStyle: 'italic' },
    { tag: hlTags.lineComment, color: 'var(--muted)', fontStyle: 'italic' },
    { tag: hlTags.blockComment, color: 'var(--muted)', fontStyle: 'italic' },
    { tag: hlTags.keyword, color: 'var(--accent)', fontWeight: 'bold' },
    { tag: hlTags.definitionKeyword, color: 'var(--accent)' },
    { tag: hlTags.modifier, color: 'var(--accent)' },
    { tag: hlTags.operatorKeyword, color: 'var(--accent)' },
    { tag: hlTags.string, color: 'var(--success)' },
    { tag: hlTags.number, color: 'var(--warning)' },
    { tag: hlTags.bool, color: 'var(--warning)' },
    { tag: hlTags.null, color: 'var(--danger)' },
    { tag: hlTags.operator, color: 'var(--foreground)' },
    { tag: hlTags.punctuation, color: 'var(--muted)' },
    { tag: hlTags.name, color: 'var(--foreground)' },
    { tag: hlTags.typeName, color: 'var(--warning)' },
    { tag: hlTags.function(hlTags.variableName), color: 'var(--warning)' },
    { tag: hlTags.special(hlTags.name), color: 'var(--accent)' },
  ],
})

// ── Types ────────────────────────────────────────────────────────────────────

type Section = 'tables' | 'describe' | 'sample' | 'search' | 'sql'
type TableData = { headers: string[], rows: string[][] }

// ── Sub-components ───────────────────────────────────────────────────────────

function ResultTable({ headers, rows }: TableData) {
  const { t } = useTranslation()
  const safeHeaders = headers ?? []
  const safeRows = rows ?? []
  if (safeRows.length === 0 || safeHeaders.length === 0) {
    return <p className="text-sm text-muted">{t('database.noResults')}</p>
  }
  const columns: Column<string>[] = safeHeaders.map((h, i) => ({
    key: `c${i}`,
    label: h,
  }))
  type Row = { _id: string, values: string[] }
  const items: Row[] = safeRows.map((r, i) => ({ _id: String(i), values: r ?? [] }))
  return (
    <DataTable<Row, string>
      aria-label={t('database.resultLabel')}
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

/** Type-ahead table picker — matches the GiveItemsModal search-dropdown pattern. */
function TableSearchInput({ value, onChange, onRun, tableNames, ariaLabel, placeholder }: {
  value: string
  onChange: (v: string) => void
  onRun: () => void
  tableNames: string[]
  ariaLabel: string
  placeholder: string
}) {
  const [open, setOpen] = useState(false)

  const filtered = useMemo(() => {
    const q = value.toLowerCase().trim()
    if (!q) return tableNames.slice(0, 40)
    return tableNames.filter((n) => n.toLowerCase().includes(q))
  }, [value, tableNames])

  const pick = (name: string) => {
    onChange(name)
    setOpen(false)
  }

  return (
    <div
      className="relative flex-1 max-w-md"
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
          setOpen(false)
        }
      }}
    >
      <SearchField
        className="w-full"
        value={value}
        onChange={(v) => {
          onChange(v)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        aria-label={ariaLabel}
      >
        <SearchField.Group>
          <SearchField.SearchIcon />
          <SearchField.Input
            placeholder={placeholder}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                setOpen(false)
                onRun()
              }
              if (e.key === 'Escape') setOpen(false)
              if (e.key === 'ArrowDown') setOpen(true)
            }}
          />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>
      {open && filtered.length > 0 && (
        <div className="absolute z-50 w-full mt-1 rounded-[var(--radius)] border border-border bg-surface overflow-y-auto max-h-52 shadow-lg">
          {filtered.map((n) => (
            <button
              key={n}
              type="button"
              className="w-full text-left px-3 py-1.5 text-xs cursor-pointer hover:bg-surface-hover"
              onMouseDown={(e) => {
                e.preventDefault()
                pick(n)
              }}
            >
              <span className="text-muted mr-0.5">dune.</span>
              <span className="font-mono">{n}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Main component ───────────────────────────────────────────────────────────

export default function DatabaseTab({ section = 'tables' }: { section?: Section }) {
  const { t } = useTranslation()

  const SECTIONS: { key: Section, label: string }[] = [
    { key: 'tables', label: t('database.sections.tables') },
    { key: 'describe', label: t('database.sections.describe') },
    { key: 'sample', label: t('database.sections.sample') },
    { key: 'search', label: t('database.sections.search') },
    { key: 'sql', label: t('database.sections.sql') },
  ]

  const [tableInput, setTableInput] = useState('')
  const [limitInput, setLimitInput] = useState(20)
  const [searchInput, setSearchInput] = useState('')
  const [sqlInput, setSqlInput] = useState('')
  const [result, setResult] = useState<TableData | null>(null)
  const [truncated, setTruncated] = useState(false)
  const [tableNames, setTableNames] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const sqlExtension = useMemo(() => sqlLang({
    dialect: PostgreSQL,
    upperCaseKeywords: true,
    schema: Object.fromEntries(tableNames.map((n) => [n, []])),
    defaultSchema: 'dune',
  }), [tableNames])

  // Promise-chain form (not async) so react-hooks/set-state-in-effect does not
  // flag the useEffect that calls it — matches the BasesTab pattern.
  const fetchTables = useCallback(() => {
    Promise.resolve()
      .then(() => {
        setLoading(true)
        setResult(null)
        setTruncated(false)
        setError(null)
      })
      .then(() => api.database.tables())
      .then((rows) => {
        setTableNames(rows.map((r) => r.name))
        setResult({
          headers: [t('database.tableColumn'), t('database.rowsColumn')],
          rows: rows.map((r) => [r.name, String(r.row_count)]),
        })
      })
      .catch((e: unknown) => {
        const msg = e instanceof Error ? e.message : String(e)
        setError(msg)
        toast.danger(t('database.failed', { message: msg }))
      })
      .finally(() => setLoading(false))
  }, [t])

  // Reset results and re-fetch whenever the section changes (driven by the left nav).
  useEffect(() => {
    setTruncated(false) // eslint-disable-line react-hooks/set-state-in-effect
    setError(null)
    setResult(null)
    if (section === 'tables') fetchTables()
  }, [section, fetchTables])

  const run = useCallback(async () => {
    if (section === 'tables') {
      fetchTables()
      return
    }
    setLoading(true)
    setResult(null)
    setTruncated(false)
    setError(null)
    try {
      if (section === 'describe') {
        if (!tableInput.trim()) {
          toast.warning(t('database.enterTableName'))
          return
        }
        const r = await api.database.describe(tableInput.trim())
        setResult({
          headers: [t('database.columnColumn'), t('database.typeColumn'), t('database.nullableColumn')],
          rows: r.columns.map((c) => [c.name, c.data_type, c.nullable]),
        })
      }
      else if (section === 'sample') {
        if (!tableInput.trim()) {
          toast.warning(t('database.enterTableName'))
          return
        }
        const r = await api.database.sample(tableInput.trim(), limitInput)
        setResult({ headers: r.headers, rows: r.rows })
      }
      else if (section === 'search') {
        if (!searchInput.trim()) {
          toast.warning(t('database.enterSearchTerm'))
          return
        }
        const r = await api.database.search(searchInput.trim())
        setResult({ headers: r.headers, rows: r.rows })
      }
      else {
        if (!sqlInput.trim()) {
          toast.warning(t('database.enterSQL'))
          return
        }
        const r = await api.database.sql(sqlInput.trim())
        setResult({ headers: r.headers, rows: r.rows })
        setTruncated(r.truncated)
      }
    }
    catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setError(msg)
      toast.danger(t('database.failed', { message: msg }))
    }
    finally {
      setLoading(false)
    }
  }, [section, fetchTables, limitInput, searchInput, sqlInput, tableInput, t])

  const editorKeymap = useMemo(() => [
    Prec.highest(keymap.of([
      {
        key: 'Mod-Enter',
        run: () => {
          void run()
          return true
        },
      },
      // Must be Prec.highest to beat basicSetup's indent binding
      { key: 'Tab', run: acceptCompletion },
    ])),
  ], [run])

  const activeLabel = SECTIONS.find((s) => s.key === section)?.label ?? ''

  return (
    <div className="h-full min-h-0 flex flex-col gap-3">
      <PageHeader title={activeLabel}>
        <Button
          size="sm"
          variant="ghost"
          isIconOnly
          onPress={() => void run()}
          isDisabled={loading}
          aria-label={t('database.refreshLabel')}
        >
          {loading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
        </Button>
      </PageHeader>

      {(section === 'describe' || section === 'sample') && (
        <div className="flex items-center gap-3 shrink-0">
          <TableSearchInput
            value={tableInput}
            onChange={setTableInput}
            onRun={() => void run()}
            tableNames={tableNames}
            ariaLabel={t('database.tableNameLabel')}
            placeholder={t('database.tablePlaceholder')}
          />
          {section === 'sample' && (
            <NumberInput
              ariaLabel={t('database.limitLabel')}
              min={1}
              max={1000}
              value={limitInput}
              onChange={setLimitInput}
              showButtons={false}
              className="w-28"
            />
          )}
          <Button onPress={() => void run()} isDisabled={loading} size="sm">
            {loading ? <Spinner size="sm" color="current" /> : <Icon name="play" />}
            {' '}
            {t('database.runBtn')}
          </Button>
        </div>
      )}

      {section === 'search' && (
        <div className="flex items-center gap-3 shrink-0">
          <SearchField
            className="flex-1 max-w-md"
            value={searchInput}
            onChange={setSearchInput}
            aria-label={t('database.searchLabel')}
          >
            <SearchField.Group>
              <SearchField.SearchIcon />
              <SearchField.Input
                placeholder={t('database.searchPlaceholder')}
                onKeyDown={(e) => e.key === 'Enter' && void run()}
              />
              <SearchField.ClearButton />
            </SearchField.Group>
          </SearchField>
          <Button onPress={() => void run()} isDisabled={loading} size="sm">
            {loading ? <Spinner size="sm" color="current" /> : <Icon name="search" />}
            {' '}
            Search
          </Button>
        </div>
      )}

      {section === 'sql' && (
        <div className="flex flex-col gap-2 shrink-0">
          <div
            className="rounded-[var(--radius)] overflow-hidden border"
            style={{ borderColor: 'var(--field-border)' }}
          >
            <CodeMirror
              value={sqlInput}
              onChange={setSqlInput}
              extensions={editorKeymap.concat(sqlExtension)}
              theme={duneTheme}
              height="140px"
              basicSetup={{
                lineNumbers: true,
                foldGutter: false,
                autocompletion: true,
                highlightActiveLine: true,
                highlightSelectionMatches: true,
              }}
              placeholder={t('database.sqlPlaceholder')}
            />
          </div>
          <div className="flex items-center gap-3">
            <Button onPress={() => void run()} isDisabled={loading} size="sm">
              {loading ? <Spinner size="sm" color="current" /> : <Icon name="play" />}
              {' '}
              {t('database.runQuery')}
            </Button>
            <span className="text-xs text-muted">{t('database.runHint')}</span>
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
          <strong>{t('common.error')}</strong>
          {' '}
          {error}
        </div>
      )}

      {result && !loading && !error && (
        <div className="flex-1 min-h-0 flex flex-col gap-1">
          <ResultTable headers={result.headers} rows={result.rows} />
          {truncated && (
            <p className="text-xs text-muted shrink-0">{t('database.rowsLimited')}</p>
          )}
        </div>
      )}
    </div>
  )
}
