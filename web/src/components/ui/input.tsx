import { type ComponentProps } from 'react'
import { cn } from '@/lib/utils'

// Shared field surface: recessed page-bg fill, brighter --color-input border,
// accent focus ring, and an aria-invalid red state so a control reddens when its
// Field reports an error. Near-square to match the Dune aesthetic.
const fieldClass = cn(
  'w-full rounded-sm border border-input bg-background px-3 text-sm text-foreground transition-colors outline-none',
  'placeholder:text-muted',
  'focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset',
  'disabled:cursor-not-allowed disabled:opacity-50',
  'aria-invalid:border-danger aria-invalid:ring-2 aria-invalid:ring-danger/30',
)

export function Input({ className, type = 'text', ...props }: ComponentProps<'input'>) {
  return <input type={type} data-slot="input" className={cn(fieldClass, 'h-9 py-1.5', className)} {...props} />
}

export function Textarea({ className, ...props }: ComponentProps<'textarea'>) {
  return <textarea data-slot="textarea" className={cn(fieldClass, 'min-h-16 py-2', className)} {...props} />
}
