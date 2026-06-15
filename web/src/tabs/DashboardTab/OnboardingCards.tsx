import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, CloseButton } from '@heroui/react'
import { Icon, Panel } from '../../dune-ui'

export interface OnboardingCardsProps {
  hasServers: boolean
  serversLoading: boolean
  /** Can manage servers (add/manage) — gates the admin actions; false for guests. */
  canControl: boolean
  /** Can manage auth — gates the "set up auth" card. */
  canManageAuth: boolean
  /** Auth already enabled — hide the auth card. */
  authEnabled: boolean
  /** Discord bot already configured — hide the Discord card. */
  discordEnabled: boolean
  onAddServer: () => void
  onOpenSettings: (tab?: string) => void
}

const DISMISS_PREFIX = 'dune_dashboard_dismiss_'

// Whether a help card was dismissed on THIS browser (per-PC, localStorage).
const isDismissed = (id: string): boolean => {
  try {
    return localStorage.getItem(DISMISS_PREFIX + id) === '1'
  }
  catch {
    return false
  }
}

interface HelpCardProps {
  icon: string
  title: string
  body: string
  cta: string
  onAction: () => void
  onDismiss: () => void
}

const HelpCard: React.FC<HelpCardProps> = ({ icon, title, body, cta, onAction, onDismiss }) => {
  const { t } = useTranslation()
  return (
    <Panel className="flex flex-col gap-2">
      <div className="flex items-start justify-between gap-2">
        <div className="flex items-center gap-2">
          <Icon name={icon} className="text-accent" />
          <span className="text-sm font-semibold text-foreground">{title}</span>
        </div>
        <CloseButton aria-label={t('common.dismiss', 'Dismiss')} className="size-5 opacity-60 hover:opacity-100" onPress={onDismiss} />
      </div>
      <p className="text-xs text-muted">{body}</p>
      <div className="mt-1">
        <Button size="sm" variant="outline" onPress={onAction}>{cta}</Button>
      </div>
    </Panel>
  )
}

export const OnboardingCards: React.FC<OnboardingCardsProps> = ({
  hasServers, serversLoading, canControl, canManageAuth, authEnabled, discordEnabled, onAddServer, onOpenSettings,
}) => {
  const { t } = useTranslation()
  // Re-render to drop a card when dismissed; we read localStorage on each render.
  const [, force] = React.useReducer((n: number) => n + 1, 0)
  const dismiss = (id: string) => {
    try {
      localStorage.setItem(DISMISS_PREFIX + id, '1')
    }
    catch { /* ignore */ }
    force()
  }

  // Setup cards are admin-only (hidden from guests), hidden once the feature is
  // enabled, and dismissible per-browser.
  interface Card {
    id: string
    icon: string
    title: string
    body: string
    cta: string
    show: boolean
    onAction: () => void
  }
  const cards: Card[] = [
    {
      id: 'auth',
      icon: 'lock',
      title: t('dashboard.onboarding.auth.title', 'Set up login / auth'),
      body: t('dashboard.onboarding.auth.body', 'Require a login to access the dashboard. Optional — leave off for a trusted local network.'),
      cta: t('dashboard.onboarding.auth.cta', 'Set up auth'),
      show: canManageAuth && !authEnabled,
      onAction: () => onOpenSettings('auth'),
    },
    {
      id: 'discord',
      icon: 'message-circle',
      title: t('dashboard.onboarding.discord.title', 'Connect a Discord bot'),
      body: t('dashboard.onboarding.discord.body', 'Send notifications and let roles map to permissions. Optional.'),
      cta: t('dashboard.onboarding.discord.cta', 'Connect Discord'),
      show: canControl && !discordEnabled,
      onAction: () => onOpenSettings('discord'),
    },
  ]
  const visible = cards.filter((c) => c.show && !isDismissed(c.id))

  // Don't render the empty state until the server list has actually loaded — a
  // fresh load (e.g. right after enabling auth) must not flash "No servers yet".
  const showEmpty = !hasServers && !serversLoading

  return (
    <div className="flex flex-col gap-3">
      {showEmpty && (
        <Panel className="flex flex-col items-center gap-3 py-10 text-center">
          <Icon name="server" className="size-8 text-muted" />
          <div className="flex flex-col gap-1">
            <span className="text-base font-semibold text-foreground">{t('dashboard.empty.title', 'No servers yet')}</span>
            <span className="text-sm text-muted">
              {canControl
                ? t('dashboard.empty.body', 'Add your first game server to get started. You can also set up auth or Discord below — all optional.')
                : t('dashboard.empty.guestBody', 'No servers have been configured yet.')}
            </span>
          </div>
          {canControl && (
            <Button size="sm" onPress={onAddServer}>
              <Icon name="plus" />
              {' '}
              {t('dashboard.empty.addServer', 'Add your first server')}
            </Button>
          )}
        </Panel>
      )}

      {visible.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {visible.map((c) => (
            <HelpCard
              key={c.id}
              icon={c.icon}
              title={c.title}
              body={c.body}
              cta={c.cta}
              onAction={c.onAction}
              onDismiss={() => dismiss(c.id)}
            />
          ))}
        </div>
      )}
    </div>
  )
}
