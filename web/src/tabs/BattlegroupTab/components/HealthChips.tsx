import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Chip } from '@heroui/react'
import { Icon } from '../../../dune-ui'
import type { Status } from '../../../api/client'
import type { BGInfo, ServerRow } from '../types'
import { phaseChipColor } from '../helpers'
import { portRange } from '../uptime'

type HealthChipsProps = { bg?: BGInfo, servers: ServerRow[], status: Status | null }

export const HealthChips: React.FC<HealthChipsProps> = ({ bg, servers, status }) => {
  const { t } = useTranslation()
  const ports = portRange(servers.map((s) => s.port ?? 0))
  // listen_addr is like ":9090" or "0.0.0.0:9090" — show just the port.
  const webPort = (status?.listen_addr ?? '').split(':').pop() || '—'
  return (
    <div className="flex flex-wrap items-center gap-2 shrink-0">
      <Chip size="sm" variant="soft" color="default">
        <Icon name="network" className="size-3" />
        {' '}
        {t('serverHealth.gamePorts')}
        {': '}
        {ports}
      </Chip>
      <Chip size="sm" variant="soft" color="default">
        <Icon name="globe" className="size-3" />
        {' '}
        {t('serverHealth.webPort')}
        {': '}
        {webPort}
      </Chip>
      <div className="flex-1" />
      <Chip size="sm" variant="soft" color={phaseChipColor(status?.control && status.control !== 'none' ? 'running' : 'stopped')}>
        {t('serverHealth.vm')}
        {' · '}
        {status?.control && status.control !== 'none' ? t('serverHealth.up') : t('serverHealth.down')}
      </Chip>
      <Chip size="sm" variant="soft" color={phaseChipColor(bg?.phase ?? '')}>
        {t('serverHealth.bg')}
        {' · '}
        {bg?.phase || '—'}
      </Chip>
    </div>
  )
}
