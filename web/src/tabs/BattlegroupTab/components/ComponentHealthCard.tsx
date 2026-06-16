import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Skeleton } from '@heroui/react'
import type { Status } from '../../../api/client'
import type { BGInfo, ServerRow } from '../types'
import { phaseColor, bgUptimeSeconds } from '../helpers'
import { formatUptime } from '../uptime'
import { HealthCard } from './HealthCard'

type ComponentHealthCardProps = { bg?: BGInfo, servers: ServerRow[], status: Status | null, loading?: boolean }

export const ComponentHealthCard: React.FC<ComponentHealthCardProps> = ({ bg, servers, status, loading }) => {
  const { t } = useTranslation()
  const uptime = bgUptimeSeconds(servers)
  // The Director row reflects the optional director-proxy config (director_url).
  // Showing a permanent "Not configured" row when no proxy is set is just noise
  // (#203 — reporter on a Funcom VM with no director_url), so only show the row
  // when a director proxy is actually configured, regardless of control plane.
  const showDirector = !!status?.director_url
  return (
    <HealthCard title={t('serverHealth.components')} icon="server">
      <div className="flex flex-col divide-y divide-border/30">
        <div className="flex items-center justify-between py-1.5">
          <span className="text-sm text-muted">{t('serverHealth.bgState')}</span>
          {loading
            ? <Skeleton className="h-3.5 w-20 rounded-lg" />
            : (
                <span className="text-sm font-semibold" style={{ color: phaseColor(bg?.phase ?? '') }}>
                  {bg?.phase || '—'}
                </span>
              )}
        </div>
        <div className="flex items-center justify-between py-1.5">
          <span className="text-sm text-muted">{t('serverHealth.database')}</span>
          {loading
            ? <Skeleton className="h-3.5 w-24 rounded-lg" />
            : (
                <span className="text-sm font-semibold" style={{ color: phaseColor(bg?.database ?? '') }}>
                  {bg?.database || '—'}
                </span>
              )}
        </div>
        {showDirector && (
          <div className="flex items-center justify-between py-1.5">
            <span className="text-sm text-muted">{t('serverHealth.director')}</span>
            <span className="text-sm font-semibold" style={{ color: 'var(--success)' }}>
              {t('serverHealth.configured')}
            </span>
          </div>
        )}
        <div className="flex items-center justify-between py-1.5">
          <span className="text-sm text-muted">{t('serverHealth.uptime')}</span>
          {loading
            ? <Skeleton className="h-3.5 w-16 rounded-lg" />
            : <span className="text-sm font-semibold text-foreground">{formatUptime(uptime)}</span>}
        </div>
      </div>
    </HealthCard>
  )
}
