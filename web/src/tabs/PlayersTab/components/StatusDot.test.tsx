import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { axe } from 'vitest-axe'
import { StatusDot } from './StatusDot'

// First component test — proves the harness end-to-end: Testing Library render,
// jsdom, jest-dom matchers, and vitest-axe accessibility assertions.
describe('StatusDot', () => {
  it('uses the success colour when Online', () => {
    const { container } = render(<StatusDot status="Online" />)
    const dot = container.querySelector('span')
    expect(dot).toBeInTheDocument()
    expect(dot).toHaveClass('bg-success')
  })

  it('uses the warning colour while logging out', () => {
    const { container } = render(<StatusDot status="LoggingOut" />)
    expect(container.querySelector('span')).toHaveClass('bg-warning')
  })

  it('falls back to muted for any other status', () => {
    const { container } = render(<StatusDot status="Offline" />)
    expect(container.querySelector('span')).toHaveClass('bg-muted')
  })

  it('has no axe accessibility violations', async () => {
    const { container } = render(<StatusDot status="Online" />)
    expect(await axe(container)).toHaveNoViolations()
  })
})
