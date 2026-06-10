import type React from 'react'
import { Icon as IconifyIcon } from '@iconify/react'

type IconProps = {
  /** Lucide icon name (without the `lucide:` prefix), e.g. "refresh-cw". */
  name: string
  /** Optional size class — defaults to `size-4` (1rem square). */
  className?: string
}

// Gravity-UI has crisper / better-weighted variants for these common actions.
const ALIASES: Record<string, string> = {
  'x': 'gravity-ui:xmark',
  'trash': 'gravity-ui:trash-bin',
  'trash-2': 'gravity-ui:trash-bin',
}

/**
 * Thin wrapper around `@iconify/react` that defaults to the lucide icon set
 * and a sensible inline-text size. Use any lucide icon name from
 * https://lucide.dev/icons (kebab-case). A small alias table redirects a few
 * names to gravity-ui equivalents for visual consistency.
 */
export const Icon: React.FC<IconProps> = ({ name, className = 'size-4' }) => (
  <IconifyIcon icon={ALIASES[name] ?? `lucide:${name}`} className={className} />
)
