import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Label } from './label'

describe('Label', () => {
  it('renders its text', () => {
    render(<Label htmlFor="x">Guild name</Label>)
    expect(screen.getByText('Guild name')).toBeInTheDocument()
  })

  it('appends a decorative, aria-hidden required marker', () => {
    render(<Label htmlFor="x" required>Guild name</Label>)
    const marker = screen.getByText('*')
    expect(marker).toHaveAttribute('aria-hidden', 'true')
  })
})
