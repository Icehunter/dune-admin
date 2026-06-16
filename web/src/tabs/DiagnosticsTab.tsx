import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Chip, Switch, toast } from '@heroui/react'
import { api } from '../api/client'
import type { DiagnosticsEnvironment } from '../api/client'
import { Icon, PageHeader, Panel } from '../dune-ui'

const MAX_LINES = 5000

export const DiagnosticsTab: React.FC = () => {
  const { t } = useTranslation()
  const [env, setEnv] = React.useState<DiagnosticsEnvironment | null>(null)
  const [loading, setLoading] = React.useState(false)
  const [lines, setLines] = React.useState<string[]>([])
  const [autoScroll, setAutoScroll] = React.useState(true)
  const [connected, setConnected] = React.useState(false)

  const bufRef = React.useRef<string[]>([])
  const timerRef = React.useRef<ReturnType<typeof setInterval> | null>(null)
  const preRef = React.useRef<HTMLPreElement | null>(null)

  const loadEnv = React.useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.diagnostics.environment())
      .then(setEnv)
      .catch((e: unknown) => toast.danger(t('diagnostics.failedToLoad', { message: e instanceof Error ? e.message : String(e) })))
      .finally(() => setLoading(false))
  }, [t])

  React.useEffect(() => {
    loadEnv()
    const ws = new WebSocket(api.diagnostics.streamUrl())
    ws.onopen = () => setConnected(true)
    ws.onclose = () => setConnected(false)
    ws.onmessage = (ev: MessageEvent) => {
      bufRef.current.push(ev.data as string)
    }
    timerRef.current = setInterval(() => {
      if (bufRef.current.length === 0) return
      setLines((prev) => {
        const next = [...prev, ...bufRef.current]
        bufRef.current = []
        return next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next
      })
    }, 200)
    return () => {
      if (timerRef.current) {
        clearInterval(timerRef.current)
        timerRef.current = null
      }
      ws.close()
    }
  }, [loadEnv])

  React.useEffect(() => {
    if (autoScroll && preRef.current) {
      preRef.current.scrollTop = preRef.current.scrollHeight
    }
  }, [lines, autoScroll])

  const reportIssue = () => {
    Promise.resolve()
      .then(() => api.diagnostics.report())
      .then((rep) => {
        const url = `https://github.com/${rep.repo}/issues/new?title=${encodeURIComponent(rep.title)}&body=${encodeURIComponent(rep.body)}`
        window.open(url, '_blank', 'noopener,noreferrer')
        window.location.assign(api.diagnostics.bundleUrl())
      })
      .catch((e: unknown) => toast.danger(t('diagnostics.failedToLoad', { message: e instanceof Error ? e.message : String(e) })))
  }

  return (
    <Panel>
      <PageHeader
        title={t('diagnostics.title', 'Diagnostics')}
        subtitle={t('diagnostics.subtitle', 'Runtime environment and live server logs')}
        onRefresh={loadEnv}
        loading={loading}
      />

      {env && (
        <div className="grid grid-cols-2 gap-3 md:grid-cols-4 py-2">
          <div className="rounded-[var(--radius)] border border-border bg-surface p-3">
            <div className="text-muted text-xs">{t('diagnostics.version', 'Version')}</div>
            <div className="text-foreground font-mono">{env.version}</div>
          </div>
          <div className="rounded-[var(--radius)] border border-border bg-surface p-3">
            <div className="text-muted text-xs">{t('diagnostics.controlPlane', 'Control plane')}</div>
            <div className="text-foreground">{env.control_plane}</div>
          </div>
          <div className="rounded-[var(--radius)] border border-border bg-surface p-3">
            <div className="text-muted text-xs">{t('diagnostics.osArch', 'OS / Arch')}</div>
            <div className="text-foreground">
              {env.os}
              /
              {env.arch}
            </div>
          </div>
          <div className="rounded-[var(--radius)] border border-border bg-surface p-3">
            <div className="text-muted text-xs">{t('diagnostics.activeServers', 'Active servers')}</div>
            <div className="text-foreground">{env.active_server_count}</div>
          </div>
        </div>
      )}

      <div className="flex items-center gap-3 py-2 shrink-0">
        <Chip size="sm" color={connected ? 'success' : 'default'} variant="soft">
          {connected ? t('diagnostics.live', 'Live') : t('diagnostics.disconnected', 'Disconnected')}
        </Chip>
        <div className="flex-1" />
        <Switch isSelected={autoScroll} onChange={setAutoScroll} size="sm">
          <Switch.Control><Switch.Thumb /></Switch.Control>
          <Switch.Content>{t('logs.autoScroll', 'Auto-scroll')}</Switch.Content>
        </Switch>
        <Button size="sm" variant="ghost" onPress={() => setLines([])}>
          <Icon name="trash-2" />
          {' '}
          {t('logs.clear', 'Clear')}
        </Button>
        <Button size="sm" variant="primary" onPress={reportIssue}>
          <Icon name="bug" />
          {' '}
          {t('diagnostics.reportIssue', 'Report an issue')}
        </Button>
      </div>

      <pre
        ref={preRef}
        className="h-[60vh] overflow-auto p-4 m-0 text-xs font-mono whitespace-pre-wrap break-all rounded-[var(--radius)] border border-border bg-surface text-foreground"
      >
        {lines.join('\n')}
      </pre>
    </Panel>
  )
}
