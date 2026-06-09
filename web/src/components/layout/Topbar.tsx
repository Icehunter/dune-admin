import { useTranslation } from 'react-i18next'
import { Show, SignInButton, UserButton } from '@clerk/react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Icon } from '@/dune-ui/Icon'
import { cn } from '@/lib/utils'
import { LanguageSelector } from '../LanguageSelector'
import type { Status, UpdateCheckResult } from '../../api/client'

const hasClerk = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY

interface TopbarProps {
  title: string
  status: Status | null
  reconnecting: boolean
  onReconnect: () => void
  updateInfo: UpdateCheckResult | null
  onOpenUpdate: () => void
  onOpenSettings: () => void
  onOpenSearch: () => void
  onOpenMobileNav: () => void
}

function StatusDot({ ok }: { ok: boolean }) {
  return <span className={cn('size-2 rounded-full', ok ? 'bg-success' : 'bg-danger')} />
}

/**
 * Sticky top bar — the per-page title plus the controls that used to live in the
 * old header: ⌘K search, connection status + DB reconnect, the update widget,
 * language, settings, and Clerk auth. A hamburger opens the mobile drawer.
 */
export function Topbar({
  title,
  status,
  reconnecting,
  onReconnect,
  updateInfo,
  onOpenUpdate,
  onOpenSettings,
  onOpenSearch,
  onOpenMobileNav,
}: TopbarProps) {
  const { t } = useTranslation()

  return (
    <header className="flex h-14 shrink-0 items-center gap-3 border-b border-border bg-surface px-4 sm:px-5">
      <button
        type="button"
        onClick={onOpenMobileNav}
        aria-label="Open navigation"
        className="grid size-9 place-items-center rounded-md text-muted outline-none transition-colors hover:bg-surface-hover hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring md:hidden"
      >
        <Icon name="menu" className="size-5" />
      </button>

      <h1 className="truncate text-base font-bold tracking-tight text-foreground">{title}</h1>

      <div className="ml-auto flex items-center gap-2">
        <button
          type="button"
          onClick={onOpenSearch}
          aria-keyshortcuts="Control+K Meta+K"
          className="hidden items-center gap-2 rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-muted outline-none transition-colors hover:bg-surface-hover hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring sm:flex"
        >
          <Icon name="search" className="size-3.5" />
          <span>Search</span>
          <kbd className="rounded border border-border px-1 font-mono text-[10px]">⌘K</kbd>
        </button>
        <button
          type="button"
          onClick={onOpenSearch}
          aria-label="Search"
          className="grid size-9 place-items-center rounded-md text-muted outline-none transition-colors hover:bg-surface-hover hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring sm:hidden"
        >
          <Icon name="search" className="size-4" />
        </button>

        <div className="hidden items-center gap-3 border-l border-border/60 pl-3 md:flex">
          {status?.executor === 'ssh' && (
            <span className="flex items-center gap-1.5 text-xs text-muted">
              <StatusDot ok={status.ssh_connected} />
              SSH
            </span>
          )}
          <span className="flex items-center gap-1.5 text-xs text-muted">
            <StatusDot ok={status?.db_connected ?? false} />
            DB
          </span>
          {status && !status.db_connected && (
            <Button size="sm" variant="outline" disabled={reconnecting} onClick={onReconnect}>
              {reconnecting ? t('app.reconnecting') : t('app.reconnect')}
            </Button>
          )}
        </div>

        {updateInfo?.needs_update && (
          <button
            type="button"
            onClick={onOpenUpdate}
            aria-label={t('app.updateAvailable')}
            className="rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Badge tone="warning">
              <Icon name="arrow-up" className="size-3" />
              {updateInfo.latest.replace(/^v/, '')}
            </Badge>
          </button>
        )}

        <LanguageSelector />

        <button
          type="button"
          onClick={onOpenSettings}
          aria-label={t('app.configureBackend')}
          title={status?.version ? `v${status.version}` : undefined}
          className="grid size-9 place-items-center rounded-md text-muted outline-none transition-colors hover:bg-surface-hover hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Icon name="settings" className="size-4" />
        </button>

        {hasClerk && (
          <>
            <Show when="signed-out">
              <SignInButton>
                <Button size="sm" variant="outline">{t('app.signIn')}</Button>
              </SignInButton>
            </Show>
            <Show when="signed-in">
              <UserButton />
            </Show>
          </>
        )}
      </div>
    </header>
  )
}
