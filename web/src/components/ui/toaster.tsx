import { Toaster as SonnerToaster } from 'sonner'

/**
 * Dune-themed sonner toaster. Mounted once near the app root; renders the toasts
 * produced by the `toast` facade. Sonner provides the accessible aria-live region
 * automatically. Styled with Dune tokens (amber/dark) via classNames overrides.
 */
export function Toaster() {
  return (
    <SonnerToaster
      theme="dark"
      position="top-right"
      toastOptions={{
        classNames: {
          toast: 'rounded-[var(--radius)] border border-border bg-surface text-foreground',
          title: 'text-foreground',
          description: 'text-muted',
          actionButton: 'bg-primary text-primary-foreground',
          cancelButton: 'bg-secondary text-secondary-foreground',
          success: '!border-success/40',
          error: '!border-danger/40',
          warning: '!border-warning/40',
          info: '!border-accent-soft',
        },
      }}
    />
  )
}
