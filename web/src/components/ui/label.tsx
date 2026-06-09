import { type ComponentProps } from 'react'
import { cn } from '@/lib/utils'

/**
 * Form label — associates with its control via `htmlFor`. `required` appends a
 * decorative marker (aria-hidden); the control itself carries `required` /
 * `aria-required`, so colour/marker is never the only signal.
 */
export function Label({
  className,
  required = false,
  children,
  ...props
}: ComponentProps<'label'> & { required?: boolean }) {
  return (
    <label
      data-slot="label"
      className={cn('flex items-center gap-1 text-xs font-medium text-muted select-none', className)}
      {...props}
    >
      {children}
      {required && (
        <span aria-hidden="true" className="font-bold text-danger" title="Required">*</span>
      )}
    </label>
  )
}
