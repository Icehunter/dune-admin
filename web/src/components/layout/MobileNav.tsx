import { Dialog as DialogPrimitive } from 'radix-ui'
import { cn } from '@/lib/utils'
import { Sidebar } from './Sidebar'

interface MobileNavProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentTab: string
  dbSection: string
  welcomeSection: string
  onNavigate: (tab: string) => void
  onSubNavigate: (section: 'database' | 'welcome', key: string) => void
}

/**
 * Mobile navigation drawer — a left-side Radix Dialog (focus trap, Esc, scroll
 * lock for free) that hosts the full Sidebar, always expanded. Navigating closes
 * the drawer. Hidden on md+ where the persistent sidebar is shown instead.
 */
export function MobileNav({
  open,
  onOpenChange,
  currentTab,
  dbSection,
  welcomeSection,
  onNavigate,
  onSubNavigate,
}: MobileNavProps) {
  const navigate = (tab: string) => {
    onNavigate(tab)
    onOpenChange(false)
  }
  const subNavigate = (section: 'database' | 'welcome', key: string) => {
    onSubNavigate(section, key)
    onOpenChange(false)
  }

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="fixed inset-0 z-[9990] bg-black/70 backdrop-blur-sm data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 md:hidden" />
        <DialogPrimitive.Content
          className={cn(
            'fixed inset-y-0 left-0 z-[9999] flex h-full w-60 flex-col border-r border-border shadow-lg outline-none md:hidden',
            'data-[state=open]:animate-in data-[state=open]:slide-in-from-left',
            'data-[state=closed]:animate-out data-[state=closed]:slide-out-to-left',
          )}
        >
          <DialogPrimitive.Title className="sr-only">Navigation</DialogPrimitive.Title>
          <Sidebar
            currentTab={currentTab}
            dbSection={dbSection}
            welcomeSection={welcomeSection}
            onNavigate={navigate}
            onSubNavigate={subNavigate}
            hideCollapse
          />
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}
