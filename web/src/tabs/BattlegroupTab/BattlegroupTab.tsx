import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useAutoRefresh } from '../../hooks/useAutoRefresh'
import { Button, Input, Select, ListBox, Spinner, toast, TextField } from '@heroui/react'
import { api } from '../../api/client'
import type { BackupFile } from '../../api/client'
import { NumberInput, PageHeader, SectionDivider, Icon } from '../../dune-ui'
import { ScheduledRestartsCard } from '../../components/ScheduledRestartsCard'
import { useStatus } from '../../hooks/useStatus'
import { usePermissions } from '../../hooks/usePermissions'

import { ACTIONS, INIT_WARN_MS, type ActionDef, type DetailedStatus } from './types'
import { ServersTable } from './ServersTable'
import {
  HealthCard, HealthChips, BgVmCard, ComponentHealthCard, GameReadyCard, WebInterfacesCard,
} from './components/ServerHealth'
import { ConfirmDialog } from './modals/ConfirmDialog'
import { CommandOutputModal } from './modals/CommandOutputModal'
import { RestoreModal } from './modals/RestoreModal'
import { RestoreProgressModal } from '../../components/RestoreProgressModal'

const POLL_MS = 30_000

export const BattlegroupTab: React.FC = () => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const { status: connStatus } = useStatus()
  const [status, setStatus] = React.useState<DetailedStatus | null>(null)
  const [statusLoading, setStatusLoading] = React.useState(false)

  // Command lifecycle
  const [runningCmd, setRunningCmd] = React.useState<string | null>(null)
  const [cmdOutput, setCmdOutput] = React.useState<string | null>(null)
  const [cmdDone, setCmdDone] = React.useState(false)
  const [confirmCmd, setConfirmCmd] = React.useState<ActionDef | null>(null)
  const [startedAt, setStartedAt] = React.useState<number | null>(null)
  const [lastBackupFile, setLastBackupFile] = React.useState<string | null>(null)

  // Broadcasts
  const [broadcastTitle, setBroadcastTitle] = React.useState('')
  const [broadcastBody, setBroadcastBody] = React.useState('')
  const [broadcastDuration, setBroadcastDuration] = React.useState(30)
  const [broadcastBusy, setBroadcastBusy] = React.useState(false)
  const [shutdownType, setShutdownType] = React.useState('Restart')
  const [shutdownDelay, setShutdownDelay] = React.useState(10)
  const [shutdownBusy, setShutdownBusy] = React.useState(false)
  // A broadcast arms a pending restart/stop on the backend. The Cancel button
  // only shows while something is actually pending — not before a broadcast or
  // after one is cancelled. The backend is authoritative (exposed via /status),
  // so this rehydrates on a browser refresh and clears itself when the countdown
  // fires; the optimistic sets below just make the UI feel instant.
  const [shutdownPending, setShutdownPending] = React.useState(false)
  React.useEffect(() => {
    if (typeof connStatus?.shutdown_pending === 'boolean') {
      const pending = connStatus.shutdown_pending
      Promise.resolve().then(() => setShutdownPending(pending))
    }
  }, [connStatus?.shutdown_pending])

  // Restore modal
  const [showRestore, setShowRestore] = React.useState(false)
  const [restoreProgressOpen, setRestoreProgressOpen] = React.useState(false)
  const [backupFiles, setBackupFiles] = React.useState<BackupFile[]>([])
  const [backupFilesLoading, setBackupFilesLoading] = React.useState(false)

  const fetchStatus = (): void => {
    Promise.resolve()
      .then(() => setStatusLoading(true))
      .then(() => api.battlegroup.status() as Promise<unknown>)
      .then((res) => setStatus(res as DetailedStatus))
      .catch((e: unknown) => toast.danger(t('battlegroup.statusFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setStatusLoading(false))
  }

  React.useEffect(() => {
    fetchStatus()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const { countdown, refresh: refreshStatus } = useAutoRefresh(fetchStatus, POLL_MS)

  // isInitializing tracks whether we're inside the post-start warning window.
  // We use a boolean state rather than computing from Date.now() in render (impure).
  const [isInitializing, setIsInitializing] = React.useState(false)
  React.useEffect(() => {
    if (startedAt === null) {
      const t = setTimeout(() => setIsInitializing(false), 0)
      return () => clearTimeout(t)
    }
    const remaining = INIT_WARN_MS - (Date.now() - startedAt)
    if (remaining <= 0) {
      const t = setTimeout(() => setStartedAt(null), 0)
      return () => clearTimeout(t)
    }
    const tStart = setTimeout(() => setIsInitializing(true), 0)
    const tEnd = setTimeout(() => {
      setStartedAt(null)
      setIsInitializing(false)
    }, remaining)
    return () => {
      clearTimeout(tStart)
      clearTimeout(tEnd)
    }
  }, [startedAt])

  const runCmd = async (action: ActionDef) => {
    setConfirmCmd(null)
    setRunningCmd(action.cmd)
    setCmdOutput(null)
    setCmdDone(false)
    try {
      const res = await api.battlegroup.exec(action.cmd)
      setCmdOutput(res.output || t('battlegroup.noOutput'))
      setCmdDone(true)
      if (action.cmd === 'start' || action.cmd === 'restart') setStartedAt(Date.now())
      if (action.cmd === 'backup') {
        // kubectl: "…/database-dumps/<bg>/<name>.backup". AMP: "database backup
        // created: <name>.dump (N bytes)" (#169).
        const out = res.output || ''
        const match = out.match(/database-dumps\/[^/]+\/([^\s]+\.backup)/)
          || out.match(/([A-Za-z0-9._-]+\.dump)/)
        if (match) setLastBackupFile(match[1])
      }
      toast.success(t('battlegroup.cmdCompleted', { label: t(`battlegroup.actions.${action.cmd}` as never) }))
      fetchStatus()
    }
    catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setCmdOutput(`Error: ${msg}`)
      setCmdDone(true)
      toast.danger(t('battlegroup.cmdFailed', { label: t(`battlegroup.actions.${action.cmd}` as never), message: msg }))
    }
  }

  const openRestore = () => {
    setBackupFilesLoading(true)
    setBackupFiles([])
    setShowRestore(true)
    api.battlegroup.backupFiles()
      .then(setBackupFiles)
      .catch(() => toast.danger(t('battlegroup.backupLoadFailed')))
      .finally(() => setBackupFilesLoading(false))
  }

  const bg = status?.battlegroup
  const servers = status?.servers ?? []

  // First-load gate: show skeletons only while the very first status fetch is in
  // flight (no data yet). Background polls keep rendering real values.
  const firstLoad = statusLoading && !status

  // Backup/restore are only meaningful where the backend has a working backup
  // path: AMP (db-backup provider → pg_dump/.dump) and kubectl (battlegroup.sh →
  // .backup). docker/local would surface an erroring button, so hide it (#169).
  const backupSupported = connStatus?.control === 'amp' || connStatus?.control === 'kubectl'

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">

      {/* ── Header ───────────────────────────────────────────────────── */}
      <PageHeader
        title={t('serverHealth.title')}
        subtitle={t('serverHealth.subtitle')}
        onRefresh={refreshStatus}
        loading={statusLoading}
        countdown={countdown}
      />

      <HealthChips bg={bg} servers={servers} status={connStatus} />

      {isInitializing && (
        <div className="rounded-[var(--radius)] px-3 py-2 text-sm flex items-center gap-2 bg-warning/10 text-warning border border-warning/40 shrink-0">
          <Icon name="triangle-alert" />
          <span>{t('battlegroup.initWarning')}</span>
        </div>
      )}

      {/* ── Scrollable health body ───────────────────────────────────── */}
      <div className="flex-1 min-h-0 overflow-auto flex flex-col gap-3 pr-1">

        {/* Health card grid */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
          <BgVmCard bg={bg} servers={servers} loading={firstLoad} />
          <GameReadyCard bg={bg} servers={servers} loading={firstLoad} />
          <ComponentHealthCard bg={bg} servers={servers} status={connStatus} loading={firstLoad} />
          <WebInterfacesCard status={connStatus} />
        </div>

        {/* Game servers */}
        <HealthCard
          title={t('serverHealth.gameServers')}
          icon="boxes"
          accessory={<span className="text-xs text-muted tabular-nums">{t('serverHealth.pods', { n: servers.length })}</span>}
        >
          <ServersTable
            servers={servers}
            isInitializing={isInitializing}
            loading={firstLoad}
            emptyMessage={status ? t('battlegroup.noGameServers') : t('battlegroup.clickRefresh')}
          />
        </HealthCard>

        {/* ── Server Control ───────────────────────────────────────────── */}
        {(can('server:control') || can('backups:manage')) && (
          <React.Fragment>
            <SectionDivider title={t('battlegroup.serverControl')} />
            <div className="flex flex-wrap gap-2 shrink-0">
              {ACTIONS
                .filter((action) => (action.cmd === 'backup'
                  ? can('backups:manage') && backupSupported
                  : can('server:control')))
                .map((action) => (
                  <Button
                    key={action.cmd}
                    variant={action.danger ? 'danger-soft' : 'outline'}
                    onPress={() => setConfirmCmd(action)}
                    isDisabled={runningCmd !== null}
                    size="sm"
                  >
                    {t(`battlegroup.actions.${action.cmd}` as never)}
                  </Button>
                ))}
              {can('backups:manage') && backupSupported && (
                <Button variant="danger-soft" size="sm" isDisabled={runningCmd !== null} onPress={openRestore}>
                  {t('battlegroup.restoreLabel')}
                </Button>
              )}
            </div>
          </React.Fragment>
        )}

        {/* ── Broadcasts ──────────────────────────────────────────────── */}
        {can('broadcast:send') && (
          <React.Fragment>
            <SectionDivider title={t('battlegroup.broadcasts')} />
            <div className="flex flex-wrap gap-3 shrink-0">

              {/* Generic broadcast */}
              <div className="dune-lift flex flex-col gap-2 flex-1 min-w-64 rounded-[var(--radius)] border border-border bg-surface p-8">
                <div className="text-xs font-semibold uppercase tracking-widest text-accent">{t('battlegroup.genericMessage')}</div>
                <TextField aria-label={t('battlegroup.titlePlaceholder')}>
                  <Input placeholder={t('battlegroup.titlePlaceholder')} value={broadcastTitle} onChange={(e) => setBroadcastTitle(e.target.value)} />
                </TextField>
                <TextField aria-label={t('battlegroup.bodyPlaceholder')}>
                  <Input placeholder={t('battlegroup.bodyPlaceholder')} value={broadcastBody} onChange={(e) => setBroadcastBody(e.target.value)} />
                </TextField>
                <div className="flex items-center gap-2">
                  <label className="text-xs text-muted shrink-0">{t('battlegroup.durationLabel')}</label>
                  <NumberInput
                    ariaLabel={t('battlegroup.durationLabel')}
                    min={5}
                    max={300}
                    value={broadcastDuration}
                    onChange={setBroadcastDuration}
                    showButtons={false}
                    className="w-24"
                  />
                  <div className="flex-1" />
                  <Button
                    size="sm"
                    isDisabled={broadcastBusy || !broadcastTitle}
                    onPress={async () => {
                      setBroadcastBusy(true)
                      try {
                        await api.broadcast.send([{ Key: 'en', Title: broadcastTitle, Body: broadcastBody }], broadcastDuration)
                        toast.success(t('battlegroup.broadcastSent'))
                        setBroadcastTitle('')
                        setBroadcastBody('')
                      }
                      catch (e: unknown) {
                        toast.danger(e instanceof Error ? e.message : String(e))
                      }
                      finally { setBroadcastBusy(false) }
                    }}
                  >
                    {broadcastBusy
                      ? <Spinner size="sm" color="current" />
                      : (
                          <React.Fragment>
                            <Icon name="megaphone" />
                            {' '}
                            {t('common.send')}
                          </React.Fragment>
                        )}
                  </Button>
                </div>
              </div>

              {/* Shutdown broadcast */}
              <div className="dune-lift flex flex-col gap-2 flex-1 min-w-64 rounded-[var(--radius)] border border-border bg-surface p-8">
                <div className="text-xs font-semibold uppercase tracking-widest text-accent">{t('battlegroup.shutdownBroadcast')}</div>
                <div className="flex items-center gap-2">
                  <label className="text-xs text-muted shrink-0">{t('battlegroup.shutdownType')}</label>
                  <Select selectedKey={shutdownType} onSelectionChange={(k) => setShutdownType(String(k))} className="flex-1" aria-label={t('battlegroup.shutdownTypeLabel')}>
                    <Select.Trigger>
                      <Select.Value />
                      <Select.Indicator />
                    </Select.Trigger>
                    <Select.Popover>
                      <ListBox>
                        {['Restart', 'Maintenance', 'Update'].map((t) => (
                          <ListBox.Item key={t} id={t} textValue={t}>
                            {t}
                            <ListBox.ItemIndicator />
                          </ListBox.Item>
                        ))}
                      </ListBox>
                    </Select.Popover>
                  </Select>
                </div>
                <div className="flex items-center gap-2">
                  <label className="text-xs text-muted shrink-0">{t('battlegroup.shutdownDelay')}</label>
                  <NumberInput
                    ariaLabel={t('battlegroup.shutdownDelayLabel')}
                    min={1}
                    max={120}
                    value={shutdownDelay}
                    onChange={setShutdownDelay}
                    showButtons={false}
                    className="w-24"
                  />
                </div>
                <div className="flex gap-2 mt-auto">
                  <Button
                    size="sm"
                    variant="danger-soft"
                    isDisabled={shutdownBusy}
                    onPress={async () => {
                      setShutdownBusy(true)
                      try {
                        await api.broadcast.shutdown(shutdownType, shutdownDelay)
                        toast.success(t('battlegroup.shutdownSent', { type: shutdownType, delay: shutdownDelay }))
                        setShutdownPending(true)
                      }
                      catch (e: unknown) {
                        toast.danger(e instanceof Error ? e.message : String(e))
                      }
                      finally { setShutdownBusy(false) }
                    }}
                  >
                    {shutdownBusy
                      ? <Spinner size="sm" color="current" />
                      : (
                          <React.Fragment>
                            <Icon name="triangle-alert" />
                            {' '}
                            {t('battlegroup.broadcastBtn')}
                            {' '}
                            {shutdownType}
                          </React.Fragment>
                        )}
                  </Button>
                  {shutdownPending && (
                    <Button
                      size="sm"
                      variant="ghost"
                      isDisabled={shutdownBusy}
                      onPress={async () => {
                        setShutdownBusy(true)
                        try {
                          await api.broadcast.shutdown(shutdownType, 0, true)
                          toast.success(t('battlegroup.shutdownCancelled'))
                          setShutdownPending(false)
                        }
                        catch (e: unknown) {
                          toast.danger(e instanceof Error ? e.message : String(e))
                        }
                        finally { setShutdownBusy(false) }
                      }}
                    >
                      {t('common.cancel')}
                    </Button>
                  )}
                </div>
              </div>

            </div>
          </React.Fragment>
        )}

        {/* ── Scheduled Restarts (#145) ──────────────────────────────── */}
        {can('restarts:read') && <ScheduledRestartsCard />}

      </div>

      {/* ── Modals ───────────────────────────────────────────────────── */}
      <ConfirmDialog
        action={confirmCmd}
        onConfirm={runCmd}
        onClose={() => setConfirmCmd(null)}
      />
      <CommandOutputModal
        runningCmd={runningCmd}
        cmdOutput={cmdOutput}
        cmdDone={cmdDone}
        lastBackupFile={lastBackupFile}
        onClose={() => {
          setRunningCmd(null)
          setCmdOutput(null)
        }}
      />
      <RestoreModal
        open={showRestore}
        backupFiles={backupFiles}
        backupFilesLoading={backupFilesLoading}
        setBackupFiles={setBackupFiles}
        onClose={() => setShowRestore(false)}
        onRestoreStarted={() => {
          setShowRestore(false)
          setRestoreProgressOpen(true)
        }}
      />
      <RestoreProgressModal
        open={restoreProgressOpen}
        onClose={() => setRestoreProgressOpen(false)}
      />
    </div>
  )
}
