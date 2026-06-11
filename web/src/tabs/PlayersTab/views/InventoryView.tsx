import * as React from 'react'
import { Button, toast } from '@heroui/react'
import type { Selection } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { InventoryItem } from '../../../api/client'
import { ActionBar, DataTable, Icon, LoadingState, SectionLabel, type Column } from '../../../dune-ui'
import type { InventoryViewProps, ItemKey } from './types'

export const InventoryView: React.FC<InventoryViewProps> = ({ player }) => {
  const { t } = useTranslation()
  const [items, setItems] = React.useState<InventoryItem[]>([])
  const [loading, setLoading] = React.useState(false)
  const [selectedKeys, setSelectedKeys] = React.useState<Selection>(new Set())

  const ITEM_COLUMNS: Column<ItemKey>[] = [
    { key: 'template', label: t('players.inventory.columns.template'), isRowHeader: true },
    { key: 'stack', label: t('players.inventory.columns.stack') },
    { key: 'quality', label: t('players.inventory.columns.quality') },
    { key: 'durability', label: t('players.inventory.columns.durability') },
    { key: 'actions', label: ' ', sortable: false },
  ]

  React.useEffect(() => {
    Promise.resolve()
      .then(() => {
        setItems([])
        setLoading(true)
        setSelectedKeys(new Set())
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

  const selectionCount = selectedKeys === 'all' ? items.length : (selectedKeys as Set<string>).size

  const handleBulkDelete = async () => {
    const ids
      = selectedKeys === 'all'
        ? items.map((i) => i.id)
        : items.filter((i) => (selectedKeys as Set<string>).has(String(i.id))).map((i) => i.id)

    if (ids.length === 0) return

    if (player.online_status === 'Online') {
      const ok = window.confirm(t('players.inventory.deleteOnlineWarning'))
      if (!ok) return
    }

    const deletedIds = new Set<number>()
    await Promise.allSettled(
      ids.map(async (id) => {
        await api.players.deleteItem(id)
        deletedIds.add(id)
      }),
    )
    setItems((prev) => prev.filter((i) => !deletedIds.has(i.id)))
    setSelectedKeys(new Set())
    toast.success(t('players.inventory.itemsDeleted', { count: deletedIds.size }))
  }

  if (loading) {
    return <LoadingState size="md" />
  }

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <div className="shrink-0 min-h-8 flex items-center justify-between">
        <SectionLabel>{t('players.inventory.itemsLabel')}</SectionLabel>
        <Button size="sm" variant="ghost" onPress={handleRepairAllGear}>{t('players.inventory.repairGear')}</Button>
      </div>
      <div className="shrink-0 rounded-[var(--radius)] px-4 py-2 text-xs font-medium bg-danger/10 border border-danger/40 text-danger flex items-center gap-2 -mt-1">
        <Icon name="triangle-alert" />
        <span>{t('players.inventory.repairNotice')}</span>
      </div>
      <DataTable<InventoryItem, ItemKey>
        aria-label={t('players.inventory.title')}
        className="min-h-0 max-h-full"
        columns={ITEM_COLUMNS}
        rows={items}
        rowId={(i) => String(i.id)}
        initialSort={{ column: 'template', direction: 'ascending' }}
        selectionMode="multiple"
        selectedKeys={selectedKeys}
        onSelectionChange={setSelectedKeys}
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
      <ActionBar isOpen={selectionCount > 0}>
        <ActionBar.Prefix>
          <span className="text-sm text-muted">
            {selectionCount}
          </span>
        </ActionBar.Prefix>
        <ActionBar.Content>
          <Button size="sm" variant="danger-soft" onPress={handleBulkDelete}>
            <Icon name="trash" />
            {t('players.inventory.deleteSelected')}
          </Button>
        </ActionBar.Content>
        <ActionBar.Suffix>
          <Button size="sm" variant="ghost" onPress={() => setSelectedKeys(new Set())}>
            {t('common.clear')}
          </Button>
        </ActionBar.Suffix>
      </ActionBar>
    </div>
  )
}
