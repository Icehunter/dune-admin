import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Spinner, toast } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { api } from '../../../api/client'
import type { DBBackupFile } from '../../../api/client'
import { Panel, SectionLabel, PageHeader, Icon, ConfirmDialog } from '../../../dune-ui'
import { usePermissions } from '../../../hooks/usePermissions'
import { ScheduleCard } from './ScheduleCard'
import type { BackupsViewProps } from './interfaces'

const fmtSize = (b: number): string => {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`
  return `${(b / 1024 / 1024 / 1024).toFixed(1)} GB`
}

export const BackupsView: React.FC<BackupsViewProps> = ({ onRegisterRefresh, headerContent }) => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const [backups, setBackups] = React.useState<DBBackupFile[]>([])
  const [loading, setLoading] = React.useState(true)
  const [taking, setTaking] = React.useState(false)
  const [restoreTarget, setRestoreTarget] = React.useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = React.useState<string | null>(null)
  const [busy, setBusy] = React.useState(false)

  const load = (): void => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.dbBackups.list())
      .then((res) => setBackups(res.backups ?? []))
      .catch((e: unknown) =>
        toast.danger(t('backups.loadFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }

  React.useEffect(() => {
    if (onRegisterRefresh) onRegisterRefresh(load)
  }, [onRegisterRefresh]) // eslint-disable-line react-hooks/exhaustive-deps

  React.useEffect(() => {
    load()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

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
      <PageHeader title={t('database.sections.backups')}>
        {headerContent}
      </PageHeader>

      <div className="rounded-[var(--radius)] px-3 py-2 text-sm flex items-start gap-2 bg-warning/10 text-warning border border-warning/40 shrink-0">
        <Icon name="triangle-alert" className="size-4 mt-0.5 shrink-0" />
        <span>{t('backups.warning')}</span>
      </div>

      <div className="flex-1 min-h-0 overflow-auto flex flex-col gap-3 pr-1">
        {/* Take Backup */}
        {can('backups:manage') && (
          <Panel>
            <SectionLabel>{t('backups.take.title')}</SectionLabel>
            <p className="text-xs text-muted">{t('backups.take.desc')}</p>
            <div>
              <Button size="sm" onPress={take} isDisabled={taking}>
                {taking
                  ? <Spinner size="sm" color="current" />
                  : (
                      <React.Fragment>
                        <Icon name="database-backup" />
                        {' '}
                        {t('backups.take.btn')}
                      </React.Fragment>
                    )}
              </Button>
            </div>
          </Panel>
        )}

        <ScheduleCard />

        {/* Recent backups */}
        {can('backups:read') && (
          <Panel>
            <SectionLabel>{t('backups.recent.title')}</SectionLabel>
            {loading
              ? <div className="py-3 flex justify-center"><Spinner size="sm" color="current" /></div>
              : backups.length === 0
                ? (
                    <EmptyState size="sm">
                      <EmptyState.Header>
                        <EmptyState.Media variant="icon">
                          <IconifyIcon icon="gravity-ui:document" className="size-5" />
                        </EmptyState.Media>
                        <EmptyState.Title>{t('backups.recent.empty')}</EmptyState.Title>
                      </EmptyState.Header>
                    </EmptyState>
                  )
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
                            {can('backups:read') && (
                              <a href={api.dbBackups.downloadUrl(b.name)} download>
                                <Button size="sm" variant="ghost" isIconOnly aria-label={t('backups.download')}>
                                  <Icon name="download" />
                                </Button>
                              </a>
                            )}
                            {can('backups:manage') && (
                              <Button
                                size="sm"
                                variant="outline"
                                isDisabled={busy}
                                onPress={() => setRestoreTarget(b.name)}
                              >
                                {t('backups.restoreLabel')}
                              </Button>
                            )}
                            {can('backups:manage') && (
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
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
          </Panel>
        )}
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
