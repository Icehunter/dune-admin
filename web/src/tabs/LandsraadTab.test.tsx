import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { ReactElement } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { axe } from 'vitest-axe'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { LandsraadOverview } from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('@/components/ui/toast', () => ({
  toast: { danger: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() },
}))

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return { ...actual, api: { ...actual.api, landsraad: { get: vi.fn() } } }
})

const { LandsraadTab } = await import('./LandsraadTab')
const { api } = await import('../api/client')
const { toast } = await import('@/components/ui/toast')

const getMock = vi.mocked(api.landsraad.get)

const SAMPLE: LandsraadOverview = {
  term: {
    term_id: 42,
    start_time: '2026-01-01T00:00:00Z',
    end_time: '2026-02-01T00:00:00Z',
    test_term: false,
    reigning_faction: 'Atreides',
    active_decree: 'Decree A',
    elected_decree: 'Decree B',
    winning_faction: 'Harkonnen',
  },
  decrees: [{ id: 1, name: 'Spice Tax', weight: 3, disabled: false }],
  tasks: [
    { id: 10, board_index: 1, house: 'House Vernius', completed: true, winning_faction: 'Atreides', sysselraad: false, goal_amount: 5000 },
  ],
}

const EMPTY: LandsraadOverview = { term: null, decrees: [], tasks: [] }

function renderTab(ui: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
}

describe('LandsraadTab', () => {
  beforeEach(() => {
    getMock.mockReset()
  })

  it('renders the term, decrees and task board once loaded', async () => {
    getMock.mockResolvedValue(SAMPLE)
    renderTab(<LandsraadTab />)
    expect(await screen.findByText('#42')).toBeInTheDocument()
    expect(screen.getByText('Atreides')).toBeInTheDocument()
    expect(screen.getByText('Spice Tax')).toBeInTheDocument()
    expect(screen.getByText('House Vernius')).toBeInTheDocument()
  })

  it('does NOT flash empty-state messages while still loading', async () => {
    // Hold the request open so the component stays in its initial-loading state.
    let resolve: (v: LandsraadOverview) => void = () => {}
    getMock.mockReturnValue(
      new Promise<LandsraadOverview>((r) => {
        resolve = r
      }),
    )
    renderTab(<LandsraadTab />)

    // During load the "no term" / "no decrees" copy must not appear.
    expect(screen.queryByText('landsraad.noTerm')).toBeNull()
    expect(screen.queryByText('landsraad.noDecrees')).toBeNull()

    // Once it resolves empty, the messages render (loaded-empty, not loading).
    resolve(EMPTY)
    expect(await screen.findByText('landsraad.noTerm')).toBeInTheDocument()
    expect(screen.getByText('landsraad.noDecrees')).toBeInTheDocument()
  })

  it('refetches when the refresh button is clicked', async () => {
    getMock.mockResolvedValue(SAMPLE)
    renderTab(<LandsraadTab />)
    await screen.findByText('#42')
    expect(getMock).toHaveBeenCalledTimes(1)
    await userEvent.click(screen.getByRole('button', { name: /common\.refresh/ }))
    await waitFor(() => expect(getMock).toHaveBeenCalledTimes(2))
  })

  it('surfaces a danger toast on failure', async () => {
    getMock.mockRejectedValue(new Error('boom'))
    renderTab(<LandsraadTab />)
    await waitFor(() => expect(toast.danger).toHaveBeenCalled())
  })

  it('has no axe accessibility violations', async () => {
    getMock.mockResolvedValue(SAMPLE)
    const { container } = renderTab(<LandsraadTab />)
    await screen.findByText('#42')
    expect(await axe(container)).toHaveNoViolations()
  })
})
