import { useState, useEffect, useRef, useCallback } from 'react'
import { Button, Checkbox, Chip, Spinner, toast } from '@heroui/react'
import { api, getWsBase } from '../api/client'
import type { LogPod, CheatEntry } from '../api/client'
import { DataTable, Icon, SideNav, type Column } from '../dune-ui'
import { useStatus } from '../hooks/useStatus'

type ActiveView = 'pod' | 'cheats'
type NavKey = 'cheats' | `pod:${string}`

type CheatKey = 'time' | 'character' | 'cheat_type'

const CHEAT_COLUMNS: Column<CheatKey>[] = [
  { key: 'time',       label: 'Time',       width: 180 },
  { key: 'character',  label: 'Character',  minWidth: 200 },
  { key: 'cheat_type', label: 'Cheat Type', minWidth: 200 },
]

export default function LogsTab() {
  // Control planes that surface log files (amp, docker, local) get
  // file-oriented labels; kubectl keeps "Pods".
  const control = useStatus()?.control
  const isFileBased = control === 'amp' || control === 'docker' || control === 'local'
  const sourceLabel = isFileBased ? 'Log Files' : 'Pods'
  const itemLabel = isFileBased ? 'log file' : 'pod'

  const [pods, setPods] = useState<LogPod[]>([])
  const [podsLoading, setPodsLoading] = useState(false)
  const [selectedPod, setSelectedPod] = useState<LogPod | null>(null)
  const [connected, setConnected] = useState(false)
  const [autoScroll, setAutoScroll] = useState(true)
  const [displayLines, setDisplayLines] = useState<string[]>([])
  const [activeView, setActiveView] = useState<ActiveView>('pod')
  const [cheats, setCheats] = useState<CheatEntry[]>([])
  const [cheatsLoading, setCheatsLoading] = useState(false)

  const wsRef = useRef<WebSocket | null>(null)
  const linesRef = useRef<string[]>([])
  const flushTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const logContainerRef = useRef<HTMLPreElement | null>(null)

  const loadPods = () => {
    setPodsLoading(true)
    api.logs.pods()
      .then(setPods)
      .catch((e: unknown) => toast.danger(`Failed to load pods: ${e instanceof Error ? e.message : String(e)}`))
      .finally(() => setPodsLoading(false))
  }

  useEffect(() => { loadPods() }, [])

  const startFlush = useCallback(() => {
    if (flushTimerRef.current) return
    flushTimerRef.current = setInterval(() => {
      if (linesRef.current.length > 0) {
        setDisplayLines(prev => {
          const combined = [...prev, ...linesRef.current]
          return combined.length > 5000 ? combined.slice(combined.length - 5000) : combined
        })
        linesRef.current = []
      }
    }, 200)
  }, [])

  const stopFlush = useCallback(() => {
    if (flushTimerRef.current) {
      clearInterval(flushTimerRef.current)
      flushTimerRef.current = null
    }
  }, [])

  useEffect(() => {
    if (autoScroll && logContainerRef.current) {
      logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight
    }
  }, [displayLines, autoScroll])

  const connectPod = useCallback((pod: LogPod) => {
    if (wsRef.current) { wsRef.current.close(); wsRef.current = null }
    stopFlush()
    linesRef.current = []
    setDisplayLines([])
    setConnected(false)
    setSelectedPod(pod)
    setActiveView('pod')

    const url = `${getWsBase()}/logs/stream?ns=${encodeURIComponent(pod.namespace)}&pod=${encodeURIComponent(pod.name)}`
    const ws = new WebSocket(url)
    wsRef.current = ws
    ws.onopen = () => { setConnected(true); startFlush() }
    ws.onmessage = (event: MessageEvent) => { linesRef.current.push(event.data as string) }
    ws.onerror = () => { toast.danger('WebSocket error') }
    ws.onclose = () => {
      setConnected(false)
      stopFlush()
      if (linesRef.current.length > 0) {
        setDisplayLines(prev => [...prev, ...linesRef.current])
        linesRef.current = []
      }
    }
  }, [startFlush, stopFlush])

  const disconnect = useCallback(() => {
    if (wsRef.current) { wsRef.current.close(); wsRef.current = null }
    stopFlush()
    setConnected(false)
  }, [stopFlush])

  useEffect(() => () => { disconnect() }, [disconnect])

  const exportLogs = () => {
    const blob = new Blob([displayLines.join('\n')], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${selectedPod?.name ?? 'logs'}-${new Date().toISOString()}.txt`
    a.click()
    URL.revokeObjectURL(url)
  }

  const loadCheats = async () => {
    setCheatsLoading(true)
    try {
      setCheats(await api.logs.cheats())
    } catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    } finally {
      setCheatsLoading(false)
    }
  }

  const navItems = [
    { key: 'cheats' as NavKey, label: 'Cheats (7d)', sublabel: 'Anti-cheat log' },
    ...pods.map(p => ({
      key: `pod:${p.namespace}/${p.name}` as NavKey,
      label: <span className="font-mono">{p.name}</span>,
      sublabel: p.namespace,
    })),
  ]
  const activeKey: NavKey | null = activeView === 'cheats'
    ? 'cheats'
    : selectedPod ? `pod:${selectedPod.namespace}/${selectedPod.name}` : null

  const handleNavSelect = (key: NavKey) => {
    if (key === 'cheats') {
      setSelectedPod(null)
      setActiveView('cheats')
      loadCheats()
    } else {
      const id = key.slice(4) // strip "pod:"
      const pod = pods.find(p => `${p.namespace}/${p.name}` === id)
      if (pod) connectPod(pod)
    }
  }

  return (
    <div className="flex h-full gap-3 min-h-0">
      <SideNav
        items={navItems}
        active={activeKey}
        onSelect={handleNavSelect}
        title={`${sourceLabel} (${pods.length})`}
        titleAction={
          <Button size="sm" variant="ghost" isDisabled={podsLoading} onPress={loadPods}>
            {podsLoading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
          </Button>
        }
      />

      <div className="flex-1 flex flex-col overflow-hidden gap-3 min-h-0">
        {activeView === 'cheats' ? (
          <>
            <div className="flex items-center gap-3 shrink-0">
              <h3 className="text-base font-semibold text-accent flex-1">Anti-Cheat Events (7d)</h3>
              <span className="text-xs text-muted">{cheats.length} events</span>
              <Button size="sm" variant="outline" onPress={loadCheats} isDisabled={cheatsLoading}>
                {cheatsLoading ? <Spinner size="sm" color="current" /> : <><Icon name="refresh-cw" /> Refresh</>}
              </Button>
            </div>

            {cheatsLoading ? (
              <div className="flex justify-center py-12"><Spinner size="lg" /></div>
            ) : (
              <DataTable<CheatEntry, CheatKey>
                aria-label="Anti-cheat events"
                className="min-h-0 max-h-full"
                columns={CHEAT_COLUMNS}
                rows={cheats}
                rowId={c => `${c.fls_id}-${c.event_time}-${c.cheat_type}`}
                initialSort={{ column: 'time', direction: 'descending' }}
                sortValue={(c, k) => {
                  if (k === 'time')      return c.event_time
                  if (k === 'character') return c.character_name
                  return c.cheat_type
                }}
                emptyState={<div className="py-8 text-center text-muted">No cheat events found in the last 7 days.</div>}
                renderCell={(c, key) => {
                  switch (key) {
                    case 'time':      return <span className="font-mono text-muted">{c.event_time}</span>
                    case 'character': return c.character_name
                    case 'cheat_type':
                      const suspicious = /dup|negative/i.test(c.cheat_type)
                      return (
                        <Chip size="sm" color={suspicious ? 'danger' : 'default'} variant="soft">
                          {c.cheat_type}
                        </Chip>
                      )
                  }
                }}
              />
            )}
          </>
        ) : (
          <>
            <div className="flex items-center gap-3 shrink-0">
              <Chip
                size="sm"
                color={connected ? 'success' : 'default'}
                variant="soft"
              >
                {connected ? `● Connected · ${selectedPod?.name}` : selectedPod ? '○ Disconnected' : `○ Select a ${itemLabel}`}
              </Chip>
              <div className="flex-1" />
              <Checkbox isSelected={autoScroll} onChange={setAutoScroll}>Auto-scroll</Checkbox>
              {selectedPod && connected && (
                <Button size="sm" variant="danger-soft" onPress={disconnect}>
                  <Icon name="square" /> Stop
                </Button>
              )}
              {selectedPod && !connected && (
                <Button size="sm" variant="outline" onPress={() => connectPod(selectedPod)}>
                  <Icon name="play" /> Reconnect
                </Button>
              )}
              {displayLines.length > 0 && (
                <Button size="sm" variant="ghost" onPress={exportLogs}>
                  <Icon name="download" /> Export
                </Button>
              )}
              {displayLines.length > 0 && (
                <Button size="sm" variant="ghost" onPress={() => { setDisplayLines([]); linesRef.current = [] }}>
                  <Icon name="trash-2" /> Clear
                </Button>
              )}
              <span className="text-xs text-muted">{displayLines.length} lines</span>
            </div>

            <pre
              ref={logContainerRef}
              className="flex-1 overflow-auto p-4 text-xs font-mono m-0 whitespace-pre-wrap break-all rounded-[var(--radius)] border border-border/60 bg-background text-success"
            >
              {displayLines.length === 0
                ? (selectedPod
                    ? (connected ? 'Waiting for log lines...' : 'Disconnected.')
                    : `Select a ${itemLabel} from the left panel to start streaming logs.`)
                : displayLines.join('\n')}
            </pre>
          </>
        )}
      </div>
    </div>
  )
}
