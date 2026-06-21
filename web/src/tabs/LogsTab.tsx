import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Chip, Spinner, Switch, toast } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { api, getWsBase } from '../api/client'
import type { LogPod, CheatEntry } from '../api/client'
import { DataTable, Icon, LoadingState, SideNav, type Column } from '../dune-ui'
import type { ActiveView, NavKey, CheatKey } from './types'
import type { LogsTabProps } from './interfaces'

export const LogsTab: React.FC<LogsTabProps> = ({ control }) => {
  const { t } = useTranslation()

  // Control planes that surface log files (amp, docker, local) get
  // file-oriented labels; kubectl keeps "Pods".
  const isFileBased = control === 'amp' || control === 'docker' || control === 'local'
  const sourceLabel = isFileBased ? t('logs.logFiles') : t('logs.pods')
  const itemLabel = isFileBased ? t('logs.logFileSingular') : t('logs.podSingular')

  const CHEAT_COLUMNS: Column<CheatKey>[] = [
    { key: 'time', label: t('logs.columns.time'), width: 180 },
    { key: 'character', label: t('logs.columns.character'), minWidth: 200 },
    { key: 'cheat_type', label: t('logs.columns.cheatType'), minWidth: 200 },
  ]

  const [pods, setPods] = React.useState<LogPod[]>([])
  const [podsLoading, setPodsLoading] = React.useState(false)
  const [selectedPod, setSelectedPod] = React.useState<LogPod | null>(null)
  const [connected, setConnected] = React.useState(false)
  const [autoScroll, setAutoScroll] = React.useState(true)
  const [displayLines, setDisplayLines] = React.useState<string[]>([])
  const [activeView, setActiveView] = React.useState<ActiveView>('pod')
  const [cheats, setCheats] = React.useState<CheatEntry[]>([])
  const [cheatsLoading, setCheatsLoading] = React.useState(false)

  const wsRef = React.useRef<WebSocket | null>(null)
  const linesRef = React.useRef<string[]>([])
  const flushTimerRef = React.useRef<ReturnType<typeof setInterval> | null>(null)
  const logContainerRef = React.useRef<HTMLPreElement | null>(null)

  const loadPods = (): void => {
    Promise.resolve()
      .then(() => setPodsLoading(true))
      .then(() => api.logs.pods())
      .then(setPods)
      .catch((e: unknown) => toast.danger(t('logs.failedToLoad', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setPodsLoading(false))
  }

  React.useEffect(() => {
    loadPods()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const startFlush = (): void => {
    if (flushTimerRef.current) return
    flushTimerRef.current = setInterval(() => {
      if (linesRef.current.length > 0) {
        setDisplayLines((prev) => {
          const combined = [...prev, ...linesRef.current]
          return combined.length > 5000 ? combined.slice(combined.length - 5000) : combined
        })
        linesRef.current = []
      }
    }, 200)
  }

  const stopFlush = (): void => {
    if (flushTimerRef.current) {
      clearInterval(flushTimerRef.current)
      flushTimerRef.current = null
    }
  }

  React.useEffect(() => {
    if (autoScroll && logContainerRef.current) {
      logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight
    }
  }, [displayLines, autoScroll])

  const connectPod = (pod: LogPod): void => {
    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }
    stopFlush()
    linesRef.current = []
    setDisplayLines([])
    setConnected(false)
    setSelectedPod(pod)
    setActiveView('pod')

    const url = `${getWsBase()}/logs/stream?ns=${encodeURIComponent(pod.namespace)}&pod=${encodeURIComponent(pod.name)}`
    const ws = new WebSocket(url)
    wsRef.current = ws
    ws.onopen = () => {
      setConnected(true)
      startFlush()
    }
    ws.onmessage = (event: MessageEvent) => {
      linesRef.current.push(event.data as string)
    }
    ws.onerror = () => {
      toast.danger(t('logs.wsError'))
    }
    ws.onclose = () => {
      setConnected(false)
      stopFlush()
      if (linesRef.current.length > 0) {
        setDisplayLines((prev) => [...prev, ...linesRef.current])
        linesRef.current = []
      }
    }
  }

  const disconnect = (): void => {
    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }
    stopFlush()
    setConnected(false)
  }

  React.useEffect(() => () => {
    disconnect()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

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
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
    finally {
      setCheatsLoading(false)
    }
  }

  const navItems = [
    { key: 'cheats' as NavKey, label: t('logs.cheats7d'), sublabel: t('logs.antiCheatLog') },
    ...pods.map((p) => ({
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
    }
    else {
      const id = key.slice(4) // strip "pod:"
      const pod = pods.find((p) => `${p.namespace}/${p.name}` === id)
      if (pod) connectPod(pod)
    }
  }

  return (
    <div className="flex h-full gap-3 min-h-0">
      <SideNav
        items={navItems}
        active={activeKey}
        onSelect={handleNavSelect}
        title={t('logs.sourceTitle', { label: sourceLabel, count: pods.length })}
        titleAction={(
          <Button size="sm" variant="ghost" isDisabled={podsLoading} onPress={loadPods}>
            {podsLoading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
          </Button>
        )}
      />

      <div className="flex-1 flex flex-col overflow-hidden gap-3 min-h-0">
        {activeView === 'cheats'
          ? (
              <React.Fragment>
                <div className="flex items-center gap-3 shrink-0">
                  <h3 className="text-base font-semibold text-accent flex-1">{t('logs.antiCheatTitle')}</h3>
                  <span className="text-xs text-muted">
                    {t('logs.eventsCount', { count: cheats.length })}
                  </span>
                  <Button size="sm" variant="outline" onPress={loadCheats} isDisabled={cheatsLoading}>
                    {cheatsLoading
                      ? <Spinner size="sm" color="current" />
                      : (
                          <React.Fragment>
                            <Icon name="refresh-cw" />
                            {' '}
                            {t('common.refresh')}
                          </React.Fragment>
                        )}
                  </Button>
                </div>

                {cheatsLoading
                  ? (
                      <LoadingState />
                    )
                  : (
                      <DataTable<CheatEntry, CheatKey>
                        aria-label={t('logs.antiCheatLabel')}
                        className="min-h-0 max-h-full"
                        columns={CHEAT_COLUMNS}
                        rows={cheats}
                        rowId={(c) => `${c.fls_id}-${c.event_time}-${c.cheat_type}`}
                        initialSort={{ column: 'time', direction: 'descending' }}
                        sortValue={(c, k) => {
                          if (k === 'time') return c.event_time
                          if (k === 'character') return c.character_name
                          return c.cheat_type
                        }}
                        emptyState={(
                          <EmptyState size="sm">
                            <EmptyState.Header>
                              <EmptyState.Media variant="icon">
                                <IconifyIcon icon="gravity-ui:document" className="size-5" />
                              </EmptyState.Media>
                              <EmptyState.Title>{t('logs.noCheatEvents')}</EmptyState.Title>
                            </EmptyState.Header>
                          </EmptyState>
                        )}
                        renderCell={(c, key) => {
                          switch (key) {
                            case 'time': return <span className="font-mono text-muted">{c.event_time}</span>
                            case 'character': return c.character_name
                            case 'cheat_type': {
                              const suspicious = /dup|negative/i.test(c.cheat_type)
                              return (
                                <Chip size="sm" color={suspicious ? 'danger' : 'default'} variant="soft">
                                  {c.cheat_type}
                                </Chip>
                              )
                            }
                          }
                        }}
                      />
                    )}
              </React.Fragment>
            )
          : (
              <React.Fragment>
                <div className="flex items-center gap-3 shrink-0">
                  <Chip
                    size="sm"
                    color={connected ? 'success' : 'default'}
                    variant="soft"
                  >
                    {connected
                      ? t('logs.connectedPod', { pod: selectedPod?.name })
                      : selectedPod
                        ? t('logs.disconnected')
                        : t('logs.selectSource', { label: itemLabel })}
                  </Chip>
                  <div className="flex-1" />
                  <Switch isSelected={autoScroll} onChange={setAutoScroll} size="sm">
                    <Switch.Control><Switch.Thumb /></Switch.Control>
                    <Switch.Content>{t('logs.autoScroll')}</Switch.Content>
                  </Switch>
                  {selectedPod && connected && (
                    <Button size="sm" variant="danger-soft" onPress={disconnect}>
                      <Icon name="square" />
                      {' '}
                      {t('logs.stop')}
                    </Button>
                  )}
                  {selectedPod && !connected && (
                    <Button size="sm" variant="outline" onPress={() => connectPod(selectedPod)}>
                      <Icon name="play" />
                      {' '}
                      {t('logs.reconnect')}
                    </Button>
                  )}
                  {displayLines.length > 0 && (
                    <Button size="sm" variant="ghost" onPress={exportLogs}>
                      <Icon name="download" />
                      {' '}
                      {t('common.export')}
                    </Button>
                  )}
                  {displayLines.length > 0 && (
                    <Button
                      size="sm"
                      variant="ghost"
                      onPress={() => {
                        setDisplayLines([])
                        linesRef.current = []
                      }}
                    >
                      <Icon name="trash-2" />
                      {' '}
                      {t('logs.clear')}
                    </Button>
                  )}
                  <span className="text-xs text-muted">
                    {t('logs.linesCount', { count: displayLines.length })}
                  </span>
                </div>

                <pre
                  ref={logContainerRef}
                  className="flex-1 overflow-auto p-4 text-xs font-mono m-0 whitespace-pre-wrap break-all rounded-[var(--radius)] border border-border/60 bg-background text-success"
                >
                  {displayLines.length === 0
                    ? (selectedPod
                        ? (connected ? t('logs.waitingForLines') : t('logs.disconnectedState'))
                        : t('logs.selectFromPanel', { label: itemLabel }))
                    : displayLines.join('\n')}
                </pre>
              </React.Fragment>
            )}
      </div>
    </div>
  )
}
