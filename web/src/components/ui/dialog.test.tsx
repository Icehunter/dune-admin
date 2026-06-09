import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { axe } from 'vitest-axe'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogClose } from './dialog'

function Example({ onOpenChange }: { onOpenChange?: (v: boolean) => void }) {
  return (
    <Dialog defaultOpen onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit guild</DialogTitle>
          <DialogDescription>Change the guild details.</DialogDescription>
        </DialogHeader>
        <DialogClose>Done</DialogClose>
      </DialogContent>
    </Dialog>
  )
}

describe('Dialog', () => {
  it('renders an accessible dialog titled by its DialogTitle when open', () => {
    render(<Example />)
    const dialog = screen.getByRole('dialog')
    expect(dialog).toBeInTheDocument()
    expect(dialog).toHaveAccessibleName('Edit guild')
  })

  it('closes when a close control is activated', async () => {
    const onOpenChange = vi.fn()
    render(<Example onOpenChange={onOpenChange} />)
    await userEvent.click(screen.getByRole('button', { name: 'Done' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('closes on Escape (Radix focus management)', async () => {
    const onOpenChange = vi.fn()
    render(<Example onOpenChange={onOpenChange} />)
    await userEvent.keyboard('{Escape}')
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('has no axe accessibility violations', async () => {
    render(<Example />)
    expect(await axe(screen.getByRole('dialog'))).toHaveNoViolations()
  })
})
