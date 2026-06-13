import * as React from 'react'
import { Button, ListBox, Select, Spinner, toast } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { useTranslation } from 'react-i18next'
import { usePermissions } from '../../../hooks/usePermissions'
import { ConfirmDialog, DataTable, Icon, PageHeader, type Column } from '../../../dune-ui'
import { PlayerSearchField } from '../../../components/PlayerSearchField'
import type { Player, WelcomeGrantRecord } from '../../../api/client'
import type { GrantKey, GrantsViewProps } from './types'

const fmtTime = (s: string): string => {
  if (!s) return '—'
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString()
}

export const GrantsView: React.FC<GrantsViewProps> = ({
  grants, retry, revoke, override, packages, activeVersions, load, loading,
}) => {
  const { t } = useTranslation()
  const { can } = usePermissions()

  const [ovPlayer, setOvPlayer] = React.useState<Player | null>(null)
  const [ovVersion, setOvVersion] = React.useState(() => activeVersions[0] ?? packages[0]?.version ?? '')
  const [ovConfirm, setOvConfirm] = React.useState(false)
  const [ovRunning, setOvRunning] = React.useState(false)

  const doOverride = async () => {
    if (!ovPlayer || !ovVersion) return
    setOvConfirm(false)
    setOvRunning(true)
    try {
      await override(ovPlayer.account_id, ovVersion)
      setOvPlayer(null)
    }
    catch (e) {
      toast.danger(t('welcome.overrideFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setOvRunning(false)
    }
  }

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

      {can('welcome:manage') && (
        <div className="shrink-0 rounded-[var(--radius)] border border-border bg-surface p-3 flex flex-col gap-2">
          <div className="flex items-center gap-2">
            <Icon name="gift" />
            <span className="text-sm font-medium">{t('welcome.overrideTitle')}</span>
          </div>
          <p className="text-xs text-muted">{t('welcome.overrideHint')}</p>
          <div className="flex flex-wrap items-end gap-3">
            <div className="flex flex-col gap-0.5 flex-1 min-w-56">
              <label className="text-xs text-muted">{t('welcome.overridePlayer')}</label>
              <PlayerSearchField
                ariaLabel={t('welcome.overridePlayer')}
                placeholder={t('welcome.overridePlayerPlaceholder')}
                onSelect={setOvPlayer}
                onClear={() => setOvPlayer(null)}
              />
            </div>
            <div className="flex flex-col gap-0.5">
              <label className="text-xs text-muted">{t('welcome.overridePackage')}</label>
              <Select
                aria-label={t('welcome.overridePackage')}
                selectedKey={ovVersion || null}
                onSelectionChange={(k) => setOvVersion(k ? String(k) : '')}
                className="w-48"
              >
                <Select.Trigger>
                  <Select.Value>{ovVersion || t('welcome.overrideSelectPackage')}</Select.Value>
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    {packages.map((p) => (
                      <ListBox.Item key={p.version} id={p.version} textValue={p.version}>
                        {p.version}
                        {activeVersions.includes(p.version) ? ' (active)' : ''}
                        <ListBox.ItemIndicator />
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
            </div>
            <Button
              size="sm"
              variant="secondary"
              isDisabled={!ovPlayer || !ovVersion || ovRunning}
              onPress={() => setOvConfirm(true)}
            >
              {ovRunning
                ? <Spinner size="sm" color="current" />
                : (
                    <>
                      <Icon name="gift" />
                      {' '}
                      {t('welcome.overrideButton')}
                    </>
                  )}
            </Button>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={ovConfirm}
        title={t('welcome.overrideConfirmTitle')}
        description={t('welcome.overrideConfirmBody', {
          name: ovPlayer?.name ?? '',
          version: ovVersion,
        })}
        confirmLabel={t('welcome.overrideButton')}
        onConfirm={doOverride}
        onCancel={() => setOvConfirm(false)}
      />

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
              if (!can('welcome:manage')) return null
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
