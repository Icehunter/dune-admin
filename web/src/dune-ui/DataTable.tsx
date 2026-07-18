import * as React from 'react'
import { Pagination, Skeleton } from '@heroui/react'
import type { DataGridColumn } from '@heroui-pro/react'
import { DataGrid as HeroDataGrid } from '@heroui-pro/react'
import type { Column, DataTableProps, SortDescriptor } from './types'

export type { Column }

// buildComparator sorts the FULL row set before pagination slices it (#292):
// sorting must be applied dataset-wide, not per visible page, so it lives here
// (controlled sortDescriptor) instead of in DataGrid's per-column sortFn, which
// only ever sees the current page. Value extraction mirrors the old sortFn:
// sortValue prop first, then renderCell coerced to string/number, numeric
// compare when both numbers, numeric-aware localeCompare otherwise.
const buildComparator = <T extends object, K extends string>(
  sort: SortDescriptor,
  sortValue: ((row: T, key: K) => string | number | null | undefined) | undefined,
  renderCell: (row: T, key: K) => React.ReactNode,
): ((a: T, b: T) => number) => {
  const colKey = String(sort.column) as K
  const getVal = sortValue
    ? (r: T): string | number | null | undefined => sortValue(r, colKey)
    : (r: T): string | number => {
        const v = renderCell(r, colKey)
        return typeof v === 'string' || typeof v === 'number' ? v : String(v ?? '')
      }
  const dir = sort.direction === 'descending' ? -1 : 1
  return (a: T, b: T): number => {
    const av = getVal(a)
    const bv = getVal(b)
    const base = typeof av === 'number' && typeof bv === 'number'
      ? av - bv
      : String(av ?? '').localeCompare(String(bv ?? ''), undefined, { numeric: true })
    return base * dir
  }
}

const buildPages = (current: number, total: number): Array<number | 'ellipsis'> => {
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
}: DataTableProps<T, K>): React.ReactElement => {
  const renderCellRef = React.useRef(renderCell)
  const onRowActionRef = React.useRef(onRowAction)
  React.useEffect(() => {
    renderCellRef.current = renderCell
    onRowActionRef.current = onRowAction
  })

  const [page, setPage] = React.useState(1)
  React.useEffect(() => {
    Promise.resolve().then(() => setPage(1))
  }, [rows])

  const [sort, setSort] = React.useState<SortDescriptor | undefined>(initialSort)

  // Sort the WHOLE dataset first, then slice the page (#292) — the reverse
  // order sorted only the visible page.
  const sortedRows = sort
    ? [...rows].sort(buildComparator<T, K>(sort, sortValue, renderCell))
    : rows
  const totalPages = pageSize ? Math.ceil(sortedRows.length / pageSize) : 1
  const pagedRows = pageSize ? sortedRows.slice((page - 1) * pageSize, page * pageSize) : sortedRows

  const rowsRef = React.useRef(pagedRows)
  React.useEffect(() => {
    rowsRef.current = pagedRows
  })

  const hasExplicitRowHeader = columns.some((c) => c.isRowHeader)

  const gridColumns: DataGridColumn<T>[] = columns.map((col, i) => {
    const sortable = col.sortable !== false
    const colKey = col.key as K
    // fr widths resolve fine in the standard (resize-state) layout but not in
    // the virtualizer's JS column resolution — strip them only when virtualized.
    const resolvedWidth = typeof col.width === 'string' && col.width.endsWith('fr') && virtualized ? undefined : col.width
    return {
      id: col.key,
      header: col.label,
      isRowHeader: col.isRowHeader ?? (!hasExplicitRowHeader && i === 0),
      allowsSorting: sortable,
      // DataGrid virtualizer resolves columns in JS — CSS fr units aren't supported; omit so the column auto-stretches
      ...(resolvedWidth !== undefined ? { width: resolvedWidth } : {}),
      ...(col.minWidth !== undefined ? { minWidth: col.minWidth } : {}),
      ...(col.pinned !== undefined ? { pinned: col.pinned } : {}),
      ...(col.align !== undefined ? { align: col.align } : {}),
      cell: (row: T) => {
        const maxWidth = typeof col.width === 'number' ? col.width : undefined
        const content = renderCellRef.current(row, colKey)
        // Plain-text cells ellipsize by default so long values can never widen
        // the (fixed-layout) table; component cells manage their own overflow.
        const wrapped = typeof content === 'string' || typeof content === 'number'
          ? <span className="block truncate">{content}</span>
          : content
        return col.align === 'end' || col.key === 'actions'
          ? <div className="flex justify-end items-center w-full gap-1">{wrapped}</div>
          : <div className="overflow-hidden min-w-0 w-full" style={maxWidth ? { maxWidth } : undefined}>{wrapped}</div>
      },
    }
  })

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

  const gridClassName = pageSize ? 'h-full' : className
  const gridContentClassName = pageSize ? undefined : contentClassName
  const gridScrollClassName = pageSize ? 'h-full overflow-auto' : scrollContainerClassName

  const grid = (
    <HeroDataGrid
      aria-label={ariaLabel}
      columns={gridColumns}
      data={pagedRows}
      getRowId={rowId}
      // Activates the column-width resolution container: without it the
      // per-column `width` props are inert and (with table-layout: fixed)
      // every column gets an equal share. No per-column resize handles are
      // added — columns opt in via `allowsResizing`, which we don't set.
      allowsColumnResize
      {...(gridClassName !== undefined ? { className: gridClassName } : {})}
      {...(gridContentClassName !== undefined ? { contentClassName: gridContentClassName } : {})}
      {...(gridScrollClassName !== undefined ? { scrollContainerClassName: gridScrollClassName } : {})}
      virtualized={pageSize ? false : virtualized}
      rowHeight={rowHeight}
      headingHeight={36}
      selectionMode={selectionMode ?? 'none'}
      showSelectionCheckboxes={selectionMode === 'multiple'}
      {...(selectedKeys !== undefined ? { selectedKeys } : {})}
      {...(onSelectionChange !== undefined ? { onSelectionChange } : {})}
      {...(emptyState !== undefined
        ? { renderEmptyState: () => <React.Fragment>{emptyState}</React.Fragment> }
        : {})}
      {...(sort !== undefined ? { sortDescriptor: sort } : {})}
      onSortChange={(d: SortDescriptor) => {
        setSort(d)
        setPage(1)
      }}
      {...(onRowAction !== undefined
        ? {
            onRowAction: (key: string | number) => {
              const row = rowsRef.current.find((r) => String(rowId(r)) === String(key))
              if (row) onRowActionRef.current?.(row)
            },
          }
        : {})}
    />
  )

  if (!pageSize) return grid

  return (
    // w-full + min-w-0: as a flex item this wrapper otherwise sizes to the
    // table's intrinsic content width (min-width:auto), making the grid
    // underfill wide containers and overflow narrow ones. Pinning it to the
    // container width lets the grid resolve its columns against the real
    // available space — fill the screen, never exceed it.
    <div className="flex flex-col gap-2 h-full min-h-0 w-full min-w-0">
      <div className="flex-1 min-h-0 min-w-0 overflow-hidden">
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
