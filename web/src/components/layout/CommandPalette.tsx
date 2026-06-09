import { Command } from 'cmdk'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/dune-ui/Icon'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import { NAV_DASHBOARD, NAV_GROUPS, type NavItem } from './nav.config'

interface CommandPaletteProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onNavigate: (tab: string) => void
}

/**
 * ⌘K command palette — fuzzy jump-to-section, sourced from the same nav config
 * as the sidebar. Built on the Radix Dialog primitive + cmdk (which owns the
 * filtering and keyboard selection).
 */
export function CommandPalette({ open, onOpenChange, onNavigate }: CommandPaletteProps) {
  const { t } = useTranslation()
  // Widen the strictly-typed t() for the dynamic nav keys from config.
  const tr = t as unknown as (key: string) => string
  const label = (item: NavItem) => (item.labelKey ? tr(item.labelKey) : item.literal ?? item.tab)

  const renderItem = (item: NavItem) => (
    <Command.Item
      key={item.tab}
      value={label(item)}
      onSelect={() => {
        onNavigate(item.tab)
        onOpenChange(false)
      }}
      className="flex cursor-pointer items-center gap-2.5 rounded-md px-2 py-2 text-sm text-foreground data-[selected=true]:bg-surface-hover data-[selected=true]:text-accent-brand"
    >
      <Icon name={item.icon} className="size-4 shrink-0 text-muted" />
      {label(item)}
    </Command.Item>
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="overflow-hidden p-0 sm:max-w-[560px]" showCloseButton={false} aria-describedby={undefined}>
        <DialogTitle className="sr-only">Command menu</DialogTitle>
        <Command
          className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pt-2 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:font-bold [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-[0.08em] [&_[cmdk-group-heading]]:text-muted/70"
        >
          <div className="flex items-center gap-2 border-b border-border px-3">
            <Icon name="search" className="size-4 shrink-0 text-muted" />
            <Command.Input
              placeholder="Jump to a section…"
              className="h-11 w-full bg-transparent text-sm text-foreground outline-none placeholder:text-muted"
            />
          </div>
          <Command.List className="max-h-80 overflow-y-auto p-2">
            <Command.Empty className="py-6 text-center text-sm text-muted">No results.</Command.Empty>
            <Command.Group heading="Overview">
              {renderItem(NAV_DASHBOARD)}
            </Command.Group>
            {NAV_GROUPS.map((g) => (
              <Command.Group key={g.title} heading={g.title}>
                {g.items.map(renderItem)}
              </Command.Group>
            ))}
          </Command.List>
        </Command>
      </DialogContent>
    </Dialog>
  )
}
