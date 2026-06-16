import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Skeleton } from '@heroui/react'
import { Icon } from '../../../dune-ui'
import type { BGInfo, ServerRow } from '../types'
import { allServersReady } from '../helpers'
import { HealthCard } from './HealthCard'

type GameReadyCardProps = { bg?: BGInfo, servers: ServerRow[], loading?: boolean }

export const GameReadyCard: React.FC<GameReadyCardProps> = ({ bg, servers, loading }) => {
  const { t } = useTranslation()
  const ready = allServersReady(bg?.phase, servers)
  if (loading) {
    return (
      <HealthCard title={t('serverHealth.readyState')} icon="circle-check">
        {/* size-6 matches the Icon; h-8 matches the text-2xl line-box (32px). */}
        <div className="flex items-center gap-2">
          <Skeleton className="size-6 rounded-full" />
          <Skeleton className="h-8 w-28 rounded-lg" />
        </div>
      </HealthCard>
    )
  }
  return (
    <HealthCard title={t('serverHealth.readyState')} icon={ready ? 'circle-check' : 'circle-x'}>
      <div
        className="flex items-center gap-2"
        style={{ color: ready ? 'var(--success)' : 'var(--muted)' }}
      >
        <Icon name={ready ? 'circle-check' : 'circle-x'} className="size-6" />
        <span className="text-2xl font-semibold">
          {ready ? t('serverHealth.ready') : t('serverHealth.notReady')}
        </span>
      </div>
    </HealthCard>
  )
}
