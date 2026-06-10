import type React from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@heroui/react'
import { Icon, Panel } from '../dune-ui'
import { currentBackendBase } from '../api/client'

// BackendUnreachable is shown when the SPA loaded but could never reach the
// dune-admin backend API (#165) — instead of an empty, non-working dashboard.
// It surfaces the backend target it's trying and how to fix a connection issue.
export const BackendUnreachable: React.FC<{ onRetry: () => void }> = ({ onRetry }) => {
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-center min-h-screen bg-background p-6">
      <Panel className="max-w-lg w-full flex flex-col items-center gap-4 py-8 text-center">
        <Icon name="triangle-alert" className="text-warning" />
        <h1 className="text-lg font-semibold text-foreground">{t('app.backendUnreachable.title')}</h1>
        <p className="text-sm text-muted">{t('app.backendUnreachable.body')}</p>
        <div className="text-xs text-muted flex flex-col items-center gap-0.5">
          <span>{t('app.backendUnreachable.targetLabel')}</span>
          <span className="font-mono text-foreground break-all">{currentBackendBase()}</span>
        </div>
        <ul className="text-sm text-muted text-left list-disc pl-5 space-y-1">
          <li>{t('app.backendUnreachable.hint1')}</li>
          <li>{t('app.backendUnreachable.hint2')}</li>
          <li>{t('app.backendUnreachable.hint3')}</li>
        </ul>
        <Button size="sm" onPress={onRetry}>
          <Icon name="refresh-cw" />
          {' '}
          {t('app.backendUnreachable.retry')}
        </Button>
      </Panel>
    </div>
  )
}
