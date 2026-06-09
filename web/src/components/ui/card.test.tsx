import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { axe } from 'vitest-axe'
import { Card, CardHeader, CardTitle, CardContent } from './card'

describe('Card', () => {
  it('renders the compound structure with content', () => {
    const { container } = render(
      <Card>
        <CardHeader>
          <CardTitle>Feature unavailable</CardTitle>
        </CardHeader>
        <CardContent>Not supported on this control plane.</CardContent>
      </Card>,
    )
    expect(container.querySelector('[data-slot="card"]')).not.toBeNull()
    expect(container.querySelector('[data-slot="card-header"]')).not.toBeNull()
    expect(screen.getByText('Feature unavailable')).toBeInTheDocument()
    expect(screen.getByText('Not supported on this control plane.')).toBeInTheDocument()
  })

  it('merges caller class names onto the card', () => {
    const { container } = render(<Card className="max-w-sm">x</Card>)
    expect(container.querySelector('[data-slot="card"]')).toHaveClass('max-w-sm')
  })

  it('has no axe accessibility violations', async () => {
    const { container } = render(
      <Card>
        <CardHeader>
          <CardTitle>Title</CardTitle>
        </CardHeader>
        <CardContent>Body</CardContent>
      </Card>,
    )
    expect(await axe(container)).toHaveNoViolations()
  })
})
