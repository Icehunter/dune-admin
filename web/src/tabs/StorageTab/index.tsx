import * as React from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button, Chip, SearchField, Spinner, toast,
} from '@heroui/react'
import type { Selection } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { api } from '../../api/client'
import type { InventoryItem } from '../../api/client'
import { ActionBar, DataTable, Icon, LoadingState, PageHeader, SideNav, type Column } from '../../dune-ui'
import { AddItemsModal } from './components/AddItemsModal'
import type { Container, ItemKey } from './types'

const TYPE_LABELS: Record<string, string> = {
  SpiceSilo_Placeable: 'Small Storage Container',
  GenericContainer_Placeable: 'Chest',
  StorageContainer_Placeable: 'Storage Container',
  MediumStorageContainer_Placeable: 'Medium Storage Container',
}

const shortClass = (cls: string): string => {
  return TYPE_LABELS[cls] ?? cls.replace(/_Placeable$/, '')
}

export const StorageTab: React.FC = () => {
  const { t } = useTranslation()

  const ITEM_COLUMNS: Column<ItemKey>[] = [
    { key: 'id', label: t('storage.columns.id'), width: 100 },
    { key: 'template', label: t('storage.columns.template'), minWidth: 240 },
    { key: 'stack_size', label: t('storage.columns.stack'), width: 100 },
    { key: 'quality', label: t('storage.columns.quality'), width: 100 },
    { key: 'durability', label: t('storage.columns.durability'), width: 130 },
    { key: 'actions', label: '', width: 52, sortable: false },
  ]

  const [containers, setContainers] = React.useState<Container[]>([])
  const [loading, setLoading] = React.useState(false)
  const [selected, setSelected] = React.useState<Container | null>(null)
  const [items, setItems] = React.useState<InventoryItem[]>([])
  const [itemsLoading, setItemsLoading] = React.useState(false)
  const [showAdd, setShowAdd] = React.useState(false)
  const [search, setSearch] = React.useState('')
  const [selectedKeys, setSelectedKeys] = React.useState<Selection>(new Set())

  const load = React.useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.storage.list())
      .then(setContainers)
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [])

  React.useEffect(() => {
    load()
  }, [load])

  const selectContainer = async (c: Container) => {
    setSelected(c)
    setSelectedKeys(new Set())
    setItemsLoading(true)
    try {
      setItems(await api.storage.items(c.id))
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
    finally {
      setItemsLoading(false)
    }
  }

  const handleDeleteItem = async (itemId: number) => {
    try {
      await api.players.deleteItem(itemId)
      setItems((prev) => prev.filter((i) => i.id !== itemId))
      if (selected) {
        setContainers((prev) => prev.map((c) => c.id === selected.id ? { ...c, item_count: c.item_count - 1 } : c))
      }
      toast.success(t('storage.itemRemoved'))
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
    const deletedIds = new Set<number>()
    await Promise.allSettled(
      ids.map(async (id) => {
        await api.players.deleteItem(id)
        deletedIds.add(id)
      }),
    )
    setItems((prev) => prev.filter((i) => !deletedIds.has(i.id)))
    if (selected) {
      setContainers((prev) => prev.map((c) =>
        c.id === selected.id ? { ...c, item_count: Math.max(0, c.item_count - deletedIds.size) } : c,
      ))
    }
    setSelectedKeys(new Set())
    toast.success(t('storage.itemsRemoved', { count: deletedIds.size }))
  }

  const filtered = React.useMemo(() => {
    if (!search) return containers
    const q = search.toLowerCase()
    return containers.filter((c) =>
      String(c.id).includes(q)
      || c.map.toLowerCase().includes(q)
      || shortClass(c.class).toLowerCase().includes(q)
      || (c.name && c.name.toLowerCase().includes(q))
      || (c.owner_name && c.owner_name.toLowerCase().includes(q))
      || (c.item_templates ?? []).some((tmpl) => tmpl.toLowerCase().includes(q))
      || (c.item_names ?? []).some((n) => n.toLowerCase().includes(q)),
    )
  }, [containers, search])

  const navItems = React.useMemo(() => filtered.map((c) => ({
    key: String(c.id),
    label: c.name || `#${c.id}`,
    sublabel: [
      c.name ? `#${c.id}` : null,
      shortClass(c.class),
      c.map,
      c.owner_name || null,
    ].filter(Boolean).join(' · '),
    hint: <Chip size="sm" variant="soft">{c.item_count}</Chip>,
  })), [filtered])

  return (
    <div className="flex flex-col gap-3 h-full min-h-0">
      {/* Warning banner */}
      <div className="shrink-0 rounded-[var(--radius)] px-4 py-2 text-xs font-medium bg-danger/10 border border-danger/40 text-danger flex items-center gap-2">
        <Icon name="triangle-alert" />
        <span>{t('storage.warningText')}</span>
      </div>

      <div className="flex gap-3 flex-1 min-h-0">
        <SideNav
          items={navItems}
          active={selected ? String(selected.id) : null}
          onSelect={(id) => {
            const c = containers.find((x) => String(x.id) === id)
            if (c) selectContainer(c)
          }}
          title={t('storage.containersTitle', { count: containers.length })}
          titleAction={(
            <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
              {loading ? <Spinner size="sm" color="current" /> : <Icon name="refresh-cw" />}
            </Button>
          )}
          width="w-[276px]"
        >
          <SearchField
            aria-label={t('storage.searchLabel')}
            value={search}
            onChange={setSearch}
            className="w-full"
          >
            <SearchField.Group>
              <SearchField.SearchIcon />
              <SearchField.Input placeholder={t('storage.searchPlaceholder')} />
              <SearchField.ClearButton />
            </SearchField.Group>
          </SearchField>
        </SideNav>

        <div className="flex-1 flex flex-col gap-3 min-h-0">
          {!selected
            ? (
                <div className="flex items-center justify-center h-full text-muted">
                  <p className="text-sm">{t('storage.selectContainer')}</p>
                </div>
              )
            : (
                <>
                  <PageHeader
                    title={selected.name || t('storage.containerTitle', { id: selected.id })}
                    subtitle={[
                      selected.name ? `#${selected.id}` : null,
                      shortClass(selected.class),
                      selected.map,
                      selected.owner_name ? t('storage.ownerLabel', { name: selected.owner_name }) : null,
                    ].filter(Boolean).join(' · ')}
                  >
                    <Button size="sm" variant="ghost" onPress={() => selectContainer(selected)} isDisabled={itemsLoading}>
                      {itemsLoading
                        ? <Spinner size="sm" color="current" />
                        : (
                            <>
                              <Icon name="refresh-cw" />
                              {' '}
                              {t('common.refresh')}
                            </>
                          )}
                    </Button>
                    <Button size="sm" onPress={() => setShowAdd(true)}>
                      <Icon name="plus" />
                      {' '}
                      {t('storage.addItems')}
                    </Button>
                  </PageHeader>

                  {itemsLoading
                    ? (
                        <LoadingState />
                      )
                    : (
                        <>
                          <DataTable<InventoryItem, ItemKey>
                            aria-label={t('storage.ariaLabel')}
                            className="min-h-0 max-h-full"
                            columns={ITEM_COLUMNS}
                            rows={items}
                            rowId={(i) => String(i.id)}
                            selectionMode="multiple"
                            selectedKeys={selectedKeys}
                            onSelectionChange={setSelectedKeys}
                            initialSort={{ column: 'id', direction: 'ascending' }}
                            sortValue={(i, k) => {
                              if (k === 'template') return i.name || i.template_id
                              if (k === 'actions') return ''
                              return (i as unknown as Record<string, string | number>)[k]
                            }}
                            emptyState={(
                              <EmptyState size="sm">
                                <EmptyState.Header>
                                  <EmptyState.Media variant="icon">
                                    <IconifyIcon icon="gravity-ui:box" className="size-5" />
                                  </EmptyState.Media>
                                  <EmptyState.Title>{t('storage.containerEmpty')}</EmptyState.Title>
                                </EmptyState.Header>
                              </EmptyState>
                            )}
                            renderCell={(i, key) => {
                              switch (key) {
                                case 'id': return <span className="font-mono text-muted">{i.id}</span>
                                case 'template':
                                  return (
                                    <span className="inline-flex flex-col">
                                      <span>{i.name || i.template_id}</span>
                                      {i.name && <span className="text-xs font-mono text-muted">{i.template_id}</span>}
                                    </span>
                                  )
                                case 'stack_size': return <span>{i.stack_size}</span>
                                case 'quality': return <span>{i.quality}</span>
                                case 'durability': return <span className="text-muted">{i.durability}</span>
                                case 'actions':
                                  return (
                                    <Button
                                      isIconOnly
                                      size="sm"
                                      variant="danger-soft"
                                      aria-label={t('storage.remove')}
                                      onPress={() => handleDeleteItem(i.id)}
                                    >
                                      <Icon name="trash" />
                                    </Button>
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
                        </>
                      )}
                </>
              )}
        </div>
      </div>

      {selected && (
        <AddItemsModal
          container={selected}
          open={showAdd}
          onClose={() => setShowAdd(false)}
          onSuccess={() => {
            setShowAdd(false)
            selectContainer(selected)
          }}
          onRefresh={() => selectContainer(selected)}
        />
      )}
    </div>
  )
}
