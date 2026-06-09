import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { axe } from 'vitest-axe'
import { Button, buttonVariants } from './button'

describe('Button', () => {
  it('renders its children as a native button by default', () => {
    render(<Button>Save</Button>)
    const button = screen.getByRole('button', { name: 'Save' })
    expect(button.tagName).toBe('BUTTON')
    expect(button).toHaveAttribute('data-slot', 'button')
  })

  it('exposes the chosen variant and size via data attributes', () => {
    render(<Button variant="destructive" size="lg">Delete</Button>)
    const button = screen.getByRole('button', { name: 'Delete' })
    expect(button).toHaveAttribute('data-variant', 'destructive')
    expect(button).toHaveAttribute('data-size', 'lg')
  })

  it('produces distinct classes for each variant', () => {
    const variants = ['default', 'destructive', 'outline', 'secondary', 'ghost', 'link'] as const
    const classes = variants.map((variant) => buttonVariants({ variant }))
    expect(new Set(classes).size).toBe(variants.length)
  })

  it('composes with a child element when asChild is set', () => {
    render(
      <Button asChild>
        <a href="/bases">Bases</a>
      </Button>,
    )
    const link = screen.getByRole('link', { name: 'Bases' })
    expect(link).toHaveAttribute('href', '/bases')
    expect(link).toHaveAttribute('data-slot', 'button')
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('carries a full-opacity, offset focus ring so the indicator meets 3:1', () => {
    render(<Button variant="ghost">Focus me</Button>)
    const cls = screen.getByRole('button', { name: 'Focus me' }).className
    expect(cls).toContain('focus-visible:ring-ring')
    expect(cls).toContain('focus-visible:ring-offset-2')
    expect(cls).toContain('focus-visible:ring-offset-background')
    expect(cls).not.toContain('focus-visible:ring-ring/50')
  })

  it('maps the HeroUI onPress prop onto a native click during the migration', async () => {
    const onPress = vi.fn()
    render(<Button onPress={onPress}>Press</Button>)
    await userEvent.click(screen.getByRole('button', { name: 'Press' }))
    expect(onPress).toHaveBeenCalledOnce()
  })

  it('maps the HeroUI isDisabled prop onto the native disabled state', () => {
    render(<Button isDisabled>Off</Button>)
    expect(screen.getByRole('button', { name: 'Off' })).toBeDisabled()
  })

  it('has no axe accessibility violations', async () => {
    const { container } = render(<Button>Accessible</Button>)
    expect(await axe(container)).toHaveNoViolations()
  })
})
