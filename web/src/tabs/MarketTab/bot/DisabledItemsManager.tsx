import { useState, useEffect, useMemo } from 'react'
import { Button, Spinner, toast } from '@heroui/react'
import { api } from '../../../api/client'
import type { BotConfig, CatalogItem } from '../../../api/client'
import { DataTable, type Column, Icon } from '../../../dune-ui'

type Props = {
  config: BotConfig
  onSaved: (cfg: BotConfig) => void
}

type DisabledRow = { template_id: string; display_name: string }
type RowKey = 'name' | 'template_id' | 'actions'

const COLUMNS: Column<RowKey>[] = [
  { key: 'name',        label: 'Name' },
  { key: 'template_id', label: 'Template ID' },
  { key: 'actions',     label: '',     sortable: false },
]

export default function DisabledItemsManager({ config, onSaved }: Props) {
  const [catalog, setCatalog] = useState<CatalogItem[]>([])
  const [search, setSearch] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    api.market.catalog().then(setCatalog).catch(() => {})
  }, [])

  const safeItems = useMemo(() => config.disabled_items ?? [], [config.disabled_items])

  const results = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return []
    return catalog
      .filter(c =>
        !safeItems.includes(c.template_id) &&
        (c.display_name.toLowerCase().includes(q) || c.template_id.toLowerCase().includes(q))
      )
      .slice(0, 8)
  }, [search, catalog, safeItems])

  const disabledRows: DisabledRow[] = useMemo(() =>
    safeItems.map(tmpl => ({
      template_id: tmpl,
      display_name: catalog.find(c => c.template_id === tmpl)?.display_name ?? tmpl,
    })),
    [safeItems, catalog]
  )

  const saveList = async (next: string[]) => {
    setSaving(true)
    try {
      const saved = await api.marketBot.saveConfig({ ...config, disabled_items: next })
      onSaved(saved)
    } catch (e: unknown) {
      toast.danger(`Save failed: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setSaving(false)
    }
  }

  const add = (templateId: string) => {
    if (safeItems.includes(templateId)) return
    saveList([...safeItems, templateId])
    setSearch('')
  }

  const remove = (templateId: string) => {
    saveList(safeItems.filter(i => i !== templateId))
  }

  return (
    <div className="flex flex-col gap-4">
      {/* Search + add row */}
      <div className="flex gap-2 items-end">
        <div className="flex flex-col gap-0.5 flex-1">
          <label className="text-xs text-muted">Search items to disable</label>
          <input
            className="bg-surface border border-border rounded px-2 py-1.5 text-sm text-foreground w-full"
            placeholder="Search by name or template ID…"
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
        </div>
        {saving && <Spinner size="sm" color="current" className="mb-2" />}
      </div>

      {/* Search results */}
      {results.length > 0 && (
        <div className="flex flex-col border border-border rounded overflow-hidden">
          {results.map(item => (
            <div
              key={item.template_id}
              className="flex items-center gap-3 px-3 py-2 bg-surface hover:bg-surface/70 border-b border-border/40 last:border-0 transition-colors"
            >
              <div className="flex flex-col flex-1 min-w-0">
                <span className="text-sm text-foreground truncate">{item.display_name}</span>
                <span className="text-xs text-muted font-mono truncate">{item.template_id}</span>
              </div>
              <Button size="sm" variant="outline" onPress={() => add(item.template_id)}>
                <Icon name="plus" /> Add
              </Button>
            </div>
          ))}
        </div>
      )}

      {search.trim() && results.length === 0 && (
        <p className="text-xs text-muted">No matching items found.</p>
      )}

      {/* Disabled list */}
      <div className="flex flex-col gap-2">
        <span className="text-xs font-semibold text-muted uppercase tracking-wider">
          Disabled Items {safeItems.length > 0 && `(${safeItems.length})`}
        </span>
        {disabledRows.length === 0 ? (
          <p className="text-xs text-muted">No items are currently disabled.</p>
        ) : (
          <DataTable<DisabledRow, RowKey>
            aria-label="Disabled items"
            className="flex-1 min-h-0"
            columns={COLUMNS}
            rows={disabledRows}
            rowId={r => r.template_id}
            initialSort={{ column: 'name', direction: 'ascending' }}
            sortValue={(r, k) => k === 'name' ? r.display_name : r.template_id}
            renderCell={(r, key) => {
              switch (key) {
                case 'name':
                  return <span className="font-medium text-foreground">{r.display_name}</span>
                case 'template_id':
                  return <span className="font-mono text-xs text-muted">{r.template_id}</span>
                case 'actions':
                  return (
                    <Button size="sm" variant="danger-soft" onPress={() => remove(r.template_id)}>
                      Remove
                    </Button>
                  )
              }
            }}
          />
        )}
      </div>
    </div>
  )
}
