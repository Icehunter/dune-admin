import { useRef, useMemo, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import type { Selection } from '@heroui/react'
import { Pagination, Skeleton } from '@heroui/react'
import type { DataGridColumn, DataGridColumnSize } from '@heroui-pro/react'
import { DataGrid as HeroDataGrid } from '@heroui-pro/react'

function buildPages(current: number, total: number): Array<number | 'ellipsis'> {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1)
  const pages: Array<number | 'ellipsis'> = [1]
  if (current > 3) pages.push('ellipsis')
  const lo = Math.max(2, current - 1)
  const hi = Math.min(total - 1, current + 1)
  for (let i = lo; i <= hi; i++) pages.push(i)
  if (current < total - 2) pages.push('ellipsis')
  pages.push(total)
  return pages
}

export type Column<K extends string> = {
  key: K
  label: string
  /** Whether this column is sortable. Defaults to true. */
  sortable?: boolean
  /** Marks the row-header column (accessibility). Typically the first one. */
  isRowHeader?: boolean
  /** Fixed column width (px, %, or fr — e.g. 70 or '1fr'). When omitted, column takes remaining space. */
  width?: DataGridColumnSize
  /** Minimum column width (px). Useful for the stretchy column or pinned columns. */
  minWidth?: number
  /** Pin this column to start or end so it stays visible during horizontal scroll. */
  pinned?: 'start' | 'end'
  /** Cell text alignment. */
  align?: 'start' | 'center' | 'end'
}

type DataTableProps<T, K extends string> = {
  /** Accessibility label — required by React Aria. */
  'aria-label': string
  'columns': Column<K>[]
  'rows': T[]
  /** Stable id extractor for each row. */
  'rowId': (row: T) => string
  /** Render the cell content for a given row + column key. */
  'renderCell': (row: T, key: K) => ReactNode
  /** Default sort column + direction (uncontrolled). */
  'initialSort'?: { column: K, direction: 'ascending' | 'descending' }
  /** Custom value getter for sorting (defaults to renderCell coerced to string). */
  'sortValue'?: (row: T, key: K) => string | number | null | undefined
  /** Rendered when `rows` is empty. */
  'emptyState'?: ReactNode
  /** Shows skeleton rows instead of data while true. */
  'loading'?: boolean
  /** Number of skeleton rows while loading. Defaults to 5. */
  'skeletonRows'?: number
  /** Called when a row is clicked / activated. */
  'onRowAction'?: (row: T) => void
  /** Extra classes for the root DataGrid wrapper. */
  'className'?: string
  /**
   * Extra classes for the inner scroll/content area.
   * For virtualized grids, pass a fixed height here, e.g. contentClassName="h-[500px] overflow-auto".
   * This mirrors the docs example pattern exactly.
   */
  'contentClassName'?: string
  /** Extra classes for the scroll container (non-virtualized overflow). */
  'scrollContainerClassName'?: string
  /** Opt into row virtualization for large datasets (1 000+ rows). */
  'virtualized'?: boolean
  /**
   * Fixed row height in px — required when `virtualized` is true.
   * Use ~42 for single-line rows, ~48 for rows with buttons. Defaults to 42.
   */
  'rowHeight'?: number
  /** Row selection mode. Defaults to 'none'. */
  'selectionMode'?: 'none' | 'single' | 'multiple'
  /** Controlled selected row keys. */
  'selectedKeys'?: Selection
  /** Callback when selection changes. */
  'onSelectionChange'?: (keys: Selection) => void
  /** When set, enables pagination and caps each page to this many rows. */
  'pageSize'?: number
}

export const DataTable = <T extends object, K extends string>({
  'aria-label': ariaLabel,
  columns,
  rows,
  rowId,
  renderCell,
  initialSort,
  sortValue,
  emptyState,
  loading = false,
  skeletonRows = 5,
  onRowAction,
  className,
  contentClassName,
  scrollContainerClassName,
  virtualized = false,
  rowHeight = 42,
  selectionMode,
  selectedKeys,
  onSelectionChange,
  pageSize,
}: DataTableProps<T, K>) => {
  const renderCellRef = useRef(renderCell)
  const sortValueRef = useRef(sortValue)
  const onRowActionRef = useRef(onRowAction)
  useEffect(() => {
    renderCellRef.current = renderCell
    sortValueRef.current = sortValue
    onRowActionRef.current = onRowAction
  })

  const [page, setPage] = useState(1)
  useEffect(() => {
    Promise.resolve().then(() => setPage(1))
  }, [rows])

  const totalPages = pageSize ? Math.ceil(rows.length / pageSize) : 1
  const pagedRows = pageSize ? rows.slice((page - 1) * pageSize, page * pageSize) : rows

  const rowsRef = useRef(pagedRows)
  useEffect(() => {
    rowsRef.current = pagedRows
  })

  const columnKey = columns
    .map((c) => [c.key, c.label, c.width, c.minWidth, c.sortable, c.isRowHeader, c.pinned, c.align].join(':'))
    .join('|')

  const hasExplicitRowHeader = columns.some((c) => c.isRowHeader)

  const gridColumns = useMemo<DataGridColumn<T>[]>(() => columns.map((col, i) => {
    const sortable = col.sortable !== false
    const colKey = col.key as K
    return {
      id: col.key,
      header: col.label,
      isRowHeader: col.isRowHeader ?? (!hasExplicitRowHeader && i === 0),
      allowsSorting: sortable,
      // DataGrid virtualizer resolves columns in JS — CSS fr units aren't supported; omit so the column auto-stretches
      width: typeof col.width === 'string' && col.width.endsWith('fr') ? undefined : col.width,
      minWidth: col.minWidth,
      pinned: col.pinned,
      align: col.align,
      cell: (row: T) => {
        const maxWidth = typeof col.width === 'number' ? col.width : undefined
        return col.align === 'end' || col.key === 'actions'
          ? <div className="flex justify-end items-center w-full gap-1">{renderCellRef.current(row, colKey)}</div>
          : <div className="overflow-hidden min-w-0 w-full" style={maxWidth ? { maxWidth } : undefined}>{renderCellRef.current(row, colKey)}</div>
      },
      ...(sortable && {
        sortFn: (a: T, b: T) => {
          const sv = sortValueRef.current
          const getVal = sv
            ? (r: T) => sv(r, colKey)
            : (r: T) => {
                const v = renderCellRef.current(r, colKey)
                return typeof v === 'string' || typeof v === 'number' ? v : String(v ?? '')
              }
          const av = getVal(a)
          const bv = getVal(b)
          if (typeof av === 'number' && typeof bv === 'number') return av - bv
          return String(av ?? '').localeCompare(String(bv ?? ''), undefined, { numeric: true })
        },
      }),
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- columnKey fingerprints structure; callbacks live in refs
  }), [columnKey, hasExplicitRowHeader])

  if (loading) {
    return (
      <div className={`border border-border/60 rounded-md overflow-hidden ${className ?? ''}`}>
        {Array.from({ length: skeletonRows }, (_, i) => (
          <div
            key={i}
            className="flex gap-3 px-3 py-2.5 border-b border-border/40 last:border-0"
          >
            {columns.map((c) => (
              <Skeleton key={c.key} className="h-3.5 rounded flex-1" />
            ))}
          </div>
        ))}
      </div>
    )
  }

  const grid = (
    <HeroDataGrid
      aria-label={ariaLabel}
      columns={gridColumns}
      data={pagedRows}
      getRowId={rowId}
      className={pageSize ? 'h-full' : className}
      contentClassName={pageSize ? undefined : contentClassName}
      scrollContainerClassName={pageSize ? 'h-full overflow-auto' : scrollContainerClassName}
      virtualized={pageSize ? false : virtualized}
      rowHeight={rowHeight}
      headingHeight={36}
      selectionMode={selectionMode ?? 'none'}
      showSelectionCheckboxes={selectionMode === 'multiple'}
      selectedKeys={selectedKeys}
      onSelectionChange={onSelectionChange}
      renderEmptyState={emptyState ? () => <>{emptyState}</> : undefined}
      defaultSortDescriptor={
        initialSort
          ? { column: initialSort.column, direction: initialSort.direction }
          : undefined
      }
      onRowAction={
        onRowAction
          ? (key: string | number) => {
              const row = rowsRef.current.find((r) => String(rowId(r)) === String(key))
              if (row) onRowActionRef.current?.(row)
            }
          : undefined
      }
    />
  )

  if (!pageSize) return grid

  return (
    <div className="flex flex-col gap-2 h-full min-h-0">
      <div className="flex-1 min-h-0 overflow-hidden">
        {grid}
      </div>
      {totalPages > 1 && (
        <div className="flex items-center justify-between shrink-0 py-1 px-1">
          <span className="text-xs text-muted tabular-nums whitespace-nowrap">
            {(page - 1) * pageSize + 1}
            {' – '}
            {Math.min(page * pageSize, rows.length)}
            {' of '}
            {rows.length}
          </span>
          <Pagination size="sm" className="ml-auto w-auto">
            <Pagination.Content>
              <Pagination.Item>
                <Pagination.Previous isDisabled={page === 1} onPress={() => setPage((p) => Math.max(1, p - 1))}>
                  <Pagination.PreviousIcon />
                </Pagination.Previous>
              </Pagination.Item>
              {buildPages(page, totalPages).map((p, i) =>
                p === 'ellipsis'
                  ? (
                      <Pagination.Item key={`ellipsis-${i}`}>
                        <Pagination.Ellipsis />
                      </Pagination.Item>
                    )
                  : (
                      <Pagination.Item key={p}>
                        <Pagination.Link isActive={p === page} onPress={() => setPage(p)}>
                          {p}
                        </Pagination.Link>
                      </Pagination.Item>
                    ),
              )}
              <Pagination.Item>
                <Pagination.Next
                  isDisabled={page === totalPages}
                  onPress={() => setPage((p) => Math.min(totalPages, p + 1))}
                >
                  <Pagination.NextIcon />
                </Pagination.Next>
              </Pagination.Item>
            </Pagination.Content>
          </Pagination>
        </div>
      )}
    </div>
  )
}
