import { useState, useEffect, useCallback } from 'react'
import { Button, Modal, Spinner, toast } from '@heroui/react'
import { api } from '../api/client'
import type { BackupFile } from '../api/client'
import { useTableSort } from '../hooks/useTableSort'
import { SortIndicator } from '../components/SortIndicator'

type ServerSortKey = 'map' | 'phase' | 'players' | 'ready' | 'dimension' | 'partition'

const SERVER_COLUMNS: { key: ServerSortKey; label: string }[] = [
  { key: 'map', label: 'Map' },
  { key: 'phase', label: 'Phase' },
  { key: 'players', label: 'Players' },
  { key: 'ready', label: 'Ready' },
  { key: 'dimension', label: 'Dim' },
  { key: 'partition', label: 'Part' },
]

type ServerRow = {
  map: string
  sietch: string
  dimension: number
  partition: number
  phase: string
  ready: boolean
  players: number
}

type BGInfo = {
  name: string
  title: string
  phase: string
  database: string
}

type DetailedStatus = {
  battlegroup: BGInfo
  servers: ServerRow[]
}

const INIT_WARN_MS = 3 * 60 * 1000

function phaseColor(phase: string): string {
  switch (phase?.toLowerCase()) {
    case 'running':    return '#27ae60'
    case 'reconciling':
    case 'starting':
    case 'initializing': return '#f0a830'
    case 'stopping':
    case 'preshutdown':
    case 'terminating': return '#e87040'
    case 'stopped':
    case 'terminated': return '#555'
    default:           return '#aaa'
  }
}

const ACTIONS = [
  { label: 'Start',   cmd: 'start',   danger: false, msg: 'Start the battlegroup server?' },
  { label: 'Stop',    cmd: 'stop',    danger: true,  msg: 'Stop the server? All players will be disconnected.' },
  { label: 'Restart', cmd: 'restart', danger: false, msg: 'Restart the server? Players will be briefly disconnected.' },
  { label: 'Update',  cmd: 'update',  danger: false, msg: 'Run a server update? This takes the server offline briefly.' },
  { label: 'Backup',  cmd: 'backup',  danger: false, msg: 'Create a database backup? This may take a few minutes.' },
]

type ActionDef = typeof ACTIONS[0]

export default function BattlegroupTab() {
  const [status, setStatus] = useState<DetailedStatus | null>(null)
  const [statusLoading, setStatusLoading] = useState(false)
  const [runningCmd, setRunningCmd] = useState<string | null>(null)
  const [cmdOutput, setCmdOutput] = useState<string | null>(null)
  const [cmdDone, setCmdDone] = useState(false)
  const [confirmCmd, setConfirmCmd] = useState<ActionDef | null>(null)
  const [startedAt, setStartedAt] = useState<number | null>(null)
  const [lastBackupFile, setLastBackupFile] = useState<string | null>(null)
  const [showRestore, setShowRestore] = useState(false)
  const [backupFiles, setBackupFiles] = useState<BackupFile[]>([])
  const [backupFilesLoading, setBackupFilesLoading] = useState(false)
  const [selectedRestoreFile, setSelectedRestoreFile] = useState('')
  const [restoreRunning, setRestoreRunning] = useState(false)
  const [uploadDragging, setUploadDragging] = useState(false)
  const [uploading, setUploading] = useState(false)

  const fetchStatus = useCallback(async () => {
    setStatusLoading(true)
    try {
      const res = await api.battlegroup.status() as unknown as DetailedStatus
      setStatus(res)
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      toast.danger(`Status failed: ${msg}`)
    } finally {
      setStatusLoading(false)
    }
  }, [])

  useEffect(() => { fetchStatus() }, [fetchStatus])

  const [, forceRender] = useState(0)
  useEffect(() => {
    if (startedAt === null) return
    const remaining = INIT_WARN_MS - (Date.now() - startedAt)
    if (remaining <= 0) { setStartedAt(null); return }
    const t = setTimeout(() => { setStartedAt(null); forceRender(n => n + 1) }, remaining)
    return () => clearTimeout(t)
  }, [startedAt])

  const isInitializing = startedAt !== null && (Date.now() - startedAt) < INIT_WARN_MS

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
        // Parse filename from "Backup file (on this host): /funcom/artifacts/database-dumps/<bg>/<name>"
        const match = (res.output || '').match(/database-dumps\/[^/]+\/([^\s]+\.backup)/)
        if (match) setLastBackupFile(match[1])
      }
      toast.success(`${action.label} completed`)
      fetchStatus()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setCmdOutput(`Error: ${msg}`)
      setCmdDone(true)
      toast.danger(`${action.label} failed: ${msg}`)
    }
  }

  const bg = status?.battlegroup
  const servers = status?.servers ?? []
  const { sorted: sortedServers, sortKey: serverSortKey, sortDir: serverSortDir, toggle: toggleServerSort } =
    useTableSort<ServerRow, ServerSortKey>(servers, 'map', (r, k) => {
      if (k === 'ready') return r.ready ? 1 : 0
      return r[k] as string | number
    })

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: '16px', gap: '0' }}>

      {/* Battlegroup overview */}
      <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', gap: '12px' }}>
        <div className="flex items-center gap-3">
          <h2 className="text-base font-semibold" style={{ color: 'var(--color-primary)' }}>
            {bg ? `${bg.title} (${bg.name})` : 'Battlegroup Status'}
          </h2>
          <Button size="sm" variant="ghost" onPress={fetchStatus} isDisabled={statusLoading}>
            {statusLoading ? <Spinner size="sm" color="current" /> : '↻ Refresh'}
          </Button>
        </div>

        {/* Battlegroup health row */}
        {bg && (
          <div className="flex items-center gap-4 rounded-lg px-4 py-3 text-sm"
            style={{ background: '#0f0d09', border: '1px solid #2a2418' }}>
            <div className="flex items-center gap-2">
              <span style={{ color: 'var(--color-text-dim)' }}>Phase</span>
              <span className="font-semibold" style={{ color: phaseColor(bg.phase) }}>{bg.phase || '—'}</span>
            </div>
            <div className="flex items-center gap-2">
              <span style={{ color: 'var(--color-text-dim)' }}>Database</span>
              <span className="font-semibold" style={{ color: phaseColor(bg.database) }}>{bg.database || '—'}</span>
            </div>
          </div>
        )}

        {isInitializing && (
          <div className="rounded-lg px-3 py-2 text-sm flex items-center gap-2"
            style={{ background: '#1a1400', color: '#f0a830', border: '1px solid #3a2800' }}>
            <span>⚠</span>
            <span>Server just started — game servers may still be initializing (~2 min).</span>
          </div>
        )}

        {/* Game servers table */}
        <div style={{ flex: 1, minHeight: 0, overflowY: 'auto' }}>
          {statusLoading && !status ? (
            <div className="flex items-center gap-2 py-4" style={{ color: 'var(--color-text-dim)' }}>
              <Spinner size="sm" color="current" />
              <span className="text-sm">Loading status...</span>
            </div>
          ) : servers.length === 0 ? (
            <p className="text-sm" style={{ color: 'var(--color-text-dim)' }}>
              {status ? 'No game servers found.' : 'Click Refresh to load status.'}
            </p>
          ) : (
            <div className="overflow-auto rounded-lg" style={{ border: '1px solid #2a2418' }}>
              <table className="w-full text-sm">
                <thead>
                  <tr style={{ background: '#1a1610', borderBottom: '1px solid #2a2418' }}>
                    {SERVER_COLUMNS.map(c => (
                      <th
                        key={c.key}
                        onClick={() => toggleServerSort(c.key)}
                        className="text-left px-4 py-2 font-semibold text-xs uppercase tracking-wide select-none"
                        style={{ color: 'var(--color-primary)', cursor: 'pointer' }}
                      >
                        {c.label}
                        <SortIndicator active={serverSortKey === c.key} dir={serverSortDir} />
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {sortedServers.map((s, i) => (
                    <tr key={`${s.map}-${s.dimension}-${s.partition}`}
                      style={{ borderBottom: '1px solid #1a1610', background: i % 2 === 0 ? '#0d0b07' : '#111009' }}>
                      <td className="px-4 py-2 font-mono text-xs" style={{ color: 'var(--color-text)' }}>{s.map}</td>
                      <td className="px-4 py-2 text-xs font-semibold" style={{ color: phaseColor(s.phase) }}>
                        {s.phase || '—'}
                        {isInitializing && s.phase === 'Running' && (
                          <span className="ml-1 font-normal" style={{ color: '#f0a830' }}>(initializing)</span>
                        )}
                      </td>
                      <td className="px-4 py-2 text-xs font-semibold" style={{ color: s.players > 0 ? '#27ae60' : 'var(--color-text-dim)' }}>
                        {s.players}
                      </td>
                      <td className="px-4 py-2 text-xs" style={{ color: s.ready ? '#27ae60' : '#e87040' }}>
                        {s.ready ? '✓' : '✗'}
                      </td>
                      <td className="px-4 py-2 text-xs" style={{ color: 'var(--color-text-dim)' }}>{s.dimension}</td>
                      <td className="px-4 py-2 text-xs" style={{ color: 'var(--color-text-dim)' }}>{s.partition}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      {/* Action buttons */}
      <div className="shrink-0" style={{ borderTop: '1px solid #2a2418', paddingTop: '12px', marginTop: '12px' }}>
        <h2 className="text-base font-semibold mb-3" style={{ color: 'var(--color-primary)' }}>
          Server Control
        </h2>
        <div className="flex flex-wrap gap-2">
          {ACTIONS.map(action => (
            <Button key={action.cmd}
              variant={action.danger ? 'danger-soft' : 'outline'}
              onPress={() => setConfirmCmd(action)}
              isDisabled={runningCmd !== null}
              size="sm">
              {action.label}
            </Button>
          ))}
          <Button variant="danger-soft" size="sm" isDisabled={runningCmd !== null}
            onPress={() => {
              setBackupFilesLoading(true)
              setBackupFiles([])
              setSelectedRestoreFile('')
              setShowRestore(true)
              api.battlegroup.backupFiles()
                .then(setBackupFiles)
                .catch(() => toast.danger('Could not load backup files'))
                .finally(() => setBackupFilesLoading(false))
            }}>
            Restore
          </Button>
        </div>
      </div>

      {/* Confirm dialog */}
      <Modal>
        <Modal.Backdrop isOpen={confirmCmd !== null} onOpenChange={v => { if (!v) setConfirmCmd(null) }}>
          <Modal.Container>
            <Modal.Dialog>
              <Modal.CloseTrigger />
              <Modal.Header><Modal.Heading>{confirmCmd?.label ?? ''} Server</Modal.Heading></Modal.Header>
              <Modal.Body>
                <p style={{ color: 'var(--color-text)' }}>{confirmCmd?.msg ?? ''}</p>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="tertiary" onPress={() => setConfirmCmd(null)}>Cancel</Button>
                <Button variant={confirmCmd?.danger ? 'danger' : 'primary'}
                  onPress={() => confirmCmd && runCmd(confirmCmd)}>
                  Confirm {confirmCmd?.label ?? ''}
                </Button>
              </Modal.Footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>

      {/* Running command modal */}
      <Modal>
        <Modal.Backdrop isOpen={runningCmd !== null}
          onOpenChange={v => { if (!v && cmdDone) { setRunningCmd(null); setCmdOutput(null) } }}>
          <Modal.Container>
            <Modal.Dialog>
              <Modal.Header><Modal.Heading>{runningCmd ?? ''}</Modal.Heading></Modal.Header>
              <Modal.Body>
                {!cmdDone ? (
                  <div className="flex flex-col items-center gap-4 py-6">
                    <Spinner size="lg" />
                    <p className="text-sm" style={{ color: 'var(--color-text-dim)' }}>
                      Running {runningCmd?.toLowerCase() ?? ''}...
                    </p>
                  </div>
                ) : (
                  <div className="rounded-lg p-3 font-mono text-xs overflow-auto max-h-60"
                    style={{ background: '#0a0806', color: '#a8d8a8', border: '1px solid #2a2418' }}>
                    <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{cmdOutput}</pre>
                  </div>
                )}
              </Modal.Body>
              {cmdDone && (
                <Modal.Footer>
                  {lastBackupFile && runningCmd === 'Backup' && (
                    <a
                      href={api.battlegroup.backupDownloadUrl(lastBackupFile)}
                      download={lastBackupFile.replace('.backup', '.zip')}
                      className="text-sm px-3 py-1.5 rounded"
                      style={{ background: '#1a2a1a', color: '#8d8', border: '1px solid #2a4a2a', textDecoration: 'none' }}
                    >
                      ↓ Download
                    </a>
                  )}
                  <Button onPress={() => { setRunningCmd(null); setCmdOutput(null) }}>Close</Button>
                </Modal.Footer>
              )}
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
      {/* Restore modal — file picker */}
      <Modal>
        <Modal.Backdrop isOpen={showRestore} onOpenChange={v => { if (!v && !restoreRunning) setShowRestore(false) }}>
          <Modal.Container>
            <Modal.Dialog style={{ width: '640px', maxWidth: '90vw' }}>
              <Modal.CloseTrigger />
              <Modal.Header><Modal.Heading>Restore from Backup</Modal.Heading></Modal.Header>
              <Modal.Body>
                <p className="text-sm mb-3" style={{ color: '#e87040' }}>
                  ⚠ This overwrites current data. Server should be stopped first.
                </p>
                {/* Drop zone for uploading a local backup */}
                <div
                  className="rounded-lg flex flex-col items-center justify-center gap-2 py-4 mb-3 text-sm cursor-pointer"
                  style={{
                    border: `2px dashed ${uploadDragging ? '#f0a830' : '#3a3020'}`,
                    background: uploadDragging ? '#1a1400' : '#0d0b07',
                    color: uploadDragging ? '#f0a830' : 'var(--color-text-dim)',
                    transition: 'all 0.15s',
                  }}
                  onDragOver={e => { e.preventDefault(); setUploadDragging(true) }}
                  onDragLeave={() => setUploadDragging(false)}
                  onDrop={async e => {
                    e.preventDefault()
                    setUploadDragging(false)
                    const file = e.dataTransfer.files[0]
                    if (!file?.name.endsWith('.backup') && !file?.name.endsWith('.zip')) { toast.warning('Drop a .backup or .zip file'); return }
                    setUploading(true)
                    try {
                      const res = await api.battlegroup.backupUpload(file)
                      toast.success(`Uploaded ${res.name}`)
                      const updated = await api.battlegroup.backupFiles()
                      setBackupFiles(updated)
                      setSelectedRestoreFile(res.name)
                    } catch (e: unknown) {
                      toast.danger(e instanceof Error ? e.message : String(e))
                    } finally {
                      setUploading(false)
                    }
                  }}
                  onClick={() => {
                    const input = document.createElement('input')
                    input.type = 'file'
                    input.accept = '.backup,.zip'
                    input.onchange = async () => {
                      const file = input.files?.[0]
                      if (!file) return
                      setUploading(true)
                      try {
                        const res = await api.battlegroup.backupUpload(file)
                        toast.success(`Uploaded ${res.name}`)
                        const updated = await api.battlegroup.backupFiles()
                        setBackupFiles(updated)
                        setSelectedRestoreFile(res.name)
                      } catch (e: unknown) {
                        toast.danger(e instanceof Error ? e.message : String(e))
                      } finally {
                        setUploading(false)
                      }
                    }
                    input.click()
                  }}
                >
                  {uploading ? <><Spinner size="sm" color="current" /><span>Uploading…</span></> : <span>↑ Drop or click to upload a .backup or .zip from your computer</span>}
                </div>

                {backupFilesLoading ? (
                  <div className="flex justify-center py-4"><Spinner /></div>
                ) : backupFiles.length === 0 ? (
                  <p className="text-sm" style={{ color: 'var(--color-text-dim)' }}>No backup files on server yet.</p>
                ) : (
                  <div className="flex flex-col gap-1">
                    {backupFiles.map(f => (
                      <label key={f.name} className="flex items-center gap-3 rounded px-3 py-2 cursor-pointer"
                        style={{ background: selectedRestoreFile === f.name ? '#1a2a1a' : '#0d0b07', border: `1px solid ${selectedRestoreFile === f.name ? '#2a4a2a' : '#2a2418'}` }}>
                        <input type="radio" name="restore-file" value={f.name}
                          checked={selectedRestoreFile === f.name}
                          onChange={() => setSelectedRestoreFile(f.name)} />
                        <div className="flex-1 min-w-0">
                          <div className="text-xs font-mono" style={{ color: 'var(--color-text)' }}>{f.name}</div>
                          <div className="text-xs flex items-center gap-2" style={{ color: 'var(--color-text-dim)' }}>
                            <span>{(f.size_bytes / 1024 / 1024).toFixed(1)} MB · {f.modified}</span>
                            {f.has_yaml && <span className="px-1 rounded" style={{ background: '#1a2a1a', color: '#8d8', fontSize: '10px' }}>+yaml</span>}
                          </div>
                        </div>
                        <a href={api.battlegroup.backupDownloadUrl(f.name)}
                          download={f.name.replace('.backup', '.zip')}
                          onClick={e => e.stopPropagation()}
                          className="text-xs px-2 py-1 rounded"
                          style={{ background: '#1a1a2a', color: '#8888ff', border: '1px solid #3a3a6a', textDecoration: 'none' }}>
                          ↓
                        </a>
                      </label>
                    ))}
                  </div>
                )}
              </Modal.Body>
              <Modal.Footer>
                <Button variant="tertiary" onPress={() => setShowRestore(false)} isDisabled={restoreRunning}>Cancel</Button>
                <Button variant="danger" isDisabled={!selectedRestoreFile || restoreRunning || backupFilesLoading}
                  onPress={async () => {
                    setRestoreRunning(true)
                    try {
                      const res = await api.battlegroup.restore(selectedRestoreFile)
                      toast.success('Restore completed')
                      setCmdOutput(res.output || '(done)')
                      setCmdDone(true)
                      setRunningCmd('Restore')
                      setShowRestore(false)
                    } catch (e: unknown) {
                      toast.danger(e instanceof Error ? e.message : String(e))
                    } finally {
                      setRestoreRunning(false)
                    }
                  }}>
                  {restoreRunning ? <Spinner size="sm" color="current" /> : `Restore ${selectedRestoreFile ? selectedRestoreFile.slice(-20) : ''}`}
                </Button>
              </Modal.Footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>

    </div>
  )
}
