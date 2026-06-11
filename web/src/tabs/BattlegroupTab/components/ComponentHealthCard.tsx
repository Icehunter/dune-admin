import * as React from 'react'
import { useTranslation } from 'react-i18next'
import type { Status } from '../../../api/client'
import type { BGInfo, ServerRow } from '../types'
import { phaseColor, bgUptimeSeconds } from '../helpers'
import { formatUptime } from '../uptime'
import { HealthCard } from './HealthCard'

type ComponentHealthCardProps = { bg?: BGInfo, servers: ServerRow[], status: Status | null }

export const ComponentHealthCard: React.FC<ComponentHealthCardProps> = ({ bg, servers, status }) => {
  const { t } = useTranslation()
  const uptime = bgUptimeSeconds(servers)
  const directorSet = !!status?.director_url
  return (
    <HealthCard title={t('serverHealth.components')} icon="server">
      <div className="flex flex-col divide-y divide-border/30">
        <div className="flex items-center justify-between py-1.5">
          <span className="text-sm text-muted">{t('serverHealth.bgState')}</span>
          <span className="text-sm font-semibold" style={{ color: phaseColor(bg?.phase ?? '') }}>
            {bg?.phase || '—'}
          </span>
        </div>
        <div className="flex items-center justify-between py-1.5">
          <span className="text-sm text-muted">{t('serverHealth.database')}</span>
          <span className="text-sm font-semibold" style={{ color: phaseColor(bg?.database ?? '') }}>
            {bg?.database || '—'}
          </span>
        </div>
        <div className="flex items-center justify-between py-1.5">
          <span className="text-sm text-muted">{t('serverHealth.director')}</span>
          <span
            className="text-sm font-semibold"
            style={{ color: directorSet ? 'var(--success)' : 'var(--muted)' }}
          >
            {directorSet ? t('serverHealth.configured') : t('serverHealth.notConfigured')}
          </span>
        </div>
        <div className="flex items-center justify-between py-1.5">
          <span className="text-sm text-muted">{t('serverHealth.uptime')}</span>
          <span className="text-sm font-semibold text-foreground">{formatUptime(uptime)}</span>
        </div>
      </div>
    </HealthCard>
  )
}
