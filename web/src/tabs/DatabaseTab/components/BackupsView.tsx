import { useState, useEffect, useCallback } from 'react'
import type React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Spinner, toast } from '@heroui/react'
import { api } from '../../../api/client'
import type { DBBackupFile, ScheduledBackups, BackupRule } from '../../../api/client'
import { Panel, SectionLabel, PageHeader, Icon, ConfirmDialog } from '../../../dune-ui'

const DOW = [0, 1, 2, 3, 4, 5, 6] // Sun..Sat

function fmtSize(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`
  return `${(b / 1024 / 1024 / 1024).toFixed(1)} GB`
}

const inputCls = 'bg-surface text-foreground border border-border rounded px-2 py-1 text-sm'

// ── Backup schedule card (self-contained, mirrors ScheduledRestartsCard) ──────
const ScheduleCard: React.FC = () => {
  const { t, i18n } = useTranslation()
  const [data, setData] = useState<ScheduledBackups | null>(null)
  const [enabled, setEnabled] = useState(false)
  const [timezone, setTimezone] = useState('')
  const [keepN, setKeepN] = useState(0)
  const [rules, setRules] = useState<BackupRule[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  const apply = (d: ScheduledBackups) => {
    setData(d)
    setEnabled(d.enabled)
    setTimezone(d.timezone)
    setKeepN(d.keep_n || 0)
    setRules(d.rules ?? [])
  }

  const load = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.scheduledBackups.get())
      .then(apply)
      .catch((e: unknown) =>
        toast.danger(t('backups.loadFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t])

  useEffect(() => {
    load()
  }, [load])

  const save = () => {
    setSaving(true)
    api.scheduledBackups.update({ enabled, timezone, rules, keep_n: keepN })
      .then((res) => {
        toast.success(res.ok)
        load()
      })
      .catch((e: unknown) =>
        toast.danger(t('backups.schedule.saveFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setSaving(false))
  }

  const addRule = () => setRules((r) => [...r, { days: [...DOW], time: '04:00' }])
  const removeRule = (i: number) => setRules((r) => r.filter((_, idx) => idx !== i))
  const setRuleTime = (i: number, time: string) =>
    setRules((r) => r.map((rule, idx) => (idx === i ? { ...rule, time } : rule)))
  const toggleDay = (i: number, d: number) =>
    setRules((r) => r.map((rule, idx) => {
      if (idx !== i) return rule
      const days = rule.days.includes(d) ? rule.days.filter((x) => x !== d) : [...rule.days, d].sort((a, b) => a - b)
      return { ...rule, days }
    }))

  const dowLabel = (d: number) =>
    new Intl.DateTimeFormat(i18n.language, { weekday: 'short' }).format(new Date(Date.UTC(2023, 0, 1 + d)))

  return (
    <Panel>
      <div className="flex items-center justify-between mb-1">
        <SectionLabel>{t('backups.schedule.title')}</SectionLabel>
        <label className="flex items-center gap-2 text-xs text-muted cursor-pointer">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          {t('backups.schedule.enable')}
        </label>
      </div>
      <p className="text-xs text-muted mb-2">{t('backups.schedule.desc')}</p>

      {loading
        ? <div className="py-3 flex justify-center"><Spinner size="sm" color="current" /></div>
        : (
            <>
              <div className="text-sm mb-2">
                {enabled && data?.next_backup
                  ? (
                      <span className="text-success">
                        {t('backups.schedule.nextBackup', { when: new Date(data.next_backup).toLocaleString() })}
                      </span>
                    )
                  : <span className="text-muted">{t('backups.schedule.noneScheduled')}</span>}
              </div>

              {rules.length === 0 && <div className="text-xs text-muted mb-2">{t('backups.schedule.noRules')}</div>}
              {rules.map((rule, i) => (
                <div key={i} className="flex items-center gap-2 mb-2 flex-wrap">
                  <div className="flex gap-1">
                    {DOW.map((d) => (
                      <button
                        key={d}
                        type="button"
                        onClick={() => toggleDay(i, d)}
                        className={`h-7 px-1.5 rounded text-xs transition-colors ${
                          rule.days.includes(d)
                            ? 'bg-accent text-accent-foreground'
                            : 'bg-surface-secondary text-muted hover:text-foreground'
                        }`}
                      >
                        {dowLabel(d)}
                      </button>
                    ))}
                  </div>
                  <input
                    type="time"
                    value={rule.time}
                    onChange={(e) => setRuleTime(i, e.target.value)}
                    className={inputCls}
                  />
                  <Button
                    size="sm"
                    variant="ghost"
                    isIconOnly
                    aria-label={t('backups.schedule.removeRule')}
                    onPress={() => removeRule(i)}
                  >
                    <Icon name="x" />
                  </Button>
                </div>
              ))}

              <Button size="sm" variant="outline" className="mb-3" onPress={addRule}>
                <Icon name="plus" />
                {' '}
                {t('backups.schedule.addRule')}
              </Button>

              <div className="flex items-center gap-4 mb-3 text-sm flex-wrap">
                <label className="flex items-center gap-2">
                  {t('backups.schedule.keepN')}
                  <input
                    type="number"
                    min={0}
                    value={keepN}
                    onChange={(e) => setKeepN(Number(e.target.value) || 0)}
                    className={`${inputCls} w-20`}
                  />
                  <span className="text-xs text-muted">{t('backups.schedule.keepHint')}</span>
                </label>
                <label className="flex items-center gap-2 flex-1 min-w-[160px]">
                  {t('backups.schedule.timezone')}
                  <input
                    value={timezone}
                    placeholder={t('backups.schedule.tzPlaceholder')}
                    onChange={(e) => setTimezone(e.target.value)}
                    className={`${inputCls} flex-1`}
                  />
                </label>
              </div>

              <Button size="sm" onPress={save} isDisabled={saving}>
                {saving ? <Spinner size="sm" color="current" /> : t('backups.schedule.save')}
              </Button>
            </>
          )}
    </Panel>
  )
}

// ── Backups view ─────────────────────────────────────────────────────────────
export const BackupsView: React.FC = () => {
  const { t } = useTranslation()
  const [backups, setBackups] = useState<DBBackupFile[]>([])
  const [loading, setLoading] = useState(true)
  const [taking, setTaking] = useState(false)
  const [restoreTarget, setRestoreTarget] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.dbBackups.list())
      .then((res) => setBackups(res.backups ?? []))
      .catch((e: unknown) =>
        toast.danger(t('backups.loadFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t])

  useEffect(() => {
    load()
  }, [load])

  const take = () => {
    setTaking(true)
    api.dbBackups.take()
      .then((res) => {
        toast.success(t('backups.taken', { name: res.name }))
        load()
      })
      .catch((e: unknown) =>
        toast.danger(t('backups.takeFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setTaking(false))
  }

  const doRestore = () => {
    if (!restoreTarget) return
    const file = restoreTarget
    setRestoreTarget(null)
    setBusy(true)
    api.dbBackups.restore(file)
      .then((res) => toast.success(res.ok))
      .catch((e: unknown) =>
        toast.danger(t('backups.restoreFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setBusy(false))
  }

  const doDelete = () => {
    if (!deleteTarget) return
    const file = deleteTarget
    setDeleteTarget(null)
    setBusy(true)
    api.dbBackups.remove(file)
      .then((res) => {
        toast.success(res.ok)
        load()
      })
      .catch((e: unknown) =>
        toast.danger(t('backups.deleteFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setBusy(false))
  }

  return (
    <div className="h-full min-h-0 flex flex-col gap-3">
      <PageHeader title={t('database.sections.backups')} onRefresh={load} loading={loading} />

      <div className="rounded-[var(--radius)] px-3 py-2 text-sm flex items-start gap-2 bg-warning/10 text-warning border border-warning/40 shrink-0">
        <Icon name="triangle-alert" className="size-4 mt-0.5 shrink-0" />
        <span>{t('backups.warning')}</span>
      </div>

      <div className="flex-1 min-h-0 overflow-auto flex flex-col gap-3 pr-1">
        {/* Take Backup */}
        <Panel>
          <SectionLabel>{t('backups.take.title')}</SectionLabel>
          <p className="text-xs text-muted">{t('backups.take.desc')}</p>
          <div>
            <Button size="sm" onPress={take} isDisabled={taking}>
              {taking
                ? <Spinner size="sm" color="current" />
                : (
                    <>
                      <Icon name="database-backup" />
                      {' '}
                      {t('backups.take.btn')}
                    </>
                  )}
            </Button>
          </div>
        </Panel>

        <ScheduleCard />

        {/* Recent backups */}
        <Panel>
          <SectionLabel>{t('backups.recent.title')}</SectionLabel>
          {loading
            ? <div className="py-3 flex justify-center"><Spinner size="sm" color="current" /></div>
            : backups.length === 0
              ? <div className="text-sm text-muted py-2">{t('backups.recent.empty')}</div>
              : (
                  <div className="flex flex-col gap-1">
                    <div className="grid grid-cols-[1fr_auto_auto_auto] gap-3 px-2 text-xs uppercase tracking-wide text-muted">
                      <span>{t('backups.col.name')}</span>
                      <span className="text-right">{t('backups.col.size')}</span>
                      <span>{t('backups.col.modified')}</span>
                      <span />
                    </div>
                    {backups.map((b) => (
                      <div
                        key={b.name}
                        className="grid grid-cols-[1fr_auto_auto_auto] gap-3 items-center px-2 py-1.5 rounded bg-surface border border-border/40"
                      >
                        <span className="font-mono text-sm truncate" title={b.name}>{b.name}</span>
                        <span className="text-sm text-muted text-right tabular-nums">{fmtSize(b.size_bytes)}</span>
                        <span className="text-sm text-muted">{new Date(b.modified).toLocaleString()}</span>
                        <div className="flex items-center gap-1">
                          <a href={api.dbBackups.downloadUrl(b.name)} download>
                            <Button size="sm" variant="ghost" isIconOnly aria-label={t('backups.download')}>
                              <Icon name="download" />
                            </Button>
                          </a>
                          <Button
                            size="sm"
                            variant="outline"
                            isDisabled={busy}
                            onPress={() => setRestoreTarget(b.name)}
                          >
                            {t('backups.restoreLabel')}
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            isIconOnly
                            aria-label={t('backups.deleteLabel')}
                            isDisabled={busy}
                            onPress={() => setDeleteTarget(b.name)}
                          >
                            <Icon name="trash-2" />
                          </Button>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
        </Panel>
      </div>

      <ConfirmDialog
        open={restoreTarget !== null}
        title={t('backups.restoreConfirmTitle')}
        description={t('backups.restoreConfirmDesc', { name: restoreTarget ?? '' })}
        confirmLabel={t('backups.restoreLabel')}
        onConfirm={doRestore}
        onCancel={() => setRestoreTarget(null)}
      />
      <ConfirmDialog
        open={deleteTarget !== null}
        title={t('backups.deleteConfirmTitle')}
        description={t('backups.deleteConfirmDesc', { name: deleteTarget ?? '' })}
        confirmLabel={t('backups.deleteLabel')}
        onConfirm={doDelete}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}
