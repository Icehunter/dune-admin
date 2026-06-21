import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Spinner, toast } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { api, ApiError } from '../api/client'
import type { BaseRow } from '../api/client'
import { DataTable, Icon, PageHeader, type Column } from '../dune-ui'
import { usePermissions } from '../hooks/usePermissions'
import type { BasesTabKey } from './types'
import type { BasesTabProps } from './interfaces'

export const BasesTab: React.FC<BasesTabProps> = ({ isSignedIn = true }) => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const canExportData = can('data:export')
  const [bases, setBases] = React.useState<BaseRow[]>([])
  const [loading, setLoading] = React.useState(false)
  const [unsupported, setUnsupported] = React.useState(false)

  const COLUMNS: Column<BasesTabKey>[] = [
    { key: 'id', label: t('bases.columns.id'), width: 80 },
    { key: 'name', label: t('bases.columns.name'), minWidth: 220 },
    { key: 'pieces', label: t('bases.columns.pieces'), width: 100 },
    { key: 'placeables', label: t('bases.columns.placeables'), width: 110 },
    { key: 'actions', label: '', width: 120, sortable: false },
  ]

  const load = (): void => {
    Promise.resolve()
      .then(() => {
        setLoading(true)
        setUnsupported(false)
      })
      .then(() => api.bases.list())
      .then(setBases)
      .catch((e: unknown) => {
        if (e instanceof ApiError && e.status === 404) setUnsupported(true)
        else toast.danger(t('bases.failedToLoad', { message: e instanceof Error ? e.message : String(e) }))
      })
      .finally(() => setLoading(false))
  }

  React.useEffect(() => {
    load()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      {!isSignedIn && (
        <div className="shrink-0 rounded-[var(--radius)] px-4 py-2 text-xs font-medium bg-danger/10 border border-danger/40 text-danger flex items-center gap-2">
          <Icon name="triangle-alert" />
          <span>
            A
            {' '}
            <strong>{t('bases.layoutAccountStrong')}</strong>
            {' '}
            account is required to export bases. Sign in using the button in the top
            right.
          </span>
        </div>
      )}

      <PageHeader
        title={t('bases.title', { count: bases.length })}
        subtitle={t('bases.subtitle')}
      >
        <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
          {loading
            ? (
                <Spinner size="sm" color="current" />
              )
            : (
                <React.Fragment>
                  <Icon name="refresh-cw" />
                  {' '}
                  {t('common.refresh')}
                </React.Fragment>
              )}
        </Button>
      </PageHeader>

      {unsupported
        ? (
            <Card className="self-center max-w-sm">
              <Card.Header>
                <Card.Title className="text-accent text-sm">{t('bases.featureNotAvailable')}</Card.Title>
              </Card.Header>
              <Card.Content>
                <p className="text-xs text-muted text-center">
                  {t('bases.featureNotAvailableDesc')}
                </p>
              </Card.Content>
            </Card>
          )
        : (
            <DataTable<BaseRow, BasesTabKey>
              aria-label={t('bases.ariaLabel')}
              className="min-h-0 max-h-full"
              columns={canExportData ? COLUMNS : COLUMNS.filter((c) => c.key !== 'actions')}
              rows={bases}
              loading={loading}
              rowId={(b) => String(b.id)}
              initialSort={{ column: 'id', direction: 'ascending' }}
              sortValue={(b, k) => (k === 'actions' ? '' : (b as unknown as Record<string, string | number>)[k])}
              emptyState={(
                <EmptyState size="sm">
                  <EmptyState.Header>
                    <EmptyState.Title>{t('bases.noBasesFound')}</EmptyState.Title>
                  </EmptyState.Header>
                </EmptyState>
              )}
              renderCell={(b, key) => {
                switch (key) {
                  case 'id':
                    return <span className="font-mono text-muted">{b.id}</span>
                  case 'name':
                    return b.name || <span className="text-muted">—</span>
                  case 'pieces':
                    return <span className="text-muted">{b.pieces}</span>
                  case 'placeables':
                    return <span className="text-muted">{b.placeables}</span>
                  case 'actions':
                    return isSignedIn
                      ? (
                          <a href={api.bases.exportUrl(b.id)} download={b.name ? `${b.name}.json` : `base-${b.id}.json`}>
                            <Button size="sm" variant="outline" className="w-full">
                              <Icon name="download" />
                              {' '}
                              {t('bases.export')}
                            </Button>
                          </a>
                        )
                      : (
                          <Button size="sm" variant="outline" className="w-full" isDisabled>
                            <Icon name="download" />
                            {' '}
                            {t('bases.export')}
                          </Button>
                        )
                }
              }}
            />
          )}
    </div>
  )
}
