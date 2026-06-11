import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../../../dune-ui'
import type { BGInfo, ServerRow } from '../types'
import { allServersReady } from '../helpers'
import { HealthCard } from './HealthCard'

export const GameReadyCard: React.FC<{ bg?: BGInfo, servers: ServerRow[] }> = ({ bg, servers }) => {
  const { t } = useTranslation()
  const ready = allServersReady(bg?.phase, servers)
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
