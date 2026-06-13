import * as React from 'react'
import { Chip, toast } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { VehicleRow } from '../../../api/client'
import { DataTable, LoadingState, SectionLabel, type Column } from '../../../dune-ui'
import { usePermissions } from '../../../hooks/usePermissions'
import type { VehicleKey, VehiclesViewProps } from './types'

export const VehiclesView: React.FC<VehiclesViewProps> = ({ player }) => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const canPlayersWrite = can('players:write')
  const [vehicles, setVehicles] = React.useState<VehicleRow[]>([])
  const [loading, setLoading] = React.useState(false)

  const ALL_VEHICLE_COLUMNS: Column<VehicleKey>[] = [
    { key: 'class', label: t('players.vehicles.columns.class'), isRowHeader: true },
    { key: 'location', label: t('players.vehicles.columns.location') },
    { key: 'chassis', label: t('players.vehicles.columns.chassis') },
    { key: 'name', label: t('players.vehicles.columns.name') },
    { key: 'type', label: t('players.vehicles.columns.type'), sortable: false },
    { key: 'actions', label: ' ', sortable: false },
  ]
  const VEHICLE_COLUMNS = ALL_VEHICLE_COLUMNS.filter((c) => canPlayersWrite || c.key !== 'actions')

  React.useEffect(() => {
    Promise.resolve()
      .then(() => {
        setVehicles([])
        setLoading(true)
      })
      .then(() => api.players.vehicles(player.controller_id))
      .then(setVehicles)
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [player.controller_id])

  if (loading) {
    return <LoadingState size="md" />
  }

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <div className="shrink-0 min-h-8 flex items-center"><SectionLabel>{t('players.vehicles.vehiclesLabel')}</SectionLabel></div>
      <DataTable<VehicleRow, VehicleKey>
        aria-label={t('players.vehicles.vehiclesLabel')}
        className="min-h-0 max-h-full"
        columns={VEHICLE_COLUMNS}
        rows={vehicles}
        rowId={(v) => `${v.id}-${v.is_backup ? 'b' : 'a'}`}
        initialSort={{ column: 'class', direction: 'ascending' }}
        sortValue={(v, k) => {
          if (k === 'class') return v.class
          if (k === 'location') return v.map ?? ''
          if (k === 'chassis') return v.chassis_durability
          if (k === 'name') return v.vehicle_name ?? ''
          return ''
        }}
        emptyState={(
          <EmptyState size="sm">
            <EmptyState.Header>
              <EmptyState.Media variant="icon">
                <IconifyIcon icon="gravity-ui:car" className="size-5" />
              </EmptyState.Media>
              <EmptyState.Title>{t('players.vehicles.noVehiclesFound')}</EmptyState.Title>
            </EmptyState.Header>
          </EmptyState>
        )}
        renderCell={(v, key) => {
          switch (key) {
            case 'class': return <span className="font-semibold">{v.class}</span>
            case 'location': return <span className="text-muted">{v.map || '—'}</span>
            case 'chassis':
              return (
                <span className={v.chassis_durability < 0.3 ? 'text-danger' : 'text-muted'}>
                  {Math.round(v.chassis_durability * 100)}
                  %
                </span>
              )
            case 'name': return <span className="text-muted">{v.vehicle_name || '—'}</span>
            case 'type':
              return (
                <div className="flex gap-1">
                  {v.is_backup && <Chip size="sm" color="accent" variant="soft">{t('players.vehicles.backup')}</Chip>}
                  {v.is_recovered && <Chip size="sm" color="warning" variant="soft">{t('players.vehicles.recovered')}</Chip>}
                </div>
              )
            case 'actions':
              return !v.is_backup
                ? (
                    <div className="flex gap-1" />
                  )
                : null
          }
        }}
      />
    </div>
  )
}
