import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { ReactElement } from 'react'
import { render, screen } from '@testing-library/react'
import { axe } from 'vitest-axe'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { DetailedStatus } from './BattlegroupTab/types'
import type { Status, ServerSummary, MarketStats, GuildSummary, UpdateCheckResult } from '../api/client'

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      status: vi.fn(),
      players: { ...actual.api.players, summary: vi.fn() },
      battlegroup: { ...actual.api.battlegroup, status: vi.fn() },
      market: { ...actual.api.market, stats: vi.fn() },
      guilds: { ...actual.api.guilds, list: vi.fn() },
      update: { ...actual.api.update, check: vi.fn() },
    },
  }
})

const { DashboardTab } = await import('./DashboardTab')
const { api } = await import('../api/client')

const STATUS: Status = {
  executor: 'local', control: 'docker', ssh_connected: true, db_connected: true,
  ssh_host: '', db_host: 'localhost', pod_ns: '', pod_ip: '', version: '1.2.3',
}
const SUMMARY: ServerSummary = {
  total_players: 1200, online_players: 42,
  by_map: [], by_faction: [
    { faction: 'Atreides', players: 25, solaris: 0, scrip: 0, avg_level: 30 },
    { faction: 'Harkonnen', players: 17, solaris: 0, scrip: 0, avg_level: 28 },
  ],
  total_solaris: 99999, total_scrip: 0, avg_char_level: 31.4, total_playtime_secs: 3_600_000,
  activity_trend: [
    { day: '2026-06-01', count: 5 }, { day: '2026-06-02', count: 8 }, { day: '2026-06-03', count: 6 },
  ],
  trend_days: 7,
}
const FLEET: DetailedStatus = {
  battlegroup: { name: 'bg1', title: 'Arrakis', phase: 'ready', database: 'ok' },
  servers: [
    {
      map: 'Hagga Basin', sietch: 's1', dimension: 1, partition: 0, phase: 'ready',
      ready: true, players: 30, playerHardCap: 40, queue: 2,
    },
  ],
}
const MARKET: MarketStats = {
  total_listings: 350, bot_listings: 200, player_listings: 150,
  total_stock: 0, bot_stock: 0, player_stock: 0, unique_items: 80,
}
const GUILDS: GuildSummary[] = [
  { guild_id: 1, name: 'A', description: '', faction_id: 1, faction_name: 'Atreides', member_count: 4 },
  { guild_id: 2, name: 'B', description: '', faction_id: 2, faction_name: 'Harkonnen', member_count: 3 },
]
const UPDATE: UpdateCheckResult = { current: '1.2.3', latest: '1.2.3', needs_update: false, release_url: '' }

function renderTab(ui: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
}

describe('DashboardTab', () => {
  beforeEach(() => {
    vi.mocked(api.status).mockResolvedValue(STATUS)
    vi.mocked(api.players.summary).mockResolvedValue(SUMMARY)
    vi.mocked(api.battlegroup.status).mockResolvedValue(FLEET as unknown)
    vi.mocked(api.market.stats).mockResolvedValue(MARKET)
    vi.mocked(api.guilds.list).mockResolvedValue(GUILDS)
    vi.mocked(api.update.check).mockResolvedValue(UPDATE)
  })

  it('renders the overview with a healthy verdict and real stats', async () => {
    renderTab(<DashboardTab isActive={false} />)
    expect(await screen.findByText('Server Dashboard')).toBeInTheDocument()
    expect(await screen.findByText('Operational')).toBeInTheDocument()
    expect(await screen.findAllByText('42')).not.toHaveLength(0) // online players
  })

  it('renders the faction breakdown and the game-server fleet', async () => {
    renderTab(<DashboardTab isActive={false} />)
    expect(await screen.findByText('Hagga Basin')).toBeInTheDocument()
    expect(await screen.findAllByText('Atreides')).not.toHaveLength(0)
    // the players-by-faction ring exposes an accessible label
    expect(screen.getByRole('img', { name: /Players by faction/ })).toBeInTheDocument()
  })

  it('has no axe accessibility violations', async () => {
    const { container } = renderTab(<DashboardTab isActive={false} />)
    await screen.findByText('Server Dashboard')
    expect(await axe(container)).toHaveNoViolations()
  })
})
