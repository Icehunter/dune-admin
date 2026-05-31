import { useState, useEffect, useCallback } from 'react'
import { useAutoRefresh } from '../../hooks/useAutoRefresh'
import { Button, Input, Select, ListBox, Spinner, toast, TextField } from '@heroui/react'
import { api } from '../../api/client'
import type { BackupFile } from '../../api/client'
import { PageHeader, InfoCard, SectionDivider, Icon } from '../../dune-ui'

import { phaseColor } from './helpers'
import { ACTIONS, INIT_WARN_MS, type ActionDef, type DetailedStatus } from './types'
import { ServersTable } from './ServersTable'
import { ConfirmDialog } from './modals/ConfirmDialog'
import { CommandOutputModal } from './modals/CommandOutputModal'
import { RestoreModal } from './modals/RestoreModal'

const POLL_MS = 30_000

export default function BattlegroupTab({ isActive = false }: { isActive?: boolean }) {
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
      .catch((e: unknown) => toast.danger(`Status failed: ${e instanceof Error ? e.message : String(e)}`))
      .finally(() => setStatusLoading(false))
  }, [])

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
    setRunningCmd(action.label)
    setCmdOutput(null)
    setCmdDone(false)
    try {
      const res = await api.battlegroup.exec(action.cmd)
      setCmdOutput(res.output || '(no output)')
      setCmdDone(true)
      if (action.cmd === 'start' || action.cmd === 'restart') setStartedAt(Date.now())
      if (action.cmd === 'backup') {
        const match = (res.output || '').match(/database-dumps\/[^/]+\/([^\s]+\.backup)/)
        if (match) setLastBackupFile(match[1])
      }
      toast.success(`${action.label} completed`)
      fetchStatus()
    }
    catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setCmdOutput(`Error: ${msg}`)
      setCmdDone(true)
      toast.danger(`${action.label} failed: ${msg}`)
    }
  }

  const openRestore = () => {
    setBackupFilesLoading(true)
    setBackupFiles([])
    setShowRestore(true)
    api.battlegroup.backupFiles()
      .then(setBackupFiles)
      .catch(() => toast.danger('Could not load backup files'))
      .finally(() => setBackupFilesLoading(false))
  }

  const bg = status?.battlegroup
  const servers = status?.servers ?? []

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">

      {/* ── Overview ─────────────────────────────────────────────────── */}
      <PageHeader title={bg ? `${bg.title} (${bg.name})` : 'Battlegroup Status'}>
        <Button size="sm" variant="ghost" onPress={refreshStatus} isDisabled={statusLoading}>
          {statusLoading
            ? <Spinner size="sm" color="current" />
            : (
                <>
                  {isActive && (
                    <span className="w-7 text-right tabular-nums text-muted/60 text-xs">
                      {countdown}
                      s
                    </span>
                  )}
                  <Icon name="refresh-cw" />
                </>
              )}
        </Button>
      </PageHeader>

      {bg && (
        <InfoCard>
          <InfoCard.Item label="Phase" value={bg.phase || '—'} valueColor={phaseColor(bg.phase)} />
          <InfoCard.Item label="Database" value={bg.database || '—'} valueColor={phaseColor(bg.database)} />
        </InfoCard>
      )}

      {isInitializing && (
        <div className="rounded-[var(--radius)] px-3 py-2 text-sm flex items-center gap-2 bg-warning/10 text-warning border border-warning/40 shrink-0">
          <Icon name="triangle-alert" />
          <span>Server just started — game servers may still be initializing (~2 min).</span>
        </div>
      )}

      <div className="flex-1 min-h-0 flex flex-col">
        {statusLoading && !status
          ? (
              <div className="flex items-center gap-2 py-4 text-muted">
                <Spinner size="sm" color="current" />
                <span className="text-sm">Loading status...</span>
              </div>
            )
          : (
              <ServersTable
                servers={servers}
                isInitializing={isInitializing}
                emptyMessage={status ? 'No game servers found.' : 'Click Refresh to load status.'}
              />
            )}
      </div>

      {/* ── Server Control ───────────────────────────────────────────── */}
      <SectionDivider title="Server Control" />
      <div className="flex flex-wrap gap-2 shrink-0">
        {ACTIONS.map((action) => (
          <Button
            key={action.cmd}
            variant={action.danger ? 'danger-soft' : 'outline'}
            onPress={() => setConfirmCmd(action)}
            isDisabled={runningCmd !== null}
            size="sm"
          >
            {action.label}
          </Button>
        ))}
        <Button variant="danger-soft" size="sm" isDisabled={runningCmd !== null} onPress={openRestore}>
          Restore
        </Button>
      </div>

      {/* ── Broadcasts ──────────────────────────────────────────────── */}
      <SectionDivider title="Broadcasts" />
      <div className="flex flex-wrap gap-3 shrink-0">

        {/* Generic broadcast */}
        <div className="flex flex-col gap-2 flex-1 min-w-64 rounded-[var(--radius)] border border-border bg-surface p-3">
          <div className="text-xs font-semibold uppercase tracking-widest text-accent">Generic Message</div>
          <TextField aria-label="Title">
            <Input placeholder="Title" value={broadcastTitle} onChange={(e) => setBroadcastTitle(e.target.value)} />
          </TextField>
          <TextField aria-label="Body">
            <Input placeholder="Body" value={broadcastBody} onChange={(e) => setBroadcastBody(e.target.value)} />
          </TextField>
          <div className="flex items-center gap-2">
            <label className="text-xs text-muted shrink-0">Duration (s)</label>
            <Input
              type="number"
              min={5}
              max={300}
              value={broadcastDuration}
              onChange={(e) => setBroadcastDuration(Math.max(5, parseInt(e.target.value) || 30))}
              className="w-20"
              aria-label="Duration"
            />
            <div className="flex-1" />
            <Button
              size="sm"
              isDisabled={broadcastBusy || !broadcastTitle}
              onPress={async () => {
                setBroadcastBusy(true)
                try {
                  await api.broadcast.send([{ Key: 'en', Title: broadcastTitle, Body: broadcastBody }], broadcastDuration)
                  toast.success('Broadcast sent')
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
                      Send
                    </>
                  )}
            </Button>
          </div>
        </div>

        {/* Shutdown broadcast */}
        <div className="flex flex-col gap-2 flex-1 min-w-64 rounded-[var(--radius)] border border-border bg-surface p-3">
          <div className="text-xs font-semibold uppercase tracking-widest text-accent">Shutdown Broadcast</div>
          <div className="flex items-center gap-2">
            <label className="text-xs text-muted shrink-0">Type</label>
            <Select selectedKey={shutdownType} onSelectionChange={(k) => setShutdownType(String(k))} className="flex-1" aria-label="Shutdown type">
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
            <label className="text-xs text-muted shrink-0">Delay (min)</label>
            <Input
              type="number"
              min={1}
              max={120}
              value={shutdownDelay}
              onChange={(e) => setShutdownDelay(Math.max(1, parseInt(e.target.value) || 10))}
              className="w-20"
              aria-label="Delay minutes"
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
                  toast.success(`Shutdown broadcast sent (${shutdownDelay} min)`)
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
                      Broadcast
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
                  toast.success('Shutdown broadcast cancelled')
                }
                catch (e: unknown) {
                  toast.danger(e instanceof Error ? e.message : String(e))
                }
                finally { setShutdownBusy(false) }
              }}
            >
              Cancel
            </Button>
          </div>
        </div>

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
          setRunningCmd('Restore')
          setShowRestore(false)
        }}
      />
    </div>
  )
}
