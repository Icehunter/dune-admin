import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Spinner, toast } from '@heroui/react'
import { api, ApiError } from '../api/client'
import type { BaseRow } from '../api/client'
import { DataTable, Icon, PageHeader, type Column } from '../dune-ui'

type Key = 'id' | 'name' | 'pieces' | 'placeables' | 'actions'

export default function BasesTab({ isSignedIn = true }: { isSignedIn?: boolean }) {
  const { t } = useTranslation()
  const [bases, setBases] = useState<BaseRow[]>([])
  const [loading, setLoading] = useState(false)
  const [unsupported, setUnsupported] = useState(false)

  const COLUMNS: Column<Key>[] = [
    { key: 'id', label: t('bases.columns.id'), width: 80 },
    { key: 'name', label: t('bases.columns.name'), minWidth: 220 },
    { key: 'pieces', label: t('bases.columns.pieces'), width: 100 },
    { key: 'placeables', label: t('bases.columns.placeables'), width: 110 },
    { key: 'actions', label: '', width: 120, sortable: false },
  ]

  const load = useCallback(() => {
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
  }, [t])

  useEffect(() => {
    load()
  }, [load])

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
                <>
                  <Icon name="refresh-cw" />
                  {' '}
                  {t('common.refresh')}
                </>
              )}
        </Button>
      </PageHeader>

      {loading
        ? (
            <div className="flex justify-center py-12">
              <Spinner size="lg" />
            </div>
          )
        : unsupported
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
              <DataTable<BaseRow, Key>
                aria-label={t('bases.ariaLabel')}
                className="min-h-0 max-h-full"
                columns={COLUMNS}
                rows={bases}
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
