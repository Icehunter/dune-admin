import * as React from 'react'
import { Button, Chip, Modal, Spinner, toast } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { InventoryItem, VehicleRow } from '../../../api/client'
import { DataTable, Icon, LoadingState, Panel, SectionLabel, type Column } from '../../../dune-ui'
import type { ItemKey, VehicleKey, InventoryModalProps } from './types'

export const InventoryModal: React.FC<InventoryModalProps> = ({ player, open, onClose }) => {
  const { t } = useTranslation()
  const [items, setItems] = React.useState<InventoryItem[]>([])
  const [loading, setLoading] = React.useState(false)
  const [vehicles, setVehicles] = React.useState<VehicleRow[]>([])
  const [vehiclesLoading, setVehiclesLoading] = React.useState(false)

  const ITEM_COLUMNS: Column<ItemKey>[] = [
    { key: 'template', label: t('players.inventory.columns.template'), isRowHeader: true },
    { key: 'stack', label: t('players.inventory.columns.stack') },
    { key: 'quality', label: t('players.inventory.columns.quality') },
    { key: 'durability', label: t('players.inventory.columns.durability') },
    { key: 'actions', label: ' ', sortable: false },
  ]

  const VEHICLE_COLUMNS: Column<VehicleKey>[] = [
    { key: 'class', label: t('players.vehicles.columns.class'), isRowHeader: true },
    { key: 'location', label: t('players.vehicles.columns.location') },
    { key: 'chassis', label: t('players.vehicles.columns.chassis') },
    { key: 'name', label: t('players.vehicles.columns.name') },
    { key: 'type', label: t('players.vehicles.columns.type'), sortable: false },
    { key: 'actions', label: ' ', sortable: false },
  ]

  React.useEffect(() => {
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
        api.players.inventory(player.id).then(setItems).catch(() => {})
      }
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <Modal.Backdrop variant="blur" className="bg-linear-to-t from-(--background)/85 via-(--background)/40 to-transparent" isOpen={open} onOpenChange={(v) => !v && onClose()}>
      <Modal.Container size="cover" scroll="outside">
        <Modal.Dialog className="p-10">
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
                  <LoadingState size="md" />
                )
              : (
                  <div className="flex flex-col gap-4 flex-1 min-h-0 overflow-hidden">
                    <Panel className="flex-1 min-h-0 overflow-hidden">
                      <div className="shrink-0 flex items-center justify-between">
                        <SectionLabel>{t('players.inventory.itemsLabel')}</SectionLabel>
                        <Button size="sm" variant="ghost" onPress={handleRepairAllGear}>{t('players.inventory.repairGear')}</Button>
                      </div>
                      <div className="shrink-0 rounded-[var(--radius)] px-4 py-2 text-xs font-medium bg-danger/10 border border-danger/40 text-danger flex items-center gap-2">
                        <Icon name="triangle-alert" className="shrink-0" />
                        <span>{t('players.inventory.repairNotice')}</span>
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
                        emptyState={(
                          <EmptyState size="sm">
                            <EmptyState.Header>
                              <EmptyState.Media variant="icon">
                                <IconifyIcon icon="gravity-ui:box" className="size-5" />
                              </EmptyState.Media>
                              <EmptyState.Title>{t('players.inventory.noItemsFound')}</EmptyState.Title>
                            </EmptyState.Header>
                          </EmptyState>
                        )}
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
                                {' / '}
                                {i.max_durability}
                              </span>
                            )
                            case 'actions':
                              return (
                                <div className="flex gap-1">
                                  {i.max_durability !== 'N/A' && (
                                    <Button size="sm" variant="ghost" onPress={() => handleRepair(i)}>{t('players.inventory.repair')}</Button>
                                  )}
                                  <Button isIconOnly size="sm" variant="danger-soft" aria-label={t('common.delete')} onPress={() => handleDelete(i.id)}><Icon name="trash" /></Button>
                                </div>
                              )
                          }
                        }}
                      />
                    </Panel>

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
                            case 'location': return <span className="text-muted">{v.map || '–'}</span>
                            case 'chassis':
                              return (
                                <span className={v.chassis_durability < 0.3 ? 'text-danger' : 'text-muted'}>
                                  {Math.round(v.chassis_durability * 100)}
                                  %
                                </span>
                              )
                            case 'name': return <span className="text-muted">{v.vehicle_name || '–'}</span>
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
                    </Panel>
                  </div>
                )}
          </Modal.Body>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
