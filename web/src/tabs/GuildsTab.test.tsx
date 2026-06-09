import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { ReactElement } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { axe } from 'vitest-axe'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { GuildDetail, GuildSummary } from '../api/client'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('@/components/ui/toast', () => ({
  toast: { danger: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() },
}))

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      guilds: { list: vi.fn(), get: vi.fn(), update: vi.fn(), setRole: vi.fn() },
    },
  }
})

const { GuildsTab } = await import('./GuildsTab')
const { api } = await import('../api/client')

const listMock = vi.mocked(api.guilds.list)
const getMock = vi.mocked(api.guilds.get)
const updateMock = vi.mocked(api.guilds.update)
const setRoleMock = vi.mocked(api.guilds.setRole)

const SUMMARY: GuildSummary[] = [
  { guild_id: 1, name: 'House Atreides', description: 'Noble house', faction_id: 1, faction_name: 'Atreides', member_count: 2 },
]
const DETAIL: GuildDetail = {
  ...SUMMARY[0],
  members: [
    { player_id: 10, role_id: 50, character_name: 'Duncan' },
    { player_id: 11, role_id: 100, character_name: 'Leto' },
  ],
  invites: [],
}

function renderTab(ui: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
}

describe('GuildsTab', () => {
  beforeEach(() => {
    listMock.mockReset()
    getMock.mockReset()
    updateMock.mockReset()
    setRoleMock.mockReset()
    listMock.mockResolvedValue(SUMMARY)
    getMock.mockResolvedValue(DETAIL)
    updateMock.mockResolvedValue(DETAIL)
    setRoleMock.mockResolvedValue({ ok: 'ok' })
  })

  it('renders guild rows', async () => {
    renderTab(<GuildsTab />)
    expect(await screen.findByText('House Atreides')).toBeInTheDocument()
  })

  it('opens the detail dialog when a row is clicked', async () => {
    renderTab(<GuildsTab />)
    await screen.findByText('House Atreides')
    await userEvent.click(screen.getAllByRole('row')[1]) // [0] is the header row
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Duncan')).toBeInTheDocument()
    expect(within(dialog).getByText('Leto')).toBeInTheDocument()
  })

  it('saves edits via the update mutation', async () => {
    renderTab(<GuildsTab />)
    await screen.findByText('House Atreides')
    await userEvent.click(screen.getAllByRole('row')[1])
    await screen.findByRole('dialog')
    await userEvent.click(await screen.findByRole('button', { name: 'guilds.save' }))
    await waitFor(() =>
      expect(updateMock).toHaveBeenCalledWith(1, expect.objectContaining({ name: 'House Atreides' })),
    )
  })

  it('promotes a member via the setRole mutation', async () => {
    renderTab(<GuildsTab />)
    await screen.findByText('House Atreides')
    await userEvent.click(screen.getAllByRole('row')[1])
    await screen.findByRole('dialog')
    await userEvent.click(screen.getByRole('button', { name: 'guilds.makeAdmin' }))
    await waitFor(() => expect(setRoleMock).toHaveBeenCalledWith(1, 10, 100))
  })

  it('shows a read-only view (no edit form) when signed out', async () => {
    renderTab(<GuildsTab isSignedIn={false} />)
    await screen.findByText('House Atreides')
    await userEvent.click(screen.getAllByRole('row')[1])
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).queryByText('guilds.editGuild')).toBeNull()
    expect(within(dialog).queryByRole('button', { name: 'guilds.makeAdmin' })).toBeNull()
    expect(within(dialog).getByText('Noble house')).toBeInTheDocument()
  })

  it('has no axe accessibility violations (table)', async () => {
    const { container } = renderTab(<GuildsTab />)
    await screen.findByText('House Atreides')
    expect(await axe(container)).toHaveNoViolations()
  })

  it('has no axe accessibility violations (open dialog)', async () => {
    renderTab(<GuildsTab />)
    await screen.findByText('House Atreides')
    await userEvent.click(screen.getAllByRole('row')[1])
    const dialog = await screen.findByRole('dialog')
    expect(await axe(dialog)).toHaveNoViolations()
  })
})
