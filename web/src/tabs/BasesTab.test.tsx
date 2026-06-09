import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { ReactElement } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { axe } from 'vitest-axe'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

// Return the i18n key verbatim so assertions are deterministic without booting
// the full i18next instance.
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

// toast facade spy — assert non-404 failures surface a danger toast.
vi.mock('@/components/ui/toast', () => ({
  toast: { danger: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() },
}))

// Mock the API client but keep the real ApiError class for instanceof checks.
vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      bases: {
        list: vi.fn(),
        exportUrl: (id: number) => `http://test/api/v1/bases/${id}/export`,
      },
    },
  }
})

const { BasesTab } = await import('./BasesTab')
const { api, ApiError } = await import('../api/client')
const { toast } = await import('@/components/ui/toast')

const listMock = vi.mocked(api.bases.list)
const SAMPLE = [{ id: 1, name: 'Alpha Base', pieces: 10, placeables: 5 }]

function renderTab(ui: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
}

describe('BasesTab', () => {
  beforeEach(() => {
    listMock.mockReset()
  })

  it('renders fetched bases in the table', async () => {
    listMock.mockResolvedValue(SAMPLE)
    renderTab(<BasesTab />)
    expect(await screen.findByText('Alpha Base')).toBeInTheDocument()
  })

  it('refetches when the refresh button is clicked', async () => {
    listMock.mockResolvedValue(SAMPLE)
    renderTab(<BasesTab />)
    await screen.findByText('Alpha Base')
    expect(listMock).toHaveBeenCalledTimes(1)
    await userEvent.click(screen.getByRole('button', { name: /common\.refresh/ }))
    await waitFor(() => expect(listMock).toHaveBeenCalledTimes(2))
  })

  it('shows the unavailable card on a 404, not an error toast', async () => {
    listMock.mockRejectedValue(new ApiError(404, 'not found'))
    renderTab(<BasesTab />)
    expect(await screen.findByText('bases.featureNotAvailable')).toBeInTheDocument()
    expect(screen.queryByText('Alpha Base')).toBeNull()
    expect(toast.danger).not.toHaveBeenCalled()
  })

  it('surfaces a danger toast on a non-404 failure', async () => {
    listMock.mockRejectedValue(new Error('boom'))
    renderTab(<BasesTab />)
    await waitFor(() => expect(toast.danger).toHaveBeenCalled())
  })

  it('disables the export action when signed out', async () => {
    listMock.mockResolvedValue(SAMPLE)
    renderTab(<BasesTab isSignedIn={false} />)
    await screen.findByText('Alpha Base')
    expect(screen.getByText('bases.layoutAccountStrong')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /bases\.export/ })).toBeDisabled()
  })

  it('has no axe accessibility violations', async () => {
    listMock.mockResolvedValue(SAMPLE)
    const { container } = renderTab(<BasesTab />)
    await screen.findByText('Alpha Base')
    expect(await axe(container)).toHaveNoViolations()
  })
})
