import { useEffect } from 'react'
import type React from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import { toast } from '@/components/ui/toast'
import { api, ApiError } from '../api/client'
import type { BaseRow } from '../api/client'
import { qk } from '../api/queryKeys'
import { DataTable, Icon, PageHeader, type Column } from '../dune-ui'

type Key = 'id' | 'name' | 'pieces' | 'placeables' | 'actions'

interface BasesTabProps {
  isSignedIn?: boolean
}

export const BasesTab: React.FC<BasesTabProps> = ({ isSignedIn = true }) => {
  const { t } = useTranslation()

  const { data: bases = [], isFetching, error, refetch } = useQuery({
    queryKey: qk.bases.list,
    queryFn: api.bases.list,
    // A 404 is an expected "this control plane can't export bases" signal, not a
    // transient failure — don't retry it (keeps the unsupported card instant).
    retry: false,
  })

  // A 404 means the feature is unavailable on this control plane — show the
  // notice card. Any other failure is surfaced as a toast, once per error.
  const unsupported = error instanceof ApiError && error.status === 404
  useEffect(() => {
    if (error && !unsupported) {
      toast.danger(t('bases.failedToLoad', { message: error instanceof Error ? error.message : String(error) }))
    }
  }, [error, unsupported, t])

  const COLUMNS: Column<Key>[] = [
    { key: 'id', label: t('bases.columns.id'), width: 80 },
    { key: 'name', label: t('bases.columns.name'), minWidth: 220 },
    { key: 'pieces', label: t('bases.columns.pieces'), width: 100 },
    { key: 'placeables', label: t('bases.columns.placeables'), width: 110 },
    { key: 'actions', label: '', width: 120, sortable: false },
  ]

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
        <Button size="sm" variant="ghost" onClick={() => refetch()} disabled={isFetching}>
          {isFetching
            ? (
                <Spinner size="sm" color="current" />
              )
            : (
                <>
                  <Icon name="refresh-cw" />
                  {' '}
                  {t('common.refresh')}
                </>
              )}
        </Button>
      </PageHeader>

      {unsupported
        ? (
            <Card className="self-center max-w-sm">
              <CardHeader>
                <CardTitle className="text-accent-brand text-sm">{t('bases.featureNotAvailable')}</CardTitle>
              </CardHeader>
              <CardContent>
                <p className="text-xs text-muted text-center">
                  {t('bases.featureNotAvailableDesc')}
                </p>
              </CardContent>
            </Card>
          )
        : (
            <DataTable<BaseRow, Key>
              aria-label={t('bases.ariaLabel')}
              className="min-h-0 max-h-full"
              columns={COLUMNS}
              rows={bases}
              loading={isFetching}
              rowId={(b) => String(b.id)}
              initialSort={{ column: 'id', direction: 'ascending' }}
              sortValue={(b, k) => (k === 'actions' ? '' : (b as unknown as Record<string, string | number>)[k])}
              emptyState={<div className="py-8 text-center text-muted">{t('bases.noBasesFound')}</div>}
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
                          // asChild renders a single anchor styled as a button — no
                          // nested interactive (button-in-link) a11y violation.
                          <Button asChild size="sm" variant="outline" className="w-full">
                            <a href={api.bases.exportUrl(b.id)} download={b.name ? `${b.name}.json` : `base-${b.id}.json`}>
                              <Icon name="download" />
                              {' '}
                              {t('bases.export')}
                            </a>
                          </Button>
                        )
                      : (
                          <Button size="sm" variant="outline" className="w-full" disabled>
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
