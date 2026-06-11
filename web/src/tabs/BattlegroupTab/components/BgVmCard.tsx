import * as React from 'react'
import { useTranslation } from 'react-i18next'
import type { BGInfo, ServerRow } from '../types'
import { phaseColor, bgUptimeSeconds } from '../helpers'
import { formatUptime } from '../uptime'
import { HealthCard } from './HealthCard'

export const BgVmCard: React.FC<{ bg?: BGInfo, servers: ServerRow[] }> = ({ bg, servers }) => {
  const { t } = useTranslation()
  const uptime = bgUptimeSeconds(servers)
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
