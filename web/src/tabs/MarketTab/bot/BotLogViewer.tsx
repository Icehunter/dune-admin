import { useState, useEffect, useRef, useCallback } from 'react'
import { Button, Checkbox } from '@heroui/react'
import { getWsBase, api } from '../../../api/client'
import { Icon } from '../../../dune-ui'

type Props = {
  active?: boolean
}

type ConnState = 'idle' | 'connecting' | 'connected' | 'error'

export default function BotLogViewer({ active = false }: Props) {
  const [connState, setConnState] = useState<ConnState>('idle')
  const [error, setError] = useState<string | null>(null)
  const [lines, setLines] = useState<string[]>([])
  const [autoScroll, setAutoScroll] = useState(true)
  const wsRef = useRef<WebSocket | null>(null)
  const bufRef = useRef<string[]>([])
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const containerRef = useRef<HTMLPreElement | null>(null)

  useEffect(() => {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight
    }
  }, [lines, autoScroll])

  const startFlush = useCallback(() => {
    if (timerRef.current) return
    timerRef.current = setInterval(() => {
      if (bufRef.current.length > 0) {
        setLines(prev => {
          const combined = [...prev, ...bufRef.current]
          bufRef.current = []
          return combined.length > 5000 ? combined.slice(-5000) : combined
        })
      }
    }, 200)
  }, [])

  const stopFlush = useCallback(() => {
    if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null }
  }, [])

  const connect = useCallback(async () => {
    if (wsRef.current) { wsRef.current.close(); wsRef.current = null }
    stopFlush()
    bufRef.current = []
    setLines([])
    setError(null)
    setConnState('connecting')

    try {
      const check = await api.marketBot.logsReady()
      if (!check.ready) {
        setError(check.reason ?? 'Log streaming not available')
        setConnState('error')
        return
      }
    } catch {
      setError('Could not reach backend — check that it is running.')
      setConnState('error')
      return
    }

    const ws = new WebSocket(`${getWsBase()}/market-bot/logs`)
    wsRef.current = ws

    ws.onopen = () => { setConnState('connected'); startFlush() }

    ws.onmessage = (e: MessageEvent) => { bufRef.current.push(e.data as string) }

    ws.onerror = () => {
      setError('WebSocket connection failed — the log stream was interrupted.')
      setConnState('error')
    }

    ws.onclose = (e) => {
      stopFlush()
      if (bufRef.current.length > 0) {
        setLines(prev => [...prev, ...bufRef.current])
        bufRef.current = []
      }
      if (connState !== 'error') {
        if (e.code !== 1000 && e.code !== 1001) {
          setError(`Connection closed (code ${e.code})${e.reason ? ': ' + e.reason : ''}`)
          setConnState('error')
        } else {
          setConnState('idle')
        }
      }
    }
  }, [startFlush, stopFlush, connState])

  const disconnect = useCallback(() => {
    if (wsRef.current) { wsRef.current.close(1000); wsRef.current = null }
    stopFlush()
    setConnState('idle')
    setError(null)
  }, [stopFlush])

  useEffect(() => {
    if (active) connect()
    else disconnect()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active])

  useEffect(() => () => { disconnect() }, [disconnect])

  const stateLabel = {
    idle: '○ disconnected',
    connecting: '◌ connecting…',
    connected: '● connected',
    error: '✕ error',
  }[connState]

  const stateColor = {
    idle: 'text-muted',
    connecting: 'text-muted animate-pulse',
    connected: 'text-success',
    error: 'text-danger',
  }[connState]

  return (
    <div className="flex flex-col gap-2 h-full min-h-0">
      <div className="flex items-center gap-2 shrink-0 flex-wrap">
        <span className={`text-xs font-mono ${stateColor}`}>{stateLabel}</span>
        <div className="flex-1" />
        <Checkbox isSelected={autoScroll} onChange={setAutoScroll}>Auto-scroll</Checkbox>
        {connState !== 'connected' ? (
          <Button size="sm" variant="outline" onPress={connect} isDisabled={connState === 'connecting'}>
            <Icon name="play" /> Connect
          </Button>
        ) : (
          <Button size="sm" variant="danger-soft" onPress={disconnect}>
            <Icon name="square" /> Stop
          </Button>
        )}
        {lines.length > 0 && (
          <Button size="sm" variant="ghost" onPress={() => { setLines([]); bufRef.current = [] }}>
            <Icon name="trash-2" /> Clear
          </Button>
        )}
      </div>

      {error && (
        <p className="text-xs text-danger bg-danger/10 border border-danger/20 rounded px-2 py-1.5 shrink-0">
          {error}
        </p>
      )}

      <pre
        ref={containerRef}
        className="flex-1 overflow-auto p-3 text-xs font-mono m-0 whitespace-pre-wrap break-all rounded-[var(--radius)] border border-border/60 bg-background text-success min-h-[200px]"
      >
        {lines.length === 0
          ? (connState === 'connected' ? 'Waiting for log lines…' : connState === 'connecting' ? 'Connecting…' : '')
          : lines.join('\n')}
      </pre>
    </div>
  )
}
