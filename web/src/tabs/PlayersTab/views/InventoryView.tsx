import { useState, useEffect } from 'react'
import { Button, Spinner, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { Player, InventoryItem } from '../../../api/client'
import { DataTable, SectionLabel, type Column } from '../../../dune-ui'

type ItemKey = 'template' | 'stack' | 'quality' | 'durability' | 'actions'

interface Props {
  player: Player
}

export function InventoryView({ player }: Props) {
  const { t } = useTranslation()
  const [items, setItems] = useState<InventoryItem[]>([])
  const [loading, setLoading] = useState(false)

  const ITEM_COLUMNS: Column<ItemKey>[] = [
    { key: 'template', label: t('players.inventory.columns.template'), isRowHeader: true },
    { key: 'stack', label: t('players.inventory.columns.stack') },
    { key: 'quality', label: t('players.inventory.columns.quality') },
    { key: 'durability', label: t('players.inventory.columns.durability') },
    { key: 'actions', label: '', sortable: false },
  ]

  useEffect(() => {
    Promise.resolve()
      .then(() => {
        setItems([])
        setLoading(true)
      })
      .then(() => api.players.inventory(player.id))
      .then(setItems)
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [player.id])

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

  if (loading) {
    return <div className="flex justify-center py-8"><Spinner size="lg" /></div>
  }

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <div className="shrink-0 min-h-8 flex items-center justify-between">
        <SectionLabel>{t('players.inventory.itemsLabel')}</SectionLabel>
        <Button size="sm" variant="ghost" onPress={handleRepairAllGear}>{t('players.inventory.repairGear')}</Button>
      </div>
      <DataTable<InventoryItem, ItemKey>
        aria-label={t('players.inventory.title')}
        className="min-h-0 max-h-full"
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
                  <Button size="sm" variant="danger-soft" onPress={() => handleDelete(i.id)}>X</Button>
                </div>
              )
          }
        }}
      />
    </div>
  )
}
