import { useState, useEffect } from 'react'
import { Button, Spinner, toast } from '@heroui/react'
import { api } from '../../../api/client'
import type { Player, InventoryItem } from '../../../api/client'
import { DataTable, SectionLabel, type Column } from '../../../dune-ui'

type ItemKey = 'template' | 'stack' | 'quality' | 'durability' | 'actions'

const ITEM_COLUMNS: Column<ItemKey>[] = [
  { key: 'template', label: 'Template', isRowHeader: true },
  { key: 'stack', label: 'Stack' },
  { key: 'quality', label: 'Quality' },
  { key: 'durability', label: 'Durability' },
  { key: 'actions', label: '', sortable: false },
]

interface Props {
  player: Player
}

export function InventoryView({ player }: Props) {
  const [items, setItems] = useState<InventoryItem[]>([])
  const [loading, setLoading] = useState(false)

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
      const ok = window.confirm('Player is online — deleting items may cause inventory desyncs. Continue?')
      if (!ok) return
    }
    try {
      await api.players.deleteItem(itemId)
      setItems((prev) => prev.filter((i) => i.id !== itemId))
      toast.success('Item deleted')
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
  }

  const handleRepair = async (item: InventoryItem) => {
    try {
      await api.players.repairItem(item.id)
      setItems((prev) => prev.map((i) => i.id === item.id ? { ...i, durability: i.max_durability } : i))
      toast.success(`Repaired ${item.name || item.template_id}`)
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
  }

  const handleRepairAllGear = async () => {
    try {
      const res = await api.players.repairGear(player.id)
      if (res.repaired === 0) {
        toast.success(`No gear needed repair (${res.scanned} items scanned)`)
      }
      else {
        toast.success(`Repaired ${res.repaired} of ${res.scanned} gear pieces — relog to see in-game`)
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
        <SectionLabel>Items</SectionLabel>
        <Button size="sm" variant="ghost" onPress={handleRepairAllGear}>Repair gear</Button>
      </div>
      <DataTable<InventoryItem, ItemKey>
        aria-label="Inventory items"
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
        emptyState={<div className="py-6 text-center text-muted">No items found</div>}
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
                    <Button size="sm" variant="ghost" onPress={() => handleRepair(i)}>Repair</Button>
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
