import { describe, it, expect, vi } from 'vitest'
import type { ReactNode } from 'react'
import { render, screen, within, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { axe } from 'vitest-axe'
import { DataTable, type Column } from './DataTable'

type Row = { id: string, name: string, n: number }
type Key = 'name' | 'n' | 'act'

// name-asc, name-desc, n-asc, n-desc are all distinct orders, so a sort
// assertion can't pass by coincidence.
const ROWS: Row[] = [
  { id: '1', name: 'Beta', n: 1 },
  { id: '2', name: 'Alpha', n: 3 },
  { id: '3', name: 'Gamma', n: 2 },
]

const COLUMNS: Column<Key>[] = [
  { key: 'name', label: 'Name' },
  { key: 'n', label: 'Count' },
  { key: 'act', label: '', sortable: false },
]

type Overrides = Partial<React.ComponentProps<typeof DataTable<Row, Key>>>

function renderTable(overrides: Overrides = {}) {
  return render(
    <DataTable<Row, Key>
      aria-label="Test table"
      columns={COLUMNS}
      rows={ROWS}
      rowId={(r) => r.id}
      sortValue={(r, k) => (k === 'n' ? r.n : k === 'name' ? r.name : '')}
      renderCell={(r, k): ReactNode => (k === 'act' ? <span>—</span> : k === 'n' ? r.n : r.name)}
      {...overrides}
    />,
  )
}

// First column is promoted to row-header, so the name cell of each row is the
// rowheader; reading them in DOM order gives the visible row order.
const rowOrder = () => screen.getAllByRole('rowheader').map((c) => c.textContent)

describe('DataTable', () => {
  it('renders rows sorted by the initial column ascending by default', () => {
    renderTable()
    expect(rowOrder()).toEqual(['Alpha', 'Beta', 'Gamma'])
  })

  it('toggles sort direction when a sortable header is clicked', async () => {
    renderTable()
    await userEvent.click(screen.getByRole('button', { name: /Name/ }))
    expect(rowOrder()).toEqual(['Gamma', 'Beta', 'Alpha'])
  })

  it('sorts numerically via sortValue on a numeric column', async () => {
    renderTable()
    await userEvent.click(screen.getByRole('button', { name: /Count/ }))
    // n ascending: Beta(1), Gamma(2), Alpha(3)
    expect(rowOrder()).toEqual(['Beta', 'Gamma', 'Alpha'])
  })

  it('seeds order and direction from initialSort', () => {
    renderTable({ initialSort: { column: 'n', direction: 'descending' } })
    // n descending: Alpha(3), Gamma(2), Beta(1)
    expect(rowOrder()).toEqual(['Alpha', 'Gamma', 'Beta'])
  })

  it('renders no sort button for a non-sortable column but keeps an accessible name', () => {
    renderTable()
    const headers = screen.getAllByRole('columnheader')
    const actionsHeader = headers[2]
    expect(within(actionsHeader).queryByRole('button')).toBeNull()
    // empty label falls back to the sr-only column key
    expect(actionsHeader).toHaveTextContent('act')
  })

  it('reflects the current sort on the header via aria-sort', () => {
    renderTable()
    expect(screen.getAllByRole('columnheader')[0]).toHaveAttribute('aria-sort', 'ascending')
    expect(screen.getAllByRole('columnheader')[1]).toHaveAttribute('aria-sort', 'none')
  })

  it('shows the empty state when there are no rows', () => {
    renderTable({ rows: [], emptyState: <div>No rows here</div> })
    expect(screen.getByText('No rows here')).toBeInTheDocument()
    expect(screen.queryByText('Alpha')).toBeNull()
  })

  it('shows skeleton rows while loading and hides data', () => {
    const { container } = renderTable({ loading: true, skeletonRows: 4 })
    // 4 skeleton rows x 3 columns
    expect(container.querySelectorAll('[data-slot="skeleton"]')).toHaveLength(12)
    expect(screen.queryByText('Alpha')).toBeNull()
  })

  it('fires onRowAction with the row on click', async () => {
    const onRowAction = vi.fn()
    renderTable({ onRowAction })
    const firstDataRow = screen.getAllByRole('row')[1] // [0] is the header row
    await userEvent.click(firstDataRow)
    expect(onRowAction).toHaveBeenCalledWith(expect.objectContaining({ name: 'Alpha' }))
  })

  it('fires onRowAction from the keyboard (Enter)', () => {
    const onRowAction = vi.fn()
    renderTable({ onRowAction })
    const firstDataRow = screen.getAllByRole('row')[1]
    fireEvent.keyDown(firstDataRow, { key: 'Enter' })
    expect(onRowAction).toHaveBeenCalledWith(expect.objectContaining({ name: 'Alpha' }))
  })

  it('renders semantic grid roles', () => {
    renderTable()
    expect(screen.getByRole('grid', { name: 'Test table' })).toBeInTheDocument()
    expect(screen.getAllByRole('columnheader')).toHaveLength(3)
    expect(screen.getAllByRole('rowheader')).toHaveLength(3) // one per data row (promoted first col)
  })

  it('renders rows when virtualization is enabled', () => {
    // In jsdom the scroll element has no measured height; the component falls
    // back to (or overscans) so rows still render and stay testable.
    renderTable({ virtualized: true, rowHeight: 36 })
    expect(rowOrder()).toEqual(['Alpha', 'Beta', 'Gamma'])
  })

  it('orders rows by initialSort even when the target column is not a sort control', () => {
    const cols: Column<'rank' | 'name'>[] = [
      { key: 'rank', label: 'Rank', sortable: false },
      { key: 'name', label: 'Name', isRowHeader: true },
    ]
    render(
      <DataTable<Row, 'rank' | 'name'>
        aria-label="Ranked"
        columns={cols}
        rows={ROWS}
        rowId={(r) => r.id}
        sortValue={(r, k) => (k === 'rank' ? r.n : r.name)}
        renderCell={(r, k): ReactNode => (k === 'rank' ? r.n : r.name)}
        initialSort={{ column: 'rank', direction: 'descending' }}
      />,
    )
    // rank descending: Alpha(3), Gamma(2), Beta(1)
    expect(rowOrder()).toEqual(['Alpha', 'Gamma', 'Beta'])
    // ...but the non-sortable column still exposes no header sort control
    expect(within(screen.getAllByRole('columnheader')[0]).queryByRole('button')).toBeNull()
  })

  it('gives sort header buttons a visible focus ring', () => {
    renderTable()
    expect(screen.getByRole('button', { name: /Name/ }).className).toContain('focus-visible:ring-ring')
  })

  it('makes interactive rows a focusable tab stop with a focus ring', () => {
    renderTable({ onRowAction: vi.fn() })
    const firstDataRow = screen.getAllByRole('row')[1]
    expect(firstDataRow).toHaveAttribute('tabindex', '0')
    expect(firstDataRow.className).toContain('focus-visible:ring-ring')
  })

  it('stamps a logical parity on each row so zebra striping is stable', () => {
    renderTable()
    const dataRows = screen.getAllByRole('row').slice(1)
    expect(dataRows.map((r) => r.getAttribute('data-parity'))).toEqual(['0', '1', '0'])
  })

  it('has no axe accessibility violations', async () => {
    const { container } = renderTable()
    expect(await axe(container)).toHaveNoViolations()
  })

  it('has no axe accessibility violations when virtualized', async () => {
    const { container } = renderTable({ virtualized: true })
    expect(await axe(container)).toHaveNoViolations()
  })
})
