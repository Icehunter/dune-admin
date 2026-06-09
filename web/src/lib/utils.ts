import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/**
 * Merge class names with Tailwind conflict resolution: clsx handles
 * conditional/falsy values, tailwind-merge dedupes conflicting utilities so the
 * last one wins (e.g. `cn('px-2', cond && 'px-4')`). Used by every primitive in
 * `src/components/ui/*` to compose cva-generated classes with caller overrides.
 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}
