import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { axe } from 'vitest-axe'
import { Badge } from './badge'

describe('Badge', () => {
  it('renders its label', () => {
    render(<Badge>Online</Badge>)
    expect(screen.getByText('Online')).toBeInTheDocument()
  })

  it('exposes the chosen tone via a data attribute', () => {
    render(<Badge tone="danger">Disabled</Badge>)
    expect(screen.getByText('Disabled')).toHaveAttribute('data-tone', 'danger')
  })

  it('defaults to the neutral tone', () => {
    render(<Badge>Open</Badge>)
    expect(screen.getByText('Open')).toHaveAttribute('data-tone', 'default')
  })

  it('has no axe accessibility violations', async () => {
    const { container } = render(<Badge tone="success">Done</Badge>)
    expect(await axe(container)).toHaveNoViolations()
  })
})
