import * as React from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button,
  Spinner,
  toast,
} from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { api } from '../../api/client'
import type { BlueprintRow } from '../../api/client'
import { DataTable, Icon, PageHeader, type Column } from '../../dune-ui'
import { usePermissions } from '../../hooks/usePermissions'
import type { BlueprintsTabKey } from '../types'
import type { BlueprintsTabProps } from '../interfaces'
import { ImportModal } from './ImportModal'

export const BlueprintsTab: React.FC<BlueprintsTabProps> = ({ isSignedIn = true }) => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const canWorldWrite = can('world:write')
  const canExportData = can('data:export')
  const [blueprints, setBlueprints] = React.useState<BlueprintRow[]>([])
  const [loading, setLoading] = React.useState(false)
  const [showImport, setShowImport] = React.useState(false)

  const COLUMNS: Column<BlueprintsTabKey>[] = [
    { key: 'id', label: t('blueprints.columns.id'), width: 80 },
    { key: 'owner_name', label: t('blueprints.columns.owner'), minWidth: 140 },
    { key: 'name', label: t('blueprints.columns.name'), minWidth: 200 },
    { key: 'item_id', label: t('blueprints.columns.itemId'), minWidth: 200 },
    { key: 'pieces', label: t('blueprints.columns.pieces'), width: 100 },
    { key: 'placeables', label: t('blueprints.columns.placeables'), width: 110 },
    { key: 'actions', label: '', width: 110, sortable: false },
  ]

  const load = (): void => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.blueprints.list())
      .then(setBlueprints)
      .catch((e: unknown) => toast.danger(t('blueprints.failedToLoad', { message: e instanceof Error ? e.message : String(e) })))
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
            <strong>{t('blueprints.layoutAccountStrong')}</strong>
            {' '}
            account is required to export or import blueprints. Sign in using the button
            in the top right.
          </span>
        </div>
      )}

      <PageHeader
        title={t('blueprints.title', { count: blueprints.length })}
        subtitle={t('blueprints.subtitle')}
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
        {canWorldWrite && (
          <Button size="sm" onPress={() => setShowImport(true)} isDisabled={!isSignedIn}>
            <Icon name="upload" />
            {' '}
            {t('blueprints.importBlueprint')}
          </Button>
        )}
      </PageHeader>

      <DataTable<BlueprintRow, BlueprintsTabKey>
        aria-label={t('blueprints.ariaLabel')}
        className="min-h-0 max-h-full"
        columns={canExportData ? COLUMNS : COLUMNS.filter((c) => c.key !== 'actions')}
        rows={blueprints}
        loading={loading}
        rowId={(b) => String(b.id)}
        initialSort={{ column: 'id', direction: 'ascending' }}
        sortValue={(b, k) => (k === 'actions' ? '' : (b as unknown as Record<string, string | number>)[k])}
        emptyState={(
          <EmptyState size="sm">
            <EmptyState.Header>
              <EmptyState.Media variant="icon">
                <IconifyIcon icon="gravity-ui:box" className="size-5" />
              </EmptyState.Media>
              <EmptyState.Title>{t('blueprints.noBlueprintsFound')}</EmptyState.Title>
            </EmptyState.Header>
          </EmptyState>
        )}
        renderCell={(b, key) => {
          switch (key) {
            case 'id':
              return <span className="font-mono text-muted">{b.id}</span>
            case 'owner_name':
              return b.owner_name
            case 'name':
              return b.name || <span className="text-muted">—</span>
            case 'item_id':
              return <span className="font-mono text-muted">{b.item_id}</span>
            case 'pieces':
              return <span className="text-muted">{b.pieces}</span>
            case 'placeables':
              return <span className="text-muted">{b.placeables}</span>
            case 'actions':
              return isSignedIn
                ? (
                    <a
                      href={api.blueprints.exportUrl(b.id)}
                      download={b.name ? `${b.name.replace(/[/\\:*?"<React.Fragment>|]/g, '_')}.json` : `blueprint_${b.id}.json`}
                    >
                      <Button size="sm" variant="outline" className="w-full">
                        <Icon name="download" />
                        {' '}
                        {t('common.export')}
                      </Button>
                    </a>
                  )
                : (
                    <Button size="sm" variant="outline" className="w-full" isDisabled>
                      <Icon name="download" />
                      {' '}
                      {t('common.export')}
                    </Button>
                  )
          }
        }}
      />

      {canWorldWrite && (
        <ImportModal
          open={showImport}
          onClose={() => setShowImport(false)}
          onSuccess={() => {
            setShowImport(false)
            load()
          }}
        />
      )}
    </div>
  )
}
