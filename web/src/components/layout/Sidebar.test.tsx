import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { axe } from 'vitest-axe'
import { Sidebar } from './Sidebar'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const baseProps = {
  currentTab: 'dashboard',
  dbSection: 'tables',
  welcomeSection: 'config',
  onNavigate: vi.fn(),
  onSubNavigate: vi.fn(),
}

describe('Sidebar', () => {
  it('renders the dashboard link and the three management groups', () => {
    render(<Sidebar {...baseProps} />)
    expect(screen.getByRole('button', { name: 'Dashboard' })).toBeInTheDocument()
    expect(screen.getByText('Server Management')).toBeInTheDocument()
    expect(screen.getByText('Player Management')).toBeInTheDocument()
    expect(screen.getByText('Economy Management')).toBeInTheDocument()
  })

  it('marks the current tab active and navigates on click', async () => {
    const onNavigate = vi.fn()
    render(<Sidebar {...baseProps} currentTab="battlegroup" onNavigate={onNavigate} />)
    expect(screen.getByRole('button', { name: 'nav.battlegroup' })).toHaveAttribute('aria-current', 'page')
    await userEvent.click(screen.getByRole('button', { name: 'nav.players' }))
    expect(onNavigate).toHaveBeenCalledWith('players')
  })

  it('expands sub-sections for the active section item and sub-navigates', async () => {
    const onSubNavigate = vi.fn()
    render(<Sidebar {...baseProps} currentTab="database" onSubNavigate={onSubNavigate} />)
    await userEvent.click(screen.getByRole('button', { name: 'database.sections.sql' }))
    expect(onSubNavigate).toHaveBeenCalledWith('database', 'sql')
  })

  it('hides labels and sub-sections when collapsed', () => {
    render(<Sidebar {...baseProps} currentTab="database" collapsed />)
    // group headings and sub-items are hidden in the icon rail
    expect(screen.queryByText('Server Management')).toBeNull()
    expect(screen.queryByRole('button', { name: 'database.sections.sql' })).toBeNull()
  })

  it('keeps the Icehunter attribution in the footer', () => {
    render(<Sidebar {...baseProps} />)
    const link = screen.getByRole('link', { name: /Icehunter/ })
    expect(link).toHaveAttribute('href', 'https://github.com/Icehunter/dune-admin')
  })

  it('has no axe accessibility violations', async () => {
    const { container } = render(<Sidebar {...baseProps} />)
    expect(await axe(container)).toHaveNoViolations()
  })
})
