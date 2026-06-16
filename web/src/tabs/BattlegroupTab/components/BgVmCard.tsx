import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Skeleton } from '@heroui/react'
import type { BGInfo, ServerRow } from '../types'
import { phaseColor, bgUptimeSeconds } from '../helpers'
import { formatUptime } from '../uptime'
import { HealthCard } from './HealthCard'

type BgVmCardProps = { bg?: BGInfo, servers: ServerRow[], loading?: boolean }

export const BgVmCard: React.FC<BgVmCardProps> = ({ bg, servers, loading }) => {
  const { t } = useTranslation()
  const uptime = bgUptimeSeconds(servers)
  if (loading) {
    return (
      <HealthCard title={t('serverHealth.bgVm')} icon="activity">
        {/* Heights match the loaded text line-boxes (text-3xl=36px, text-sm=20px)
            so the skeleton → value swap causes no layout shift. */}
        <Skeleton className="h-9 w-32 rounded-lg" />
        <Skeleton className="h-5 w-40 rounded-lg" />
      </HealthCard>
    )
  }
  return (
    <HealthCard title={t('serverHealth.bgVm')} icon="activity">
      <span
        className="text-3xl font-semibold"
        style={{ color: phaseColor(bg?.phase ?? '') }}
      >
        {bg?.phase || '—'}
      </span>
      <span className="text-sm text-muted">
        {uptime > 0 ? t('serverHealth.upFor', { uptime: formatUptime(uptime) }) : t('serverHealth.noUptime')}
      </span>
    </HealthCard>
  )
}
