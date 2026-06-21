export interface DashboardTabProps {
  onAddServer: () => void
  onOpenSettings: (tab?: string) => void
  onManageServer: (id: number) => void
  /** Bumped when the settings modal closes, so onboarding state re-syncs. */
  refreshKey?: number
}

export interface HelpCardProps {
  icon: string
  title: string
  body: string
  cta: string
  onAction: () => void
  onDismiss: () => void
}

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

export interface OnboardingCard {
  id: string
  icon: string
  title: string
  body: string
  cta: string
  show: boolean
  onAction: () => void
}
