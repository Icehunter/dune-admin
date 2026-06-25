import * as React from 'react'
import { Button, Switch } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { getWsBase, api } from '../../../api/client'
import { Icon } from '../../../dune-ui'
import type { BotLogViewerProps, ConnState } from './types'

export const BotLogViewer: React.FC<BotLogViewerProps> = ({ active = false }) => {
  const { t } = useTranslation()
  const [connState, setConnState] = React.useState<ConnState>('idle')
  const [error, setError] = React.useState<string | null>(null)
  const [lines, setLines] = React.useState<string[]>([])
  const [autoScroll, setAutoScroll] = React.useState(true)
  const wsRef = React.useRef<WebSocket | null>(null)
  const bufRef = React.useRef<string[]>([])
  const timerRef = React.useRef<ReturnType<typeof setInterval> | null>(null)
  const containerRef = React.useRef<HTMLPreElement | null>(null)

  React.useEffect(() => {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight
    }
  }, [lines, autoScroll])

  const startFlush = (): void => {
    if (timerRef.current) return
    timerRef.current = setInterval(() => {
      if (bufRef.current.length > 0) {
        setLines((prev) => {
          const combined = [...prev, ...bufRef.current]
          bufRef.current = []
          return combined.length > 5000 ? combined.slice(-5000) : combined
        })
      }
    }, 200)
  }

  const stopFlush = (): void => {
    if (timerRef.current) {
      clearInterval(timerRef.current)
      timerRef.current = null
    }
  }

  const connect = (): void => {
    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }
    stopFlush()
    bufRef.current = []
    Promise.resolve()
      .then(() => {
        setLines([])
        setError(null)
        setConnState('connecting')
      })
      .then(() => api.marketBot.logsReady())
      .then((check) => {
        if (!check.ready) {
          setError(check.reason ?? t('market.bot.log.notAvailable'))
          setConnState('error')
          return
        }
        const ws = new WebSocket(`${getWsBase()}/market-bot/logs`)
        wsRef.current = ws
        ws.onopen = () => {
          setConnState('connected')
          startFlush()
        }
        ws.onmessage = (e: MessageEvent) => {
          bufRef.current.push(e.data as string)
        }
        ws.onerror = () => {
          setError(t('market.bot.log.wsError'))
          setConnState('error')
        }
        ws.onclose = (e) => {
          stopFlush()
          if (bufRef.current.length > 0) {
            setLines((prev) => [...prev, ...bufRef.current])
            bufRef.current = []
          }
          if (e.code !== 1000 && e.code !== 1001) {
            setError(e.reason
              ? t('market.bot.log.connClosedReason', { code: e.code, reason: e.reason })
              : t('market.bot.log.connClosed', { code: e.code }))
            setConnState('error')
          }
          else {
            setConnState('idle')
          }
        }
      })
      .catch(() => {
        setError(t('market.bot.log.backendUnreachable'))
        setConnState('error')
      })
  }

  const disconnect = (): void => {
    if (wsRef.current) {
      wsRef.current.close(1000)
      wsRef.current = null
    }
    stopFlush()
    Promise.resolve().then(() => {
      setConnState('idle')
      setError(null)
    })
  }

  React.useEffect(() => {
    if (active) void connect()
    else disconnect()
  }, [active]) // eslint-disable-line react-hooks/exhaustive-deps

  React.useEffect(() => () => {
    disconnect()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const stateLabel = {
    idle: t('market.bot.log.idle'),
    connecting: t('market.bot.log.connecting'),
    connected: t('market.bot.log.connected'),
    error: t('market.bot.log.error'),
  }[connState]

  const stateColor = {
    idle: 'text-muted',
    connecting: 'text-muted animate-pulse',
    connected: 'text-success',
    error: 'text-danger',
  }[connState]

  const clearLog = () => {
    setLines([])
    bufRef.current = []
  }

  return (
    <div className="flex flex-col gap-2 h-full min-h-0">
      <div className="flex items-center gap-2 shrink-0 flex-wrap">
        <span className={`text-xs font-mono ${stateColor}`}>{stateLabel}</span>
        <div className="flex-1" />
        <Switch isSelected={autoScroll} onChange={setAutoScroll} size="sm">
          <Switch.Content>
            <Switch.Control><Switch.Thumb /></Switch.Control>
            {t('market.bot.log.autoScroll')}
          </Switch.Content>
        </Switch>
        {connState !== 'connected'
          ? (
              <Button size="sm" variant="outline" onPress={connect} isDisabled={connState === 'connecting'}>
                <Icon name="play" />
                {' '}
                {t('market.bot.log.connect')}
              </Button>
            )
          : (
              <Button size="sm" variant="danger-soft" onPress={disconnect}>
                <Icon name="square" />
                {' '}
                {t('market.bot.log.stop')}
              </Button>
            )}
        {lines.length > 0 && (
          <Button size="sm" variant="ghost" onPress={clearLog}>
            <Icon name="trash-2" />
            {' '}
            {t('market.bot.log.clear')}
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
        className="flex-1 overflow-auto p-3 text-xs font-mono m-0 whitespace-pre-wrap break-all rounded-[var(--radius)] border border-border/60 bg-background text-success"
      >
        {lines.length === 0
          ? (connState === 'connected' ? t('market.bot.log.waitingForLines') : connState === 'connecting' ? t('market.bot.log.connectingState') : '')
          : lines.join('\n')}
      </pre>
    </div>
  )
}
