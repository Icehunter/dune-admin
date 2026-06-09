import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { axe } from 'vitest-axe'
import { Spinner } from './spinner'

describe('Spinner', () => {
  it('exposes a status role with an accessible label', () => {
    render(<Spinner />)
    expect(screen.getByRole('status', { name: 'Loading' })).toBeInTheDocument()
  })

  it('accepts a custom accessible label', () => {
    render(<Spinner label="Fetching bases" />)
    expect(screen.getByRole('status', { name: 'Fetching bases' })).toBeInTheDocument()
  })

  it('renders a spinning glyph', () => {
    const { container } = render(<Spinner />)
    expect(container.querySelector('svg.animate-spin')).not.toBeNull()
  })

  it('inherits the surrounding text colour when color="current"', () => {
    const { container } = render(<Spinner color="current" />)
    expect(container.querySelector('svg')?.getAttribute('class')).toContain('text-current')
  })

  it('uses brand amber by default', () => {
    const { container } = render(<Spinner />)
    expect(container.querySelector('svg')?.getAttribute('class')).toContain('text-accent-brand')
  })

  it('has no axe accessibility violations', async () => {
    const { container } = render(<Spinner />)
    expect(await axe(container)).toHaveNoViolations()
  })
})
