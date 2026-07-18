import * as React from 'react'
import type { Selection } from '@heroui/react'
import type { DataGridColumnSize } from '@heroui-pro/react'

// Structurally identical to react-aria's SortDescriptor (not re-exported by
// @heroui-pro/react's package root, so declared here).
export type SortDescriptor = {
  column: string | number
  direction: 'ascending' | 'descending'
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

export type DataTableProps<T, K extends string> = {
  /** Accessibility label — required by React Aria. */
  'aria-label': string
  'columns': Column<K>[]
  'rows': T[]
  /** Stable id extractor for each row. */
  'rowId': (row: T) => string
  /** Render the cell content for a given row + column key. */
  'renderCell': (row: T, key: K) => React.ReactNode
  /** Default sort column + direction (uncontrolled). */
  'initialSort'?: { column: K, direction: 'ascending' | 'descending' }
  /** Custom value getter for sorting (defaults to renderCell coerced to string). */
  'sortValue'?: (row: T, key: K) => string | number | null | undefined
  /** Rendered when `rows` is empty. */
  'emptyState'?: React.ReactNode
  /** Shows skeleton rows instead of data while true. */
  'loading'?: boolean | undefined
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
  'selectionMode'?: 'none' | 'single' | 'multiple' | undefined
  /** Controlled selected row keys. */
  'selectedKeys'?: Selection
  /** Callback when selection changes. */
  'onSelectionChange'?: (keys: Selection) => void
  /** When set, enables pagination and caps each page to this many rows. */
  'pageSize'?: number
}

export type ConfirmDialogProps = {
  open: boolean
  title: string
  description: React.ReactNode
  confirmLabel?: string | undefined
  onConfirm: () => void
  onCancel: () => void
}

export type CardProps = { children: React.ReactNode, className?: string }

export type ItemProps = {
  label: React.ReactNode
  value: React.ReactNode
  /** Optional explicit value text color (e.g. phase status color). */
  valueColor?: string
}

export type IconProps = {
  /** Lucide icon name (without the `lucide:` prefix), e.g. "refresh-cw". */
  name: string
  /** Optional size class — defaults to `size-4` (1rem square). */
  className?: string | undefined
}

export type DropzoneProps = {
  /** Comma-separated list of accepted file extensions, e.g. ".json" or ".backup,.zip". */
  accept: string
  /** Called with the chosen file (drag-drop or click-to-pick). */
  onSelect: (file: File) => void
  /** Show this file's name + size as a "selected" state inside the dropzone. */
  file?: File | null
  /** Override the default prompt text shown when nothing is selected. */
  prompt?: React.ReactNode
  /** Spinner overlay — drive from parent state when an upload is in flight. */
  uploading?: boolean
  /** Compact (less vertical padding). */
  compact?: boolean
  className?: string
}

export type SectionDividerProps = {
  title: React.ReactNode
  /** Optional action buttons rendered on the right side of the divider. */
  children?: React.ReactNode
}

export type LoadingStateProps = {
  /** Vertical padding size. Defaults to 'lg' (py-12). */
  size?: 'sm' | 'md' | 'lg'
  /** Fill available height with flex-1 (use inside a flex column). */
  fill?: boolean
  className?: string
}

export type PageHeaderProps = {
  title: React.ReactNode
  /** Optional descriptive subtitle below the title. */
  subtitle?: React.ReactNode
  /** When provided, a refresh button is rendered in the action slot. */
  onRefresh?: () => void
  /** Shows a spinner in the refresh button while true. */
  loading?: boolean
  /** Seconds until next auto-refresh — shown as a dim countdown beside "Refresh". */
  countdown?: number
  /** Additional action buttons / controls rendered on the right. */
  children?: React.ReactNode
}

export type PanelProps = {
  children: React.ReactNode
  className?: string
  /** Extra classes for the inner Widget.Content (e.g. `flex-1 min-h-0` so a
   *  paged DataTable can fill the panel and keep its pager visible). */
  contentClassName?: string
}

export type SideNavRenderSlot = React.ReactNode | ((active: boolean) => React.ReactNode)

export type SideNavItem<K extends string> = {
  key: K
  label: React.ReactNode
  /** Optional sub-label rendered below the main label (e.g. namespace, item count). */
  sublabel?: React.ReactNode
  /** Optional right-aligned hint (e.g. "18 items" count chip). Receives active state when a function. */
  hint?: SideNavRenderSlot
  /** Optional left icon/avatar rendered outside the truncated label area. Receives active state when a function. */
  icon?: SideNavRenderSlot
  /** Indentation level (0 = top-level, 1 = child item). */
  depth?: number
}

export type TimeInputProps = {
  value: string
  onChange: (value: string) => void
  ariaLabel?: string
  className?: string
  isDisabled?: boolean
}

export type SideNavProps<K extends string> = {
  items: SideNavItem<K>[]
  active: K | null
  onSelect: (key: K) => void
  /** Header text shown above the list (e.g. "PODS", "CONTAINERS (70)"). */
  title?: React.ReactNode
  /** Action element rendered next to the title (e.g. a refresh button). */
  titleAction?: React.ReactNode
  /** Width of the side nav. Defaults to 240px (w-60). */
  width?: string
  /** Content rendered above the list (search field, custom nav items, etc.). */
  children?: React.ReactNode
  /** Content rendered between children and the list (e.g. a filter control). */
  listHeader?: React.ReactNode
  /** Shown inside the list area when items is empty. */
  emptyContent?: React.ReactNode
}
