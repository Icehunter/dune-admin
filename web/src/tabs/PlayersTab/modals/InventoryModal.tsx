import { useState, useEffect } from 'react'
import { Button, Chip, Modal, Spinner, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { Player, InventoryItem, VehicleRow } from '../../../api/client'
import { DataTable, Panel, SectionLabel, type Column } from '../../../dune-ui'

type ItemKey = 'template' | 'stack' | 'quality' | 'durability' | 'actions'
type VehicleKey = 'class' | 'location' | 'chassis' | 'name' | 'type' | 'actions'

interface Props {
  player: Player
  open: boolean
  onClose: () => void
}

export function InventoryModal({ player, open, onClose }: Props) {
  const { t } = useTranslation()
  const [items, setItems] = useState<InventoryItem[]>([])
  const [loading, setLoading] = useState(false)
  const [vehicles, setVehicles] = useState<VehicleRow[]>([])
  const [vehiclesLoading, setVehiclesLoading] = useState(false)

  const ITEM_COLUMNS: Column<ItemKey>[] = [
    { key: 'template', label: t('players.inventory.columns.template'), isRowHeader: true },
    { key: 'stack', label: t('players.inventory.columns.stack') },
    { key: 'quality', label: t('players.inventory.columns.quality') },
    { key: 'durability', label: t('players.inventory.columns.durability') },
    { key: 'actions', label: '', sortable: false },
  ]

  const VEHICLE_COLUMNS: Column<VehicleKey>[] = [
    { key: 'class', label: t('players.vehicles.columns.class'), isRowHeader: true },
    { key: 'location', label: t('players.vehicles.columns.location') },
    { key: 'chassis', label: t('players.vehicles.columns.chassis') },
    { key: 'name', label: t('players.vehicles.columns.name') },
    { key: 'type', label: t('players.vehicles.columns.type'), sortable: false },
    { key: 'actions', label: '', sortable: false },
  ]

  useEffect(() => {
    if (!open) {
      Promise.resolve().then(() => setVehicles([]))
      return
    }
    Promise.resolve()
      .then(() => {
        setLoading(true)
        setVehiclesLoading(true)
      })
      .then(() => Promise.all([
        api.players.inventory(player.id),
        api.players.vehicles(player.controller_id),
      ]))
      .then(([inv, vehs]) => {
        setItems(inv)
        setVehicles(vehs)
      })
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => {
        setLoading(false)
        setVehiclesLoading(false)
      })
  }, [open, player.id, player.controller_id])

  const handleDelete = async (itemId: number) => {
    if (player.online_status === 'Online') {
      const ok = window.confirm(t('players.inventory.deleteOnlineWarning'))
      if (!ok) return
    }
    try {
      await api.players.deleteItem(itemId)
      setItems((prev) => prev.filter((i) => i.id !== itemId))
      toast.success(t('players.inventory.itemDeleted'))
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
  }

  const handleRepair = async (item: InventoryItem) => {
    try {
      await api.players.repairItem(item.id)
      setItems((prev) => prev.map((i) => i.id === item.id ? { ...i, durability: i.max_durability } : i))
      toast.success(t('players.inventory.repaired', { name: item.name || item.template_id }))
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
  }

  const handleRepairAllGear = async () => {
    try {
      const res = await api.players.repairGear(player.id)
      if (res.repaired === 0) {
        toast.success(t('players.inventory.repairGearNone', { scanned: res.scanned }))
      }
      else {
        toast.success(t('players.inventory.repairGearDone', { repaired: res.repaired, scanned: res.scanned }))
        // Refetch so the UI reflects the new durability values.
        api.players.inventory(player.id).then(setItems).catch(() => {})
      }
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
  }

  const handleRepairVehicle = async (v: VehicleRow) => {
    try {
      const res = await api.players.repairVehicle(v.id, player.id)
      const label = v.vehicle_name || v.class
      if (res.total === 0) {
        toast.success(t('players.vehicles.repairNone', { label }))
      }
      else if (res.skipped > 0) {
        toast.success(t('players.vehicles.repairPartialDetail', { repaired: res.repaired, total: res.total, label, skipped: res.skipped }))
      }
      else {
        toast.success(t('players.vehicles.repairDone', { repaired: res.repaired, label }))
      }
      // Refresh the chassis % indicator.
      api.players.vehicles(player.controller_id).then(setVehicles).catch(() => {})
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
  }

  const handleRefuelVehicle = async (v: VehicleRow) => {
    try {
      await api.players.refuelVehicle(v.id, player.id)
      toast.success(t('players.vehicles.refuelDone', { label: v.vehicle_name || v.class }))
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <Modal>
      <Modal.Backdrop isOpen={open} onOpenChange={(v) => !v && onClose()}>
        <Modal.Container size="cover" scroll="outside">
          <Modal.Dialog>
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading className="text-accent">
                {player.name}
                {' — '}
                {t('players.inventory.title')}
              </Modal.Heading>
            </Modal.Header>
            <Modal.Body className="flex flex-col gap-4">
              {loading
                ? (
                    <div className="flex justify-center py-8"><Spinner size="lg" /></div>
                  )
                : (
                    <div className="flex flex-col gap-4 flex-1 min-h-0 overflow-hidden">
                      {/* Items — fills remaining space and owns its own scroll */}
                      <Panel className="flex-1 min-h-0 overflow-hidden">
                        <div className="shrink-0 flex items-center justify-between">
                          <SectionLabel>{t('players.inventory.itemsLabel')}</SectionLabel>
                          <Button size="sm" variant="ghost" onPress={handleRepairAllGear}>{t('players.inventory.repairGear')}</Button>
                        </div>
                        <DataTable<InventoryItem, ItemKey>
                          aria-label={t('players.inventory.title')}
                          className="flex-1 min-h-0"
                          columns={ITEM_COLUMNS}
                          rows={items}
                          rowId={(i) => String(i.id)}
                          initialSort={{ column: 'template', direction: 'ascending' }}
                          sortValue={(i, k) => {
                            if (k === 'template') return i.name || i.template_id
                            if (k === 'stack') return i.stack_size
                            if (k === 'quality') return i.quality
                            if (k === 'durability') return typeof i.durability === 'number' ? i.durability : 0
                            return ''
                          }}
                          emptyState={<div className="py-6 text-center text-muted">{t('players.inventory.noItemsFound')}</div>}
                          renderCell={(i, key) => {
                            switch (key) {
                              case 'template':
                                return (
                                  <span className="inline-flex flex-col">
                                    <span className="font-semibold">{i.name || i.template_id}</span>
                                    {i.name && <span className="font-mono text-muted text-[10px]">{i.template_id}</span>}
                                  </span>
                                )
                              case 'stack': return <span className="text-muted">{i.stack_size}</span>
                              case 'quality': return <span className="text-muted">{i.quality}</span>
                              case 'durability': return (
                                <span className="text-muted">
                                  {i.durability}
                                  {' '}
                                  /
                                  {' '}
                                  {i.max_durability}
                                </span>
                              )
                              case 'actions':
                                return (
                                  <div className="flex gap-1">
                                    {i.max_durability !== 'N/A' && (
                                      <Button size="sm" variant="ghost" onPress={() => handleRepair(i)}>{t('players.inventory.repair')}</Button>
                                    )}
                                    <Button size="sm" variant="danger-soft" onPress={() => handleDelete(i.id)}>X</Button>
                                  </div>
                                )
                            }
                          }}
                        />
                      </Panel>

                      {/* Vehicles — fixed ~4-row window, scrolls independently */}
                      <Panel className="shrink-0">
                        <div className="flex items-center gap-2">
                          <SectionLabel>{t('players.vehicles.vehiclesLabel')}</SectionLabel>
                          {vehiclesLoading && <Spinner size="sm" color="current" />}
                        </div>
                        <DataTable<VehicleRow, VehicleKey>
                          aria-label={t('players.vehicles.vehiclesLabel')}
                          className="max-h-[180px]"
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
                          emptyState={<div className="py-6 text-center text-muted">{t('players.vehicles.noVehiclesFound')}</div>}
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
                                      <div className="flex gap-1">
                                        <Button size="sm" variant="ghost" onPress={() => handleRepairVehicle(v)}>{t('players.vehicles.repair')}</Button>
                                        <Button size="sm" variant="ghost" onPress={() => handleRefuelVehicle(v)}>{t('players.vehicles.refuel')}</Button>
                                      </div>
                                    )
                                  : null
                            }
                          }}
                        />
                      </Panel>
                    </div>
                  )}
            </Modal.Body>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
