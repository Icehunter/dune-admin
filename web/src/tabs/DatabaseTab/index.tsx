import { useCallback, useEffect, useMemo, useState } from 'react'
import type React from 'react'
import { useTranslation } from 'react-i18next'
import CodeMirror from '@uiw/react-codemirror'
import { sql as sqlLang, PostgreSQL } from '@codemirror/lang-sql'
import { keymap } from '@codemirror/view'
import { Prec } from '@codemirror/state'
import { acceptCompletion } from '@codemirror/autocomplete'
import { Button, SearchField, Spinner, toast } from '@heroui/react'
import { api } from '../../api/client'
import { Icon, LoadingState, NumberInput, PageHeader, SideNav } from '../../dune-ui'
import { duneTheme, type Section } from './constants'
import { ResultTable } from './components/ResultTable'
import { TableSearchInput } from './components/TableSearchInput'
import { BackupsView } from './components/BackupsView'

type TableData = { headers: string[], rows: string[][] }

interface DatabaseTabProps {
  showSubnav?: boolean
  section?: Section
  onSectionChange?: (s: Section) => void
}

export const DatabaseTab: React.FC<DatabaseTabProps> = ({
  section = 'backups',
  onSectionChange,
  showSubnav,
}) => {
  const { t } = useTranslation()

  const SECTIONS = useMemo<{ key: Section, label: string }[]>(() => [
    { key: 'backups', label: t('database.sections.backups') },
    { key: 'tables', label: t('database.sections.tables') },
    { key: 'describe', label: t('database.sections.describe') },
    { key: 'sample', label: t('database.sections.sample') },
    { key: 'search', label: t('database.sections.search') },
    { key: 'sql', label: t('database.sections.sql') },
  ], [t])

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

  const innerContent = (
    <>
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
        <LoadingState size="md" className="shrink-0" />
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
    </>
  )

  // The Backups section is self-contained (loads its own data); every other
  // section shares the query/inspect shell above.
  const body = section === 'backups' ? <BackupsView /> : innerContent

  if (showSubnav) {
    return (
      <div className="h-full min-h-0 flex gap-3">
        <SideNav
          title={t('database.sideNavTitle')}
          items={SECTIONS}
          active={section ?? 'backups'}
          onSelect={(key) => onSectionChange?.(key)}
        />
        <div className="flex-1 min-h-0 flex flex-col gap-3">
          {body}
        </div>
      </div>
    )
  }

  return (
    <div className="h-full min-h-0 flex flex-col gap-3">
      {body}
    </div>
  )
}
