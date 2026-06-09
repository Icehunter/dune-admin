import { useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  type ColumnDef,
  type Row,
  type SortingFn,
  type SortingState,
} from '@tanstack/react-table'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Icon } from './Icon'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

export type Column<K extends string> = {
  key: K
  label: string
  /** Whether this column is sortable. Defaults to true. */
  sortable?: boolean
  /** Marks the row-header column. Typically the first one. */
  isRowHeader?: boolean
  /** Fixed column width (px). When omitted, the column takes remaining space. */
  width?: number
  /** Minimum width (px). Useful with `width` omitted for the stretchy column. */
  minWidth?: number
}

type DataTableProps<T, K extends string> = {
  /** Accessibility label, required for the grid. */
  'aria-label': string
  'columns': Column<K>[]
  'rows': T[]
  /** Stable id extractor for each row. */
  'rowId': (row: T) => string
  /** Render the cell content for a given row + column key. */
  'renderCell': (row: T, key: K) => ReactNode
  /** Initial sort column + direction. */
  'initialSort'?: { column: K, direction: 'ascending' | 'descending' }
  /** Custom value getter for sorting (defaults to renderCell-as-string). */
  'sortValue'?: (row: T, key: K) => string | number | null | undefined
  /** Rendered when `rows` is empty. */
  'emptyState'?: ReactNode
  /** Shows skeleton rows instead of data while true. */
  'loading'?: boolean
  /** Number of skeleton rows to show while loading. Defaults to 5. */
  'skeletonRows'?: number
  /** Called when a row is clicked / activated. */
  'onRowAction'?: (row: T) => void
  /** Extra classes for the outer scroll container. */
  'className'?: string
  /**
   * Opt into row virtualization (TanStack Virtual). Set when row count can be
   * large (>200) — only rows in the viewport are rendered. `rowHeight` must be
   * the actual rendered row height in px (default 32 matches our compact
   * density). Rows can be objects or primitives.
   */
  'virtualized'?: boolean
  'rowHeight'?: number
}

/**
 * Generic data grid built on TanStack Table (model + single-column sort) and
 * TanStack Virtual (opt-in row virtualization). Renders a div-based ARIA grid
 * (`role="grid"/row/columnheader/gridcell/rowheader`) so the compact amber
 * styling in index.css (keyed on those roles) applies, while the caller supplies
 * cell content via `renderCell`. Column-driven API — callers never build the
 * table tree by hand.
 */
export const DataTable = <T, K extends string>({
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
  virtualized = false,
  rowHeight = 32,
}: DataTableProps<T, K>) => {
  // Promote the first column to row-header if no caller column claims it, so
  // every row exposes a rowheader cell for assistive tech.
  const cols = useMemo<Column<K>[]>(() => {
    if (columns.some((c) => c.isRowHeader)) return columns
    return columns.map((c, i) => (i === 0 ? { ...c, isRowHeader: true } : c))
  }, [columns])

  const [sorting, setSorting] = useState<SortingState>(
    initialSort
      ? [{ id: initialSort.column, desc: initialSort.direction === 'descending' }]
      : columns.length > 0
        ? [{ id: columns[0].key, desc: false }]
        : [],
  )

  // Each column exposes its sortValue (or renderCell-as-string fallback) as the
  // accessor — TanStack needs a value to make the column sortable — and a custom
  // comparator (numeric when both are numbers, else natural string collation).
  // The comparator returns ascending order; TanStack flips it for `desc`.
  const columnDefs = useMemo<ColumnDef<T>[]>(() => {
    const get = sortValue ?? ((row: T, key: K) => String(renderCell(row, key)))
    return cols.map((col) => ({
      id: col.key,
      accessorFn: (row: T) => get(row, col.key),
      // Sortable in the MODEL for every column, so initialSort (or the default
      // first-column seed) orders rows even when a column's header isn't a sort
      // control — `col.sortable === false` gates only the header affordance.
      // sortUndefined:false hands undefined to our comparator (treated as '').
      sortUndefined: false,
      sortingFn: ((a: Row<T>, b: Row<T>, columnId: string) => {
        const av = a.getValue(columnId) as string | number | null | undefined
        const bv = b.getValue(columnId) as string | number | null | undefined
        if (typeof av === 'number' && typeof bv === 'number') return av - bv
        return String(av ?? '').localeCompare(String(bv ?? ''), undefined, { numeric: true })
      }) as SortingFn<T>,
    }))
  }, [cols, sortValue, renderCell])

  // eslint-disable-next-line react-hooks/incompatible-library -- TanStack returns stable fns; safe here
  const table = useReactTable<T>({
    data: rows,
    columns: columnDefs,
    state: { sorting },
    onSortingChange: setSorting,
    getRowId: (row) => rowId(row),
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    enableMultiSort: false,
    enableSortingRemoval: false,
    sortDescFirst: false,
  })

  const modelRows = table.getRowModel().rows

  // Column track sizing — fixed px for `width`, a min..max band when both are
  // set, a flexible floor for `minWidth`, else an equal flexible share. Applied
  // to every row so columns align.
  const gridTemplateColumns = useMemo(
    () =>
      cols
        .map((c) =>
          c.width != null && c.minWidth != null
            ? `minmax(${c.minWidth}px, ${c.width}px)`
            : c.width != null
              ? `${c.width}px`
              : c.minWidth != null
                ? `minmax(${c.minWidth}px, 1fr)`
                : 'minmax(0, 1fr)',
        )
        .join(' '),
    [cols],
  )

  // Virtualizer is always created (rules of hooks); count is 0 unless virtualized.
  const scrollRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: virtualized ? modelRows.length : 0,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowHeight,
    overscan: 8,
  })
  const virtualItems = virtualizer.getVirtualItems()
  // In jsdom (and before the scroll element is measured) getVirtualItems is
  // empty; fall back to rendering all rows so the virtualized path stays correct
  // and testable rather than rendering nothing.
  const useVirtual = virtualized && virtualItems.length > 0

  const cellRole = (col: Column<K>) => (col.isRowHeader ? 'rowheader' : 'gridcell')

  const renderCells = (row: T) =>
    cols.map((col) => (
      <div key={col.key} role={cellRole(col)} className="flex items-center min-w-0">
        {renderCell(row, col.key)}
      </div>
    ))

  const interactive = !!onRowAction && !loading
  // Interactive rows show a visible focus ring (WCAG 2.4.7 Focus Visible) and a
  // pointer affordance. Shared by the virtual and non-virtual row branches.
  const rowClass = cn(
    'grid',
    interactive && 'cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset',
  )
  // Spread (not inline) so the handler/keyboard pair travels together; rows are
  // fully keyboard-operable (Enter/Space) when an onRowAction is provided.
  const rowInteraction = (row: T) =>
    interactive
      ? {
          tabIndex: 0,
          onClick: () => onRowAction!(row),
          onKeyDown: (e: KeyboardEvent<HTMLDivElement>) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              onRowAction!(row)
            }
          },
        }
      : {}

  return (
    <div ref={scrollRef} className={cn('overflow-auto rounded-md border border-border/60', className)}>
      <div
        role="grid"
        aria-label={ariaLabel}
        aria-rowcount={useVirtual ? modelRows.length + 1 : undefined}
        className="min-w-full text-sm"
      >
        {/* Header */}
        <div role="rowgroup" className="sticky top-0 z-10 bg-background">
          <div role="row" aria-rowindex={useVirtual ? 1 : undefined} className="grid" style={{ gridTemplateColumns }}>
            {cols.map((col) => {
              const column = table.getColumn(col.key)
              const sortable = col.sortable !== false && !loading
              const sorted = column?.getIsSorted() ?? false
              const ariaSort = !sortable
                ? undefined
                : sorted === 'asc'
                  ? 'ascending'
                  : sorted === 'desc'
                    ? 'descending'
                    : 'none'
              const label = col.label || <span className="sr-only">{col.key}</span>
              const chevron = sorted === 'asc' ? 'chevron-up' : sorted === 'desc' ? 'chevron-down' : 'chevrons-up-down'
              return (
                <div key={col.key} role="columnheader" aria-sort={ariaSort} className="table__column">
                  {sortable
                    ? (
                        <button
                          type="button"
                          onClick={() => column?.toggleSorting()}
                          className="flex w-full items-center gap-1 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
                        >
                          <span className="flex-1 truncate">{label}</span>
                          <Icon name={chevron} className={cn('size-3 shrink-0', !sorted && 'opacity-30')} />
                        </button>
                      )
                    : (
                        <span className="flex w-full items-center gap-1">
                          <span className="flex-1 truncate">{label}</span>
                        </span>
                      )}
                </div>
              )
            })}
          </div>
        </div>

        {/* Body */}
        <div role="rowgroup" style={useVirtual ? { position: 'relative', height: virtualizer.getTotalSize() } : undefined}>
          {loading
            ? Array.from({ length: skeletonRows }, (_, i) => (
                <div key={`skeleton-${i}`} role="row" data-parity={i % 2} className="grid" style={{ gridTemplateColumns }}>
                  {cols.map((col) => (
                    <div key={col.key} role={cellRole(col)} className="flex items-center">
                      <Skeleton className="h-3 w-full rounded" />
                    </div>
                  ))}
                </div>
              ))
            : modelRows.length === 0
              ? emptyState != null && (
                <div role="row" className="grid" style={{ gridTemplateColumns }}>
                  <div role="gridcell" aria-colindex={1} aria-colspan={cols.length} style={{ gridColumn: '1 / -1' }}>
                    {emptyState}
                  </div>
                </div>
              )
              : useVirtual
                ? virtualItems.map((vi) => {
                    const row = modelRows[vi.index]
                    return (
                      <div
                        key={row.id}
                        role="row"
                        aria-rowindex={vi.index + 2}
                        data-parity={vi.index % 2}
                        className={rowClass}
                        style={{
                          gridTemplateColumns,
                          position: 'absolute',
                          top: 0,
                          left: 0,
                          width: '100%',
                          height: rowHeight,
                          transform: `translateY(${vi.start}px)`,
                        }}
                        {...rowInteraction(row.original)}
                      >
                        {renderCells(row.original)}
                      </div>
                    )
                  })
                : modelRows.map((row, i) => (
                    <div
                      key={row.id}
                      role="row"
                      data-parity={i % 2}
                      className={rowClass}
                      style={{ gridTemplateColumns }}
                      {...rowInteraction(row.original)}
                    >
                      {renderCells(row.original)}
                    </div>
                  ))}
        </div>
      </div>
    </div>
  )
}
