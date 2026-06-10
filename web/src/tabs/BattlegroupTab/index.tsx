import type React from 'react'
import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useAutoRefresh } from '../../hooks/useAutoRefresh'
import { Button, Input, Select, ListBox, Spinner, toast, TextField } from '@heroui/react'
import { api } from '../../api/client'
import type { BackupFile } from '../../api/client'
import { NumberInput, PageHeader, SectionDivider, Icon } from '../../dune-ui'
import { ScheduledRestartsCard } from '../../components/ScheduledRestartsCard'
import { useStatus } from '../../hooks/useStatus'

import { ACTIONS, INIT_WARN_MS, type ActionDef, type DetailedStatus } from './types'
import { ServersTable } from './ServersTable'
import {
  HealthCard, HealthChips, BgVmCard, ComponentHealthCard, GameReadyCard, WebInterfacesCard,
} from './components/ServerHealth'
import { ConfirmDialog } from './modals/ConfirmDialog'
import { CommandOutputModal } from './modals/CommandOutputModal'
import { RestoreModal } from './modals/RestoreModal'

const POLL_MS = 30_000

interface BattlegroupTabProps {
  isActive?: boolean
}

export const BattlegroupTab: React.FC<BattlegroupTabProps> = ({ isActive = false }) => {
  const { t } = useTranslation()
  const { status: connStatus } = useStatus()
  const [status, setStatus] = useState<DetailedStatus | null>(null)
  const [statusLoading, setStatusLoading] = useState(false)

  // Command lifecycle
  const [runningCmd, setRunningCmd] = useState<string | null>(null)
  const [cmdOutput, setCmdOutput] = useState<string | null>(null)
  const [cmdDone, setCmdDone] = useState(false)
  const [confirmCmd, setConfirmCmd] = useState<ActionDef | null>(null)
  const [startedAt, setStartedAt] = useState<number | null>(null)
  const [lastBackupFile, setLastBackupFile] = useState<string | null>(null)

  // Broadcasts
  const [broadcastTitle, setBroadcastTitle] = useState('')
  const [broadcastBody, setBroadcastBody] = useState('')
  const [broadcastDuration, setBroadcastDuration] = useState(30)
  const [broadcastBusy, setBroadcastBusy] = useState(false)
  const [shutdownType, setShutdownType] = useState('Restart')
  const [shutdownDelay, setShutdownDelay] = useState(10)
  const [shutdownBusy, setShutdownBusy] = useState(false)

  // Restore modal
  const [showRestore, setShowRestore] = useState(false)
  const [backupFiles, setBackupFiles] = useState<BackupFile[]>([])
  const [backupFilesLoading, setBackupFilesLoading] = useState(false)

  const fetchStatus = useCallback(() => {
    Promise.resolve()
      .then(() => setStatusLoading(true))
      .then(() => api.battlegroup.status() as Promise<unknown>)
      .then((res) => setStatus(res as DetailedStatus))
      .catch((e: unknown) => toast.danger(t('battlegroup.statusFailed', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setStatusLoading(false))
  }, [t])

  useEffect(() => {
    fetchStatus()
  }, [fetchStatus])

  const { countdown, refresh: refreshStatus } = useAutoRefresh(fetchStatus, POLL_MS, isActive)

  // isInitializing tracks whether we're inside the post-start warning window.
  // We use a boolean state rather than computing from Date.now() in render (impure).
  const [isInitializing, setIsInitializing] = useState(false)
  useEffect(() => {
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
        const match = (res.output || '').match(/database-dumps\/[^/]+\/([^\s]+\.backup)/)
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

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">

      {/* ── Header ───────────────────────────────────────────────────── */}
      <PageHeader
        title={t('serverHealth.title')}
        subtitle={t('serverHealth.subtitle')}
        onRefresh={refreshStatus}
        loading={statusLoading}
        countdown={isActive ? countdown : undefined}
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
          <BgVmCard bg={bg} servers={servers} />
          <GameReadyCard bg={bg} servers={servers} />
          <ComponentHealthCard bg={bg} servers={servers} status={connStatus} />
          <WebInterfacesCard status={connStatus} />
        </div>

        {/* Game servers */}
        <HealthCard
          title={t('serverHealth.gameServers')}
          icon="boxes"
          accessory={<span className="text-xs text-muted tabular-nums">{t('serverHealth.pods', { n: servers.length })}</span>}
        >
          {statusLoading && !status
            ? (
                <div className="flex items-center gap-2 py-4 text-muted">
                  <Spinner size="sm" color="current" />
                  <span className="text-sm">{t('battlegroup.loadingStatus')}</span>
                </div>
              )
            : (
                <ServersTable
                  servers={servers}
                  isInitializing={isInitializing}
                  emptyMessage={status ? t('battlegroup.noGameServers') : t('battlegroup.clickRefresh')}
                />
              )}
        </HealthCard>

        {/* ── Server Control ───────────────────────────────────────────── */}
        <SectionDivider title={t('battlegroup.serverControl')} />
        <div className="flex flex-wrap gap-2 shrink-0">
          {ACTIONS.map((action) => (
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
          <Button variant="danger-soft" size="sm" isDisabled={runningCmd !== null} onPress={openRestore}>
            {t('battlegroup.restoreLabel')}
          </Button>
        </div>

        {/* ── Broadcasts ──────────────────────────────────────────────── */}
        <SectionDivider title={t('battlegroup.broadcasts')} />
        <div className="flex flex-wrap gap-3 shrink-0">

          {/* Generic broadcast */}
          <div className="flex flex-col gap-2 flex-1 min-w-64 rounded-[var(--radius)] border border-border bg-surface p-3">
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
                      <>
                        <Icon name="megaphone" />
                        {' '}
                        {t('common.send')}
                      </>
                    )}
              </Button>
            </div>
          </div>

          {/* Shutdown broadcast */}
          <div className="flex flex-col gap-2 flex-1 min-w-64 rounded-[var(--radius)] border border-border bg-surface p-3">
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
                    toast.success(t('battlegroup.shutdownSent', { delay: shutdownDelay }))
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
                      <>
                        <Icon name="triangle-alert" />
                        {' '}
                        {t('battlegroup.broadcastBtn')}
                      </>
                    )}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                isDisabled={shutdownBusy}
                onPress={async () => {
                  setShutdownBusy(true)
                  try {
                    await api.broadcast.shutdown(shutdownType, 0, true)
                    toast.success(t('battlegroup.shutdownCancelled'))
                  }
                  catch (e: unknown) {
                    toast.danger(e instanceof Error ? e.message : String(e))
                  }
                  finally { setShutdownBusy(false) }
                }}
              >
                {t('common.cancel')}
              </Button>
            </div>
          </div>

        </div>

        {/* ── Scheduled Restarts (#145) ──────────────────────────────── */}
        <SectionDivider title={t('restarts.title')} />
        <ScheduledRestartsCard />

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
        onRestoreComplete={(output) => {
          setCmdOutput(output)
          setCmdDone(true)
          setRunningCmd('restore')
          setShowRestore(false)
        }}
      />
    </div>
  )
}
