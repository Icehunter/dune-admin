import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CommandPalette } from './CommandPalette'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

describe('CommandPalette', () => {
  it('lists nav sections and navigates on select', async () => {
    const onNavigate = vi.fn()
    const onOpenChange = vi.fn()
    render(<CommandPalette open onOpenChange={onOpenChange} onNavigate={onNavigate} />)
    expect(screen.getByPlaceholderText('Jump to a section…')).toBeInTheDocument()
    await userEvent.click(screen.getByText('nav.market'))
    expect(onNavigate).toHaveBeenCalledWith('market')
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('renders nothing when closed', () => {
    render(<CommandPalette open={false} onOpenChange={vi.fn()} onNavigate={vi.fn()} />)
    expect(screen.queryByPlaceholderText('Jump to a section…')).toBeNull()
  })
})
