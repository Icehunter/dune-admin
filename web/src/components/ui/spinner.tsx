import { type ComponentProps } from 'react'
import { cn } from '@/lib/utils'

const sizeClass = {
  sm: 'size-4',
  md: 'size-5',
  lg: 'size-6',
} as const

type SpinnerProps = Omit<ComponentProps<'span'>, 'color'> & {
  size?: keyof typeof sizeClass
  /** HeroUI compat: 'current' inherits the surrounding text colour. */
  color?: 'current' | 'brand'
  /** Accessible label announced by screen readers. */
  label?: string
}

/**
 * Indeterminate loading spinner — a self-contained spinning SVG (no async icon
 * load, so it renders deterministically in tests) wrapped in a `role="status"`
 * region with an sr-only label so assistive tech announces the busy state.
 * Drop-in for the HeroUI `<Spinner size color />` calls used across the app.
 */
function Spinner({ size = 'md', color = 'brand', label = 'Loading', className, ...props }: SpinnerProps) {
  return (
    <span role="status" aria-label={label} data-slot="spinner" className={cn('inline-flex', className)} {...props}>
      <svg
        aria-hidden="true"
        viewBox="0 0 24 24"
        fill="none"
        className={cn('animate-spin', sizeClass[size], color === 'current' ? 'text-current' : 'text-accent-brand')}
      >
        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" />
        <path className="opacity-90" fill="currentColor" d="M12 2a10 10 0 0 1 10 10h-3a7 7 0 0 0-7-7V2z" />
      </svg>
    </span>
  )
}

export { Spinner }
