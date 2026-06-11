import type React from 'react'
import { Chip } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import type { BotStatus } from '../../../api/client'

function fmt(ts: string | null | undefined): string {
  if (!ts) return '—'
  try {
    return new Date(ts).toLocaleTimeString()
  }
  catch {
    return ts
  }
}

interface BotStatusCardProps {
  status: BotStatus
}

export const BotStatusCard: React.FC<BotStatusCardProps> = ({ status }) => {
  const { t } = useTranslation()
  const statusLabel = status.running ? t('market.bot.status.running') : t('market.bot.status.paused')
  const statusColor = status.running ? 'success' : 'warning'

  return (
    <div className="flex items-center flex-wrap gap-0">
      <Stat label={t('market.bot.status.label')} first>
        <Chip size="sm" color={statusColor} variant="soft">{statusLabel}</Chip>
      </Stat>
      <Sep />
      <Stat label={t('market.bot.status.uptime')}>{status.uptime || '—'}</Stat>
      <Sep />
      <Stat label={t('market.bot.status.listings')}>{status.listing_count?.toLocaleString() ?? '—'}</Stat>
      <Sep />
      <Stat label={t('market.bot.status.balance')}>{status.balance?.toLocaleString() ?? '—'}</Stat>
      <Sep />
      <Stat label={t('market.bot.status.errors')} danger={status.error_count > 0}>
        {String(status.error_count ?? 0)}
      </Stat>
      <Sep />
      <Stat label={t('market.bot.status.lastListTick')}>{fmt(status.last_list_tick)}</Stat>
      <Sep />
      <Stat label={t('market.bot.status.lastBuyTick')}>{fmt(status.last_buy_tick)}</Stat>
      {status.next_list_tick != null && (
        <>
          <Sep />
          <Stat label={t('market.bot.status.nextListTick')}>{fmt(status.next_list_tick)}</Stat>
        </>
      )}
      {status.next_buy_tick != null && (
        <>
          <Sep />
          <Stat label={t('market.bot.status.nextBuyTick')}>{fmt(status.next_buy_tick)}</Stat>
        </>
      )}
    </div>
  )
}

function Sep() {
  return <div className="w-px h-8 bg-border mx-3 shrink-0" />
}

interface StatProps {
  label: string
  first?: boolean
  danger?: boolean
  children: React.ReactNode
}

function Stat({ label, danger, children }: StatProps) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[10px] font-semibold uppercase tracking-wider text-muted">{label}</span>
      <span className={`text-sm font-mono ${danger ? 'text-danger' : 'text-foreground'}`}>{children}</span>
    </div>
  )
}
