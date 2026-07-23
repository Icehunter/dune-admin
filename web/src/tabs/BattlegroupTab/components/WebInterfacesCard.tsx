import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Checkbox, Skeleton, Spinner, toast } from '@heroui/react'
import { Icon, FieldInput } from '../../../dune-ui'
import { api } from '../../../api/client'
import type { Status, WebInterface } from '../../../api/client'
import { HealthCard } from './HealthCard'
import { InterfaceRow } from './InterfaceRow'
import { DirectorRow } from './DirectorRow'

export const WebInterfacesCard: React.FC<{ status: Status | null }> = ({ status }) => {
  const { t } = useTranslation()
  const [items, setItems] = React.useState<WebInterface[]>([])
  const [discovered, setDiscovered] = React.useState<WebInterface[]>([])
  const [draft, setDraft] = React.useState<WebInterface[]>([])
  const [editing, setEditing] = React.useState(false)
  const [loading, setLoading] = React.useState(true)
  const [saving, setSaving] = React.useState(false)
  const director = status?.director_url

  const load = (): void => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.webInterfaces.get())
      .then((res) => {
        setItems(res.interfaces ?? [])
        setDiscovered(res.discovered ?? [])
      })
      .catch((e: unknown) =>
        toast.danger(t('serverHealth.ifaceLoadFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }

  React.useEffect(() => {
    load()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const startEdit = () => {
    setDraft(items.length ? items.map((i) => ({ ...i })) : [{ label: '', url: '' }])
    setEditing(true)
  }
  const setField = (i: number, key: 'label' | 'url', v: string) =>
    setDraft((d) => d.map((row, idx) => (idx === i ? { ...row, [key]: v } : row)))
  const setNoProxy = (i: number, v: boolean) =>
    setDraft((d) => d.map((row, idx) => (idx === i ? { ...row, noProxy: v } : row)))

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
      {loading && (
        <div className="flex flex-col gap-3">
          {Array.from({ length: 2 }, (_, i) => (
            // Mirror a loaded row exactly (no inner gap; label text-sm=20px,
            // url text-xs=16px) so the skeleton → row swap doesn't shift.
            <div key={i} className="flex items-center gap-2">
              <Skeleton className="size-4 rounded" />
              <div className="flex flex-col min-w-0 flex-1">
                <Skeleton className="h-5 w-1/3 rounded-lg" />
                <Skeleton className="h-4 w-2/3 rounded-lg" />
              </div>
            </div>
          ))}
        </div>
      )}

      {!loading && director && <DirectorRow directorURL={director} />}

      {/* Control-plane-discovered links (e.g. kubectl director / file browser):
          read-only, never editable or persisted. */}
      {!loading && discovered.map((item) => (
        <InterfaceRow key={`discovered:${item.url}`} item={item} />
      ))}

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
              {/* Opt this entry out of the mesh web proxy — the SPA opens the
                  URL above as-is instead of a rewritten proxy port. For
                  NAT/reverse-proxy setups where only fixed published ports
                  are reachable, the rewritten URL is unreachable. (#261) */}
              <Checkbox
                isSelected={row.noProxy ?? false}
                onChange={(v) => setNoProxy(i, v)}
                className="shrink-0"
              >
                <span className="text-xs text-muted whitespace-nowrap">{t('serverHealth.ifaceNoProxy')}</span>
              </Checkbox>
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
