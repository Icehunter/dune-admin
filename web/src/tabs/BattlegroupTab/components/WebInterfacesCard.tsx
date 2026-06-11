import * as React from 'react'
import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Spinner, toast } from '@heroui/react'
import { Icon, FieldInput } from '../../../dune-ui'
import { copyText } from '../../../utils/clipboard'
import { api } from '../../../api/client'
import type { Status, WebInterface } from '../../../api/client'
import { HealthCard } from './HealthCard'

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
