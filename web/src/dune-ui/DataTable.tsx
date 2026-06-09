import { useState, useMemo, type ReactNode } from 'react'
import { Skeleton, Table, TableLayout, Virtualizer } from '@heroui/react'
import type { SortDescriptor } from '@heroui/react'
import { Icon } from './Icon'

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

type ColumnRenderProps = { sortDirection?: 'ascending' | 'descending' }

type DataTableProps<T, K extends string> = {
  /** Accessibility label, required by React Aria. */
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
  /** Extra classes for the outer Table element. */
  'className'?: string
  /**
   * Opt into HeroUI's TableLayout virtualizer. Set when row count can be
   * large (>200). Only renders rows in the viewport; massive speedup for
   * filter typing on large datasets. Requires `rowHeight` to be the actual
   * rendered row height in px (default 32 matches our compact density).
   *
   * NOTE: virtualization requires row type `T` to be an **object** (React
   * Aria stores items in a WeakMap keyed by the row). Don't enable this if
   * your rows are primitives (strings/numbers).
   */
  'virtualized'?: boolean
  'rowHeight'?: number
}

/**
 * Opinionated HeroUI Table wrapper with built-in sort, consistent compact
 * styling (from global CSS), optional virtualization, and a column-driven
 * API so callers don't have to type out the Table.* compound tree by hand.
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
  const [sortDescriptor, setSortDescriptor] = useState<SortDescriptor>(
    initialSort ?? { column: columns[0].key, direction: 'ascending' },
  )

  // React Aria requires at least one column with isRowHeader=true. If no
  // caller-supplied column has it, promote the first column.
  const cols = useMemo<Column<K>[]>(() => {
    if (columns.some((c) => c.isRowHeader)) return columns
    return columns.map((c, i) => i === 0 ? { ...c, isRowHeader: true } : c)
  }, [columns])

  const sorted = useMemo(() => {
    const col = sortDescriptor.column as K
    const dir = sortDescriptor.direction === 'descending' ? -1 : 1
    const get = sortValue ?? ((row: T, key: K) => String(renderCell(row, key)))
    return [...rows].sort((a, b) => {
      const av = get(a, col)
      const bv = get(b, col)
      if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * dir
      return String(av ?? '').localeCompare(String(bv ?? ''), undefined, { numeric: true }) * dir
    })
  }, [rows, sortDescriptor, sortValue, renderCell])

  const tableJSX = (
    <Table className={`bg-transparent border-0 p-0 ${className ?? ''}`}>
      <Table.ScrollContainer className="p-0 border border-border/60 rounded-md">
        <Table.Content
          aria-label={ariaLabel}
          sortDescriptor={sortDescriptor}
          onSortChange={setSortDescriptor}
          {...(!loading && onRowAction
            ? {
                onRowAction: (key) => {
                  const row = sorted.find((r) => rowId(r) === String(key))
                  if (row) onRowAction(row)
                },
              }
            : {})}
        >
          <Table.Header columns={cols}>
            {(col: Column<K>) => {
              const sortable = col.sortable !== false && !loading
              return (
                <Table.Column
                  id={col.key}
                  allowsSorting={sortable}
                  {...(col.isRowHeader ? { isRowHeader: true } : {})}
                  {...(col.width !== undefined ? { width: col.width } : {})}
                  {...(col.minWidth !== undefined ? { minWidth: col.minWidth } : {})}
                >
                  {({ sortDirection }: ColumnRenderProps) => (
                    <span className="flex items-center gap-1">
                      <span className="flex-1 truncate">
                        {/* Empty header (e.g. an actions column) still needs an
                            accessible name for screen readers — fall back to the
                            column key, visually hidden. */}
                        {col.label || <span className="sr-only">{col.key}</span>}
                      </span>
                      {sortable && (
                        <Icon
                          name={
                            sortDirection === 'ascending'
                              ? 'chevron-up'
                              : sortDirection === 'descending'
                                ? 'chevron-down'
                                : 'chevrons-up-down'
                          }
                          className={'size-3 shrink-0 ' + (sortDirection ? '' : 'opacity-30')}
                        />
                      )}
                    </span>
                  )}
                </Table.Column>
              )
            }}
          </Table.Header>
          {loading
            ? (
                <Table.Body>
                  {Array.from({ length: skeletonRows }, (_, i) => (
                    <Table.Row key={`skeleton-${i}`} id={`skeleton-${i}`}>
                      {cols.map((c) => (
                        <Table.Cell key={c.key}>
                          <Skeleton className="h-3 w-full rounded" />
                        </Table.Cell>
                      ))}
                    </Table.Row>
                  ))}
                </Table.Body>
              )
            : virtualized
              ? (
                  <Table.Body
                    items={sorted as unknown as object[]}
                    renderEmptyState={emptyState ? () => <>{emptyState}</> : undefined}
                  >
                    {((row: T) => (
                      <Table.Row id={rowId(row)}>
                        {cols.map((c) => (
                          <Table.Cell key={c.key}>{renderCell(row, c.key)}</Table.Cell>
                        ))}
                      </Table.Row>
                    )) as unknown as (item: object) => ReactNode}
                  </Table.Body>
                )
              : (
                  <Table.Body
                    renderEmptyState={emptyState ? () => <>{emptyState}</> : undefined}
                  >
                    {sorted.map((row) => (
                      <Table.Row key={rowId(row)} id={rowId(row)}>
                        {cols.map((c) => (
                          <Table.Cell key={c.key}>{renderCell(row, c.key)}</Table.Cell>
                        ))}
                      </Table.Row>
                    ))}
                  </Table.Body>
                )}
        </Table.Content>
      </Table.ScrollContainer>
    </Table>
  )

  if (!virtualized) return tableJSX
  return (
    <Virtualizer layout={TableLayout} layoutOptions={{ headingHeight: rowHeight, rowHeight }}>
      {tableJSX}
    </Virtualizer>
  )
}
