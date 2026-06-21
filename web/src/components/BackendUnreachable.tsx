import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@heroui/react'
import { Icon, Panel, FieldInput } from '../dune-ui'
import { currentBackendBase } from '../api/client'
import type { BackendUnreachableProps } from './interfaces'

// BackendUnreachable is shown when the SPA loaded but could never reach the
// dune-admin backend (#165). Includes an inline URL override so users can fix
// connection issues without needing to reach the Settings tab (#182).
export const BackendUnreachable: React.FC<BackendUnreachableProps> = ({ onRetry }) => {
  const { t } = useTranslation()
  const [url, setUrl] = React.useState(() => localStorage.getItem('dune_admin_backend') || '')

  const saveAndRetry = () => {
    const trimmed = url.trim()
    if (trimmed) {
      localStorage.setItem('dune_admin_backend', trimmed)
    }
    else {
      localStorage.removeItem('dune_admin_backend')
    }
    onRetry()
  }

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
        </ul>
        <div className="w-full flex flex-col gap-2 pt-2">
          <p className="text-xs text-muted text-left">{t('app.backendUnreachable.urlLabel')}</p>
          <div className="flex gap-2">
            <FieldInput
              value={url}
              onChange={setUrl}
              placeholder="http://host:8080"
              aria-label={t('app.backendUnreachable.urlLabel')}
              className="font-mono flex-1"
            />
            <Button size="sm" onPress={saveAndRetry}>
              <Icon name="refresh-cw" />
              {' '}
              {t('app.backendUnreachable.retry')}
            </Button>
          </div>
        </div>
      </Panel>
    </div>
  )
}
