import * as React from 'react'
import { Chip } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import type { BotStatusCardProps } from './interfaces'
import { Sep } from './Sep'
import { Stat } from './Stat'

const fmt = (ts: string | null | undefined): string => {
  if (!ts) return '—'
  try {
    return new Date(ts).toLocaleTimeString()
  }
  catch {
    return ts
  }
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
        <React.Fragment>
          <Sep />
          <Stat label={t('market.bot.status.nextListTick')}>{fmt(status.next_list_tick)}</Stat>
        </React.Fragment>
      )}
      {status.next_buy_tick != null && (
        <React.Fragment>
          <Sep />
          <Stat label={t('market.bot.status.nextBuyTick')}>{fmt(status.next_buy_tick)}</Stat>
        </React.Fragment>
      )}
    </div>
  )
}
