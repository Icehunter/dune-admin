import type React from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Chip, toast } from '@heroui/react'
import { Icon, SectionLabel } from '../../../dune-ui'
import { copyText } from '../../../utils/clipboard'
import type { Status } from '../../../api/client'
import type { BGInfo, ServerRow } from '../types'
import { phaseColor, phaseChipColor, bgUptimeSeconds, allServersReady } from '../helpers'
import { formatUptime, portRange } from '../uptime'

// Cards that read both the battlegroup status and the connection status share
// this prop shape.
type HealthProps = { bg?: BGInfo, servers: ServerRow[], status: Status | null }

// ── Card wrapper ──────────────────────────────────────────────────────────────
// HealthCard is the titled panel shell every Server Health card shares: an
// uppercase section label (optionally icon-led) with an optional right-aligned
// accessory, over the card body.
export const HealthCard: React.FC<{
  title: string
  icon?: string
  accessory?: ReactNode
  className?: string
  children: ReactNode
}> = ({ title, icon, accessory, className = '', children }) => (
  <div className={`rounded-[var(--radius)] p-4 flex flex-col gap-3 bg-surface-secondary border border-border dune-lift ${className}`}>
    <div className="flex items-center justify-between gap-2">
      <div className="flex items-center gap-2">
        {icon && <Icon name={icon} className="size-4 text-accent" />}
        <SectionLabel>{title}</SectionLabel>
      </div>
      {accessory}
    </div>
    {children}
  </div>
)

// ── Top status-chip bar ─────────────────────────────────────────────────────
export const HealthChips: React.FC<HealthProps> = ({ bg, servers, status }) => {
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

// ── Battlegroup + VM headline card ───────────────────────────────────────────
export const BgVmCard: React.FC<{ bg?: BGInfo, servers: ServerRow[] }> = ({ bg, servers }) => {
  const { t } = useTranslation()
  const uptime = bgUptimeSeconds(servers)
  return (
    <HealthCard title={t('serverHealth.bgVm')} icon="activity">
      <div className="text-3xl font-semibold" style={{ color: phaseColor(bg?.phase ?? '') }}>
        {bg?.phase || '—'}
      </div>
      <div className="text-sm text-muted">
        {uptime > 0 ? t('serverHealth.upFor', { uptime: formatUptime(uptime) }) : t('serverHealth.noUptime')}
      </div>
    </HealthCard>
  )
}

// ── Component-health rows ─────────────────────────────────────────────────────
const HealthRow: React.FC<{ label: string, value: string, color?: string }> = ({ label, value, color }) => (
  <div className="flex items-center justify-between py-1 border-b border-border/40 last:border-0">
    <span className="text-muted text-sm">{label}</span>
    <span className="font-semibold text-sm" style={color ? { color } : undefined}>{value}</span>
  </div>
)

export const ComponentHealthCard: React.FC<HealthProps> = ({ bg, servers, status }) => {
  const { t } = useTranslation()
  const uptime = bgUptimeSeconds(servers)
  const directorSet = !!status?.director_url
  return (
    <HealthCard title={t('serverHealth.components')} icon="server">
      <div className="flex flex-col">
        <HealthRow label={t('serverHealth.bgState')} value={bg?.phase || '—'} color={phaseColor(bg?.phase ?? '')} />
        <HealthRow label={t('serverHealth.database')} value={bg?.database || '—'} color={phaseColor(bg?.database ?? '')} />
        <HealthRow
          label={t('serverHealth.director')}
          value={directorSet ? t('serverHealth.configured') : t('serverHealth.notConfigured')}
          color={directorSet ? 'var(--success)' : 'var(--muted)'}
        />
        <HealthRow label={t('serverHealth.uptime')} value={formatUptime(uptime)} />
      </div>
    </HealthCard>
  )
}

// ── Game ready state ──────────────────────────────────────────────────────────
export const GameReadyCard: React.FC<{ bg?: BGInfo, servers: ServerRow[] }> = ({ bg, servers }) => {
  const { t } = useTranslation()
  const ready = allServersReady(bg?.phase, servers)
  return (
    <HealthCard title={t('serverHealth.readyState')} icon="heart-pulse">
      <div className="flex items-center gap-2">
        <Icon name={ready ? 'circle-check' : 'circle-x'} className={`size-6 ${ready ? 'text-success' : 'text-muted'}`} />
        <span className="text-2xl font-semibold" style={{ color: ready ? 'var(--success)' : 'var(--muted)' }}>
          {ready ? t('serverHealth.ready') : t('serverHealth.notReady')}
        </span>
      </div>
    </HealthCard>
  )
}

// ── Web interfaces ────────────────────────────────────────────────────────────
const InterfaceRow: React.FC<{ label: string, url: string, href: string }> = ({ label, url, href }) => {
  const { t } = useTranslation()
  const copy = () => {
    copyText(url).then((ok) => (ok ? toast.success(t('serverHealth.copied')) : toast.danger(t('serverHealth.copyFailed'))))
  }
  return (
    <div className="flex items-center gap-2">
      <Icon name="external-link" className="size-4 text-accent" />
      <div className="flex flex-col min-w-0 flex-1">
        <span className="text-sm font-semibold">{label}</span>
        <span className="text-xs text-muted font-mono truncate">{url}</span>
      </div>
      <Button size="sm" variant="ghost" isIconOnly aria-label={t('serverHealth.copy')} onPress={copy}>
        <Icon name="copy" />
      </Button>
      <Button size="sm" variant="outline" onPress={() => window.open(href, '_blank', 'noopener')}>
        {t('serverHealth.open')}
      </Button>
    </div>
  )
}

export const WebInterfacesCard: React.FC<{ status: Status | null }> = ({ status }) => {
  const { t } = useTranslation()
  return (
    <HealthCard title={t('serverHealth.webInterfaces')} icon="layout">
      {status?.director_url
        ? <InterfaceRow label={t('serverHealth.director')} url={status.director_url} href="/director/" />
        : <div className="text-sm text-muted">{t('serverHealth.noInterfaces')}</div>}
    </HealthCard>
  )
}
