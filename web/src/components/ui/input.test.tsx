import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { axe } from 'vitest-axe'
import { Input, Textarea } from './input'
import { Label } from './label'

describe('Input', () => {
  it('is a labelled textbox that accepts typing', async () => {
    render(
      <>
        <Label htmlFor="name">Name</Label>
        <Input id="name" />
      </>,
    )
    const input = screen.getByLabelText('Name')
    await userEvent.type(input, 'Atreides')
    expect(input).toHaveValue('Atreides')
  })

  it('reflects aria-invalid', () => {
    render(<Input aria-invalid aria-label="Broken" />)
    expect(screen.getByLabelText('Broken')).toHaveAttribute('aria-invalid', 'true')
  })

  it('has no axe accessibility violations', async () => {
    const { container } = render(
      <>
        <Label htmlFor="x">X</Label>
        <Input id="x" />
      </>,
    )
    expect(await axe(container)).toHaveNoViolations()
  })
})

describe('Textarea', () => {
  it('is a labelled multiline textbox that accepts typing', async () => {
    render(
      <>
        <Label htmlFor="desc">Description</Label>
        <Textarea id="desc" />
      </>,
    )
    const ta = screen.getByLabelText('Description')
    await userEvent.type(ta, 'A noble house')
    expect(ta).toHaveValue('A noble house')
  })

  it('has no axe accessibility violations', async () => {
    const { container } = render(
      <>
        <Label htmlFor="d">D</Label>
        <Textarea id="d" />
      </>,
    )
    expect(await axe(container)).toHaveNoViolations()
  })
})
