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

function fmtBalance(n: number | undefined): string {
  if (n == null) return '—'
  return n.toLocaleString()
}

export default function BotStatusCard({ status }: { status: BotStatus }) {
  const { t } = useTranslation()
  const statusLabel = status.running ? t('market.bot.status.running') : t('market.bot.status.paused')
  const statusColor = status.running ? 'success' : 'warning'

  return (
    <div className="flex flex-wrap gap-4 items-start">
      <div className="flex flex-col gap-1 min-w-[120px]">
        <span className="text-xs text-muted uppercase tracking-wider">{t('market.bot.status.label')}</span>
        <Chip
          size="sm"
          color={statusColor}
          variant="soft"
        >
          {statusLabel}
        </Chip>
      </div>

      <Stat label={t('market.bot.status.uptime')} value={status.uptime || '—'} />
      <Stat label={t('market.bot.status.listings')} value={status.listing_count?.toLocaleString() ?? '—'} />
      <Stat label={t('market.bot.status.balance')} value={fmtBalance(status.balance)} />
      <Stat label={t('market.bot.status.errors')} value={String(status.error_count ?? 0)} accent={status.error_count > 0 ? 'danger' : undefined} />
      <Stat label={t('market.bot.status.lastListTick')} value={fmt(status.last_list_tick)} />
      <Stat label={t('market.bot.status.lastBuyTick')} value={fmt(status.last_buy_tick)} />
      {status.next_list_tick != null && <Stat label={t('market.bot.status.nextListTick')} value={fmt(status.next_list_tick)} />}
      {status.next_buy_tick != null && <Stat label={t('market.bot.status.nextBuyTick')} value={fmt(status.next_buy_tick)} />}
    </div>
  )
}

function Stat({ label, value, accent }: { label: string, value: string, accent?: 'danger' }) {
  return (
    <div className="flex flex-col gap-1 min-w-[100px]">
      <span className="text-xs text-muted uppercase tracking-wider">{label}</span>
      <span className={`text-sm font-mono ${accent === 'danger' ? 'text-danger' : 'text-foreground'}`}>{value}</span>
    </div>
  )
}
