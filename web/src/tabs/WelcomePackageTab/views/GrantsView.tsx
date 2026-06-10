import type React from 'react'
import { Button, Spinner } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { useTranslation } from 'react-i18next'
import { DataTable, Icon, PageHeader, type Column } from '../../../dune-ui'
import type { WelcomeGrantRecord } from '../../../api/client'
import type { WelcomeSharedProps } from '../types'

type GrantKey = 'character' | 'fls' | 'version' | 'status' | 'attempts' | 'updated' | 'error' | 'actions'

type GrantsViewProps = Pick<WelcomeSharedProps, 'grants' | 'retry' | 'revoke' | 'load' | 'loading'>

function fmtTime(s: string): string {
  if (!s) return '—'
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString()
}

export const GrantsView: React.FC<GrantsViewProps> = ({ grants, retry, revoke, load, loading }) => {
  const { t } = useTranslation()

  const GRANT_COLUMNS: Column<GrantKey>[] = [
    { key: 'character', label: t('welcome.columns.character'), minWidth: 130 },
    { key: 'fls', label: t('welcome.columns.flsId'), minWidth: 140 },
    { key: 'version', label: t('welcome.columns.version'), width: 90 },
    { key: 'status', label: t('welcome.columns.status'), width: 90 },
    { key: 'attempts', label: t('welcome.columns.tries'), width: 60 },
    { key: 'updated', label: t('welcome.columns.updated'), minWidth: 150 },
    { key: 'error', label: t('welcome.columns.error'), minWidth: 180 },
    { key: 'actions', label: '', width: 100, sortable: false },
  ]

  return (
    <div className="flex flex-col h-full min-h-0 gap-3">
      <PageHeader
        title={t('welcome.grantsTitle', { count: grants.length })}
        subtitle={t('welcome.grantsLabel')}
      >
        <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
          {loading
            ? <Spinner size="sm" color="current" />
            : (
                <>
                  <Icon name="refresh-cw" />
                  {' '}
                  {t('common.refresh')}
                </>
              )}
        </Button>
      </PageHeader>
      <DataTable<WelcomeGrantRecord, GrantKey>
        aria-label={t('welcome.grantsLabel')}
        columns={GRANT_COLUMNS}
        rows={grants}
        rowId={(g) => `${g.fls_id}:${g.package_version}:${g.account_id}`}
        initialSort={{ column: 'updated', direction: 'descending' }}
        sortValue={(g, k) => {
          switch (k) {
            case 'character': return g.character_name
            case 'fls': return g.fls_id
            case 'version': return g.package_version
            case 'status': return g.status
            case 'attempts': return g.attempts
            case 'updated': return g.updated_at
            case 'error': return g.last_error
            default: return ''
          }
        }}
        emptyState={(
          <EmptyState size="sm">
            <EmptyState.Header>
              <EmptyState.Media variant="icon">
                <IconifyIcon icon="gravity-ui:persons" className="size-5" />
              </EmptyState.Media>
              <EmptyState.Title>{t('welcome.noGrants')}</EmptyState.Title>
            </EmptyState.Header>
          </EmptyState>
        )}
        renderCell={(g, key) => {
          switch (key) {
            case 'character':
              return g.character_name || <span className="text-muted">—</span>
            case 'fls':
              return <span className="font-mono text-xs text-muted">{g.fls_id}</span>
            case 'version':
              return <span className="text-muted text-xs">{g.package_version}</span>
            case 'status':
              return (
                <span className={g.status === 'failed' ? 'text-danger' : 'text-accent'}>
                  {g.status}
                </span>
              )
            case 'attempts':
              return <span className="text-muted">{g.attempts}</span>
            case 'updated':
              return <span className="text-muted text-xs">{fmtTime(g.updated_at)}</span>
            case 'error':
              return g.last_error
                ? <span className="text-danger text-xs">{g.last_error}</span>
                : <span className="text-muted">—</span>
            case 'actions':
              return g.status === 'failed'
                ? (
                    <Button size="sm" variant="outline" className="w-full" onPress={() => retry(g)}>
                      <Icon name="refresh-cw" />
                      {' '}
                      {t('welcome.retry')}
                    </Button>
                  )
                : (
                    <Button size="sm" variant="ghost" className="w-full" onPress={() => revoke(g)}>
                      <Icon name="rotate-ccw" />
                      {' '}
                      {t('welcome.revoke')}
                    </Button>
                  )
          }
        }}
      />
    </div>
  )
}
