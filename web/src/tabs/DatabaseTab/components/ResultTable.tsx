import { useTranslation } from 'react-i18next'
import { DataTable, type Column } from '../../../dune-ui'

type TableData = { headers: string[], rows: string[][] }

export function ResultTable({ headers, rows }: TableData) {
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
