import { useState, useEffect, useMemo, useCallback } from 'react'
import type React from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button, Chip, SearchField, Spinner, toast,
} from '@heroui/react'
import { api } from '../../api/client'
import type { InventoryItem } from '../../api/client'
import { DataTable, Icon, LoadingState, PageHeader, SideNav, type Column } from '../../dune-ui'
import { AddItemsModal } from './components/AddItemsModal'

type ItemKey = 'id' | 'template' | 'stack_size' | 'quality' | 'durability' | 'actions'

type Container = {
  id: number
  name: string
  class: string
  map: string
  item_count: number
  item_templates: string[]
  item_names: string[]
  owner_name: string
}

const TYPE_LABELS: Record<string, string> = {
  SpiceSilo_Placeable: 'Small Storage Container',
  GenericContainer_Placeable: 'Chest',
  StorageContainer_Placeable: 'Storage Container',
  MediumStorageContainer_Placeable: 'Medium Storage Container',
}

function shortClass(cls: string): string {
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
    { key: 'actions', label: '', width: 120, sortable: false },
  ]

  const [containers, setContainers] = useState<Container[]>([])
  const [loading, setLoading] = useState(false)
  const [selected, setSelected] = useState<Container | null>(null)
  const [items, setItems] = useState<InventoryItem[]>([])
  const [itemsLoading, setItemsLoading] = useState(false)
  const [showAdd, setShowAdd] = useState(false)
  const [search, setSearch] = useState('')

  const load = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.storage.list())
      .then(setContainers)
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const selectContainer = async (c: Container) => {
    setSelected(c)
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

  const filtered = useMemo(() => {
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

  const navItems = useMemo(() => filtered.map((c) => ({
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
          width="w-60"
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
                        <DataTable<InventoryItem, ItemKey>
                          aria-label={t('storage.ariaLabel')}
                          className="min-h-0 max-h-full"
                          columns={ITEM_COLUMNS}
                          rows={items}
                          rowId={(i) => String(i.id)}
                          initialSort={{ column: 'id', direction: 'ascending' }}
                          sortValue={(i, k) => {
                            if (k === 'template') return i.name || i.template_id
                            if (k === 'actions') return ''
                            return (i as unknown as Record<string, string | number>)[k]
                          }}
                          emptyState={<div className="py-8 text-center text-muted">{t('storage.containerEmpty')}</div>}
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
                                    size="sm"
                                    variant="danger-soft"
                                    className="w-full"
                                    onPress={() => handleDeleteItem(i.id)}
                                  >
                                    <Icon name="x" />
                                    {' '}
                                    {t('storage.remove')}
                                  </Button>
                                )
                            }
                          }}
                        />
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
