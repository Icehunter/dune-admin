import { type ComponentProps } from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

// Tinted status pill — replaces the HeroUI <Chip variant="soft" color> usage.
// Each tone carries text + colour (never colour-only) so meaning is conveyed by
// the label, satisfying the never-colour-only a11y rule. Near-square to match
// the Dune aesthetic.
const badgeVariants = cva(
  'inline-flex items-center gap-1 rounded-sm border px-2 py-0.5 text-xs font-medium whitespace-nowrap',
  {
    variants: {
      tone: {
        default: 'bg-secondary text-muted border-border',
        accent: 'bg-primary/10 text-accent-brand border-primary/30',
        success: 'bg-success/10 text-success border-success/30',
        warning: 'bg-warning/10 text-warning border-warning/30',
        danger: 'bg-danger/10 text-danger border-danger/30',
      },
    },
    defaultVariants: { tone: 'default' },
  },
)

type BadgeProps = ComponentProps<'span'> & VariantProps<typeof badgeVariants>

function Badge({ className, tone, ...props }: BadgeProps) {
  return <span data-slot="badge" data-tone={tone ?? 'default'} className={cn(badgeVariants({ tone, className }))} {...props} />
}

export { Badge }
