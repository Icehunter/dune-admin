import { type ComponentProps } from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { Slot } from 'radix-ui'
import { cn } from '@/lib/utils'

// Re-skinned from the webui shadcn Button onto Dune tokens: brand amber primary
// (--color-primary), Dune subtle-surface hovers (bg-surface-hover instead of
// shadcn's bg-accent, which stays brand amber here), near-square radius (the
// bridged rounded-* scale), a 1px press nudge, and a full-opacity OFFSET focus
// ring so the indicator clears 3:1 on the near-black background.
const buttonVariants = cva(
  'inline-flex shrink-0 items-center justify-center gap-2 rounded-md text-sm font-bold whitespace-nowrap transition-all outline-none active:translate-y-px focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*=\'size-\'])]:size-4',
  {
    variants: {
      variant: {
        default:
          'bg-primary text-primary-foreground shadow-[0_4px_14px_-6px_color-mix(in_oklab,var(--accent)_70%,transparent)] hover:brightness-110',
        destructive: 'bg-destructive text-destructive-foreground hover:brightness-110',
        outline: 'border border-input bg-transparent text-foreground hover:bg-surface-hover',
        secondary: 'bg-secondary text-secondary-foreground hover:bg-surface-hover',
        ghost: 'text-foreground hover:bg-surface-hover',
        link: 'text-primary underline-offset-4 hover:underline',
      },
      size: {
        'default': 'h-9 px-4 py-2 has-[>svg]:px-3',
        'xs': 'h-6 gap-1 rounded-md px-2 text-xs has-[>svg]:px-1.5 [&_svg:not([class*=\'size-\'])]:size-3',
        'sm': 'h-8 gap-1.5 rounded-md px-3 has-[>svg]:px-2.5',
        'lg': 'h-10 rounded-md px-6 has-[>svg]:px-4',
        'icon': 'size-9',
        'icon-sm': 'size-8',
        'icon-lg': 'size-10',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)

type ButtonProps = ComponentProps<'button'>
  & VariantProps<typeof buttonVariants>
  & {
    asChild?: boolean
    /** HeroUI compat: alias for `onClick`, accepted during the migration. */
    onPress?: ComponentProps<'button'>['onClick']
    /** HeroUI compat: alias for `disabled`, accepted during the migration. */
    isDisabled?: boolean
  }

function Button({
  className,
  variant = 'default',
  size = 'default',
  asChild = false,
  onPress,
  isDisabled,
  onClick,
  disabled,
  ...props
}: ButtonProps) {
  const Comp = asChild ? Slot.Root : 'button'

  return (
    <Comp
      data-slot="button"
      data-variant={variant}
      data-size={size}
      className={cn(buttonVariants({ variant, size, className }))}
      onClick={onClick ?? onPress}
      disabled={disabled ?? isDisabled}
      {...props}
    />
  )
}

// buttonVariants (a cva helper, not a component) is co-located per the shadcn
// convention so links/anchors can be styled as buttons; the rule can't tell.
// eslint-disable-next-line react-refresh/only-export-components
export { Button, buttonVariants }
