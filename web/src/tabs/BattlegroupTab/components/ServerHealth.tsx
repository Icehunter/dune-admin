import type React from 'react'
import type { ReactNode } from 'react'
import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Chip, Spinner, toast } from '@heroui/react'
import { Icon, SectionLabel, FieldInput } from '../../../dune-ui'
import { copyText } from '../../../utils/clipboard'
import { api } from '../../../api/client'
import type { Status, WebInterface } from '../../../api/client'
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

// ── Component-health rows ─────────────────────────────────────────────────────
// HealthRow → ItemCard with label as title and colored value as action Chip
export const ComponentHealthCard: React.FC<HealthProps> = ({ bg, servers, status }) => {
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

// ── Game ready state ──────────────────────────────────────────────────────────
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

// ── Web interfaces (#155: operator-configurable list) ────────────────────────
const InterfaceRow: React.FC<{ item: WebInterface }> = ({ item }) => {
  const { t } = useTranslation()
  const copy = () => {
    copyText(item.url).then((ok) =>
      (ok ? toast.success(t('serverHealth.copied')) : toast.danger(t('serverHealth.copyFailed'))))
  }
  return (
    <div className="flex items-center gap-2">
      <Icon name="external-link" className="size-4 text-accent" />
      <div className="flex flex-col min-w-0 flex-1">
        <span className="text-sm font-semibold">{item.label}</span>
        <span className="text-xs text-muted font-mono truncate">{item.url}</span>
      </div>
      <Button size="sm" variant="ghost" isIconOnly aria-label={t('serverHealth.copy')} onPress={copy}>
        <Icon name="copy" />
      </Button>
      <Button size="sm" variant="outline" onPress={() => window.open(item.url, '_blank', 'noopener')}>
        {t('serverHealth.open')}
      </Button>
    </div>
  )
}

// DirectorRow is the automatic, read-only entry shown when director_url is set:
// the Director usually binds to loopback on the host, so "Open" goes through the
// same-origin /director/ reverse proxy. The configured target is shown for context.
const DirectorRow: React.FC<{ directorURL: string }> = ({ directorURL }) => {
  const { t } = useTranslation()
  const copy = () => {
    copyText(`${window.location.origin}/director/`).then((ok) =>
      (ok ? toast.success(t('serverHealth.copied')) : toast.danger(t('serverHealth.copyFailed'))))
  }
  return (
    <div className="flex items-center gap-2">
      <Icon name="external-link" className="size-4 text-accent" />
      <div className="flex flex-col min-w-0 flex-1">
        <span className="text-sm font-semibold">
          {t('serverHealth.director')}
          {' '}
          <span className="text-xs font-normal text-muted">{t('serverHealth.directorProxied')}</span>
        </span>
        <span className="text-xs text-muted font-mono truncate">{directorURL}</span>
      </div>
      <Button size="sm" variant="ghost" isIconOnly aria-label={t('serverHealth.copy')} onPress={copy}>
        <Icon name="copy" />
      </Button>
      <Button size="sm" variant="outline" onPress={() => window.open('/director/', '_blank', 'noopener')}>
        {t('serverHealth.open')}
      </Button>
    </div>
  )
}

export const WebInterfacesCard: React.FC<{ status: Status | null }> = ({ status }) => {
  const { t } = useTranslation()
  const [items, setItems] = useState<WebInterface[]>([])
  const [draft, setDraft] = useState<WebInterface[]>([])
  const [editing, setEditing] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const director = status?.director_url

  const load = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.webInterfaces.get())
      .then((res) => setItems(res.interfaces ?? []))
      .catch((e: unknown) =>
        toast.danger(t('serverHealth.ifaceLoadFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t])

  useEffect(() => {
    load()
  }, [load])

  const startEdit = () => {
    setDraft(items.length ? items.map((i) => ({ ...i })) : [{ label: '', url: '' }])
    setEditing(true)
  }
  const setField = (i: number, key: 'label' | 'url', v: string) =>
    setDraft((d) => d.map((row, idx) => (idx === i ? { ...row, [key]: v } : row)))

  const save = () => {
    const clean = draft.filter((r) => r.label.trim() && r.url.trim())
    setSaving(true)
    api.webInterfaces.update(clean)
      .then((res) => {
        toast.success(res.ok)
        setEditing(false)
        load()
      })
      .catch((e: unknown) =>
        toast.danger(t('serverHealth.ifaceSaveFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setSaving(false))
  }

  const editBtn = (
    <Button size="sm" variant="ghost" isIconOnly aria-label={t('serverHealth.editInterfaces')} onPress={startEdit}>
      <Icon name="pencil" />
    </Button>
  )

  return (
    <HealthCard title={t('serverHealth.webInterfaces')} icon="layout" accessory={!editing && !loading ? editBtn : undefined}>
      {loading && <div className="py-2 flex justify-center"><Spinner size="sm" color="current" /></div>}

      {!loading && director && <DirectorRow directorURL={director} />}

      {!loading && editing && (
        <div className="flex flex-col gap-2">
          {draft.map((row, i) => (
            <div key={i} className="flex items-center gap-2">
              <FieldInput
                value={row.label}
                placeholder={t('serverHealth.ifaceLabel')}
                onChange={(v) => setField(i, 'label', v)}
                ariaLabel={t('serverHealth.ifaceLabel')}
                className="w-32"
              />
              <FieldInput
                value={row.url}
                placeholder={t('serverHealth.ifaceUrl')}
                onChange={(v) => setField(i, 'url', v)}
                ariaLabel={t('serverHealth.ifaceUrl')}
                className="flex-1 font-mono"
              />
              <Button
                size="sm"
                variant="ghost"
                isIconOnly
                aria-label={t('serverHealth.removeInterface')}
                onPress={() => setDraft((d) => d.filter((_, idx) => idx !== i))}
              >
                <Icon name="trash-2" />
              </Button>
            </div>
          ))}
          <div className="flex items-center gap-2">
            <Button size="sm" variant="outline" onPress={() => setDraft((d) => [...d, { label: '', url: '' }])}>
              <Icon name="plus" />
              {' '}
              {t('serverHealth.addInterface')}
            </Button>
            <div className="flex-1" />
            <Button size="sm" variant="ghost" onPress={() => setEditing(false)}>{t('common.cancel')}</Button>
            <Button size="sm" onPress={save} isDisabled={saving}>
              {saving ? <Spinner size="sm" color="current" /> : t('serverHealth.saveInterfaces')}
            </Button>
          </div>
        </div>
      )}

      {!loading && !editing && !director && items.length === 0 && (
        <div className="flex items-center justify-between gap-2">
          <span className="text-sm text-muted">{t('serverHealth.noInterfaces')}</span>
          <Button size="sm" variant="outline" onPress={startEdit}>
            <Icon name="plus" />
            {' '}
            {t('serverHealth.addInterface')}
          </Button>
        </div>
      )}

      {!loading && !editing && items.length > 0 && (
        <div className="flex flex-col gap-2">
          {items.map((it) => <InterfaceRow key={`${it.label}|${it.url}`} item={it} />)}
        </div>
      )}
    </HealthCard>
  )
}
