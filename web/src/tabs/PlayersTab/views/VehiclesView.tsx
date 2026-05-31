import { useState, useEffect } from 'react'
import { Button, Chip, Spinner, toast } from '@heroui/react'
import { api } from '../../../api/client'
import type { Player, VehicleRow } from '../../../api/client'
import { DataTable, SectionLabel, type Column } from '../../../dune-ui'

type VehicleKey = 'class' | 'location' | 'chassis' | 'name' | 'type' | 'actions'

const VEHICLE_COLUMNS: Column<VehicleKey>[] = [
  { key: 'class', label: 'Class', isRowHeader: true },
  { key: 'location', label: 'Location' },
  { key: 'chassis', label: 'Chassis' },
  { key: 'name', label: 'Name' },
  { key: 'type', label: 'Type', sortable: false },
  { key: 'actions', label: '', sortable: false },
]

interface Props {
  player: Player
}

export function VehiclesView({ player }: Props) {
  const [vehicles, setVehicles] = useState<VehicleRow[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
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

  const handleRepairVehicle = async (v: VehicleRow) => {
    try {
      const res = await api.players.repairVehicle(v.id, player.id)
      const label = v.vehicle_name || v.class
      if (res.total === 0) {
        toast.success(`No modules found on ${label}`)
      }
      else if (res.skipped > 0) {
        toast.success(`Repaired ${res.repaired} of ${res.total} modules on ${label} (${res.skipped} skipped) — relog to see in-game`)
      }
      else {
        toast.success(`Repaired ${res.repaired} modules on ${label} — relog to see in-game`)
      }
      api.players.vehicles(player.controller_id).then(setVehicles).catch(() => {})
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
  }

  const handleRefuelVehicle = async (v: VehicleRow) => {
    try {
      await api.players.refuelVehicle(v.id, player.id)
      toast.success(`Refueled ${v.vehicle_name || v.class} — relog to see in-game`)
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
  }

  if (loading) {
    return <div className="flex justify-center py-8"><Spinner size="lg" /></div>
  }

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <div className="shrink-0 min-h-8 flex items-center"><SectionLabel>Vehicles</SectionLabel></div>
      <DataTable<VehicleRow, VehicleKey>
        aria-label="Vehicles"
        className="min-h-0 max-h-full"
        columns={VEHICLE_COLUMNS}
        rows={vehicles}
        rowId={(v) => String(v.id)}
        initialSort={{ column: 'class', direction: 'ascending' }}
        sortValue={(v, k) => {
          if (k === 'class') return v.class
          if (k === 'location') return v.map ?? ''
          if (k === 'chassis') return v.chassis_durability
          if (k === 'name') return v.vehicle_name ?? ''
          return ''
        }}
        emptyState={<div className="py-6 text-center text-muted">No vehicles found</div>}
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
                  {v.is_backup && <Chip size="sm" color="accent" variant="soft">Backup</Chip>}
                  {v.is_recovered && <Chip size="sm" color="warning" variant="soft">Recovered</Chip>}
                </div>
              )
            case 'actions':
              return !v.is_backup
                ? (
                    <div className="flex gap-1">
                      <Button size="sm" variant="ghost" onPress={() => handleRepairVehicle(v)}>Repair</Button>
                      <Button size="sm" variant="ghost" onPress={() => handleRefuelVehicle(v)}>Refuel</Button>
                    </div>
                  )
                : null
          }
        }}
      />
    </div>
  )
}
