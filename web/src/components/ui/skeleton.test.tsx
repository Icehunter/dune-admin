import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { axe } from 'vitest-axe'
import { Skeleton } from './skeleton'

describe('Skeleton', () => {
  it('renders a pulsing placeholder block', () => {
    const { container } = render(<Skeleton className="h-4 w-20" />)
    const el = container.querySelector('[data-slot="skeleton"]')
    expect(el).not.toBeNull()
    expect(el).toHaveClass('animate-pulse')
    expect(el).toHaveClass('h-4', 'w-20')
  })

  it('has no axe accessibility violations', async () => {
    const { container } = render(<Skeleton className="h-4 w-20" />)
    expect(await axe(container)).toHaveNoViolations()
  })
})
