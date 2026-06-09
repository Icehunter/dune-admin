import { type ComponentProps } from 'react'
import { cn } from '@/lib/utils'

/**
 * Pulsing placeholder block for loading states. Re-skinned from the webui shadcn
 * Skeleton: uses bg-secondary (Dune subtle surface) rather than bg-accent, which
 * is brand amber in this theme.
 */
function Skeleton({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div
      data-slot="skeleton"
      className={cn('animate-pulse rounded-md bg-secondary', className)}
      {...props}
    />
  )
}

export { Skeleton }
