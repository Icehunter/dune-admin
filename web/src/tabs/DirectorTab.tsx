import { useState, useEffect, useCallback } from 'react'
import type React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Spinner, toast } from '@heroui/react'
import { api, ApiError } from '../api/client'
import type { DirectorConfig } from '../api/client'
import { PageHeader, Panel, SectionLabel, Icon } from '../dune-ui'

// DirectorTab (#147): view/edit the Battlegroup Director config
// (director_config.ini). [InstancingModes] controls map persistence; [Database]
// and [RMQ*] are read-only (launch-overridden + secrets). AMP control plane only.
export const DirectorTab: React.FC = () => {
  const { t } = useTranslation()
  const [data, setData] = useState<DirectorConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [unsupported, setUnsupported] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pending, setPending] = useState<Map<string, string>>(new Map())

  const load = useCallback(() => {
    Promise.resolve()
      .then(() => {
        setLoading(true)
        setError(null)
        setUnsupported(false)
      })
      .then(() => api.director.get())
      .then((d) => {
        setData(d)
        setPending(new Map())
      })
      .catch((e: unknown) => {
        if (e instanceof ApiError && e.status === 501) setUnsupported(true)
        else setError(e instanceof Error ? e.message : String(e))
      })
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const pk = (section: string, key: string) => `${section}|${key}`
  const setVal = (section: string, key: string, value: string) =>
    setPending((prev) => {
      const n = new Map(prev)
      n.set(pk(section, key), value)
      return n
    })

  const save = () => {
    if (pending.size === 0) return
    const updates: Record<string, Record<string, string>> = {}
    for (const [k, v] of pending) {
      const [section, key] = k.split('|')
      if (!updates[section]) updates[section] = {}
      updates[section][key] = v
    }
    setSaving(true)
    api.director.update(updates)
      .then((res) => {
        toast.success(res.ok)
        load()
      })
      .catch((e: unknown) =>
        toast.danger(t('director.saveFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setSaving(false))
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full gap-2 text-muted">
        <Spinner size="sm" color="current" />
        <span className="text-sm">{t('director.loading')}</span>
      </div>
    )
  }

  if (unsupported) {
    return (
      <div className="flex flex-col h-full gap-3">
        <PageHeader title={t('director.title')} />
        <div className="text-sm text-muted py-8 text-center">{t('director.unsupported')}</div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex flex-col h-full gap-3">
        <PageHeader title={t('director.title')} />
        <div className="rounded px-4 py-3 text-sm bg-danger/10 border border-danger/40 text-danger">{error}</div>
      </div>
    )
  }

  const dirty = pending.size

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <PageHeader title={t('director.title')} subtitle={t('director.subtitle')}>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="ghost" onPress={load} isDisabled={loading || saving}>
            <Icon name="refresh-cw" />
          </Button>
          <Button size="sm" onPress={save} isDisabled={dirty === 0 || saving}>
            {saving
              ? <Spinner size="sm" color="current" />
              : dirty > 0 ? t('director.saveWithCount', { count: dirty }) : t('director.save')}
          </Button>
        </div>
      </PageHeader>

      <p className="text-xs text-warning shrink-0">{t('director.restartNote')}</p>
      {data?.path && <p className="text-xs text-muted shrink-0 font-mono truncate">{data.path}</p>}

      <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 pb-6 pr-1">
        {data?.sections.map((sec) => (
          <Panel key={sec.name}>
            <div className="flex items-center gap-2 mb-2">
              <SectionLabel>{sec.name}</SectionLabel>
              {sec.read_only && (
                <span className="text-xs text-muted border border-border rounded px-1.5 py-0.5">
                  {t('director.readOnly')}
                </span>
              )}
            </div>
            <div className="flex flex-col gap-1.5">
              {sec.lines.map((line) => {
                const editable = !sec.read_only && !line.secret
                const cur = pending.get(pk(sec.name, line.key)) ?? line.value
                return (
                  <div
                    key={line.key}
                    className="grid grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)] items-center gap-3 text-sm"
                  >
                    <div className="min-w-0">
                      <div className="text-foreground truncate" title={line.key}>{line.key}</div>
                      {line.comment && <div className="text-xs text-muted truncate" title={line.comment}>{line.comment}</div>}
                    </div>
                    {editable
                      ? (
                          <input
                            className="w-full bg-surface text-foreground border border-border rounded px-2 py-1 text-sm"
                            value={cur}
                            onChange={(e) => setVal(sec.name, line.key, e.target.value)}
                          />
                        )
                      : <span className="text-muted font-mono truncate">{line.secret ? '••••••••' : line.value}</span>}
                  </div>
                )
              })}
            </div>
          </Panel>
        ))}
      </div>
    </div>
  )
}
