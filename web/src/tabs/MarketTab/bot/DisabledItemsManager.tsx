import * as React from 'react'
import { Button, SearchField, Spinner, toast } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../../api/client'
import type { CatalogItem } from '../../../api/client'
import { DataTable, type Column, Icon } from '../../../dune-ui'
import type { DisabledItemsManagerProps, DisabledRow, RowKey } from './types'

export const DisabledItemsManager: React.FC<DisabledItemsManagerProps> = (
  { config, onSaved }: DisabledItemsManagerProps,
) => {
  const { t } = useTranslation()
  const [catalog, setCatalog] = React.useState<CatalogItem[]>([])
  const [search, setSearch] = React.useState('')
  const [saving, setSaving] = React.useState(false)

  const COLUMNS: Column<RowKey>[] = [
    { key: 'name', label: t('market.bot.disabledItems.columns.name') },
    { key: 'template_id', label: t('market.bot.disabledItems.columns.templateId') },
    { key: 'actions', label: ' ', sortable: false },
  ]

  React.useEffect(() => {
    api.market.catalog().then(setCatalog).catch(() => {})
  }, [])

  const safeItems = React.useMemo(() => config.disabled_items ?? [], [config.disabled_items])

  const results = React.useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return []
    return catalog
      .filter((c) =>
        !safeItems.includes(c.template_id)
        && (c.display_name.toLowerCase().includes(q) || c.template_id.toLowerCase().includes(q)),
      )
      .slice(0, 8)
  }, [search, catalog, safeItems])

  const disabledRows: DisabledRow[] = React.useMemo(() =>
    safeItems.map((tmpl) => ({
      template_id: tmpl,
      display_name: catalog.find((c) => c.template_id === tmpl)?.display_name ?? tmpl,
    })),
  [safeItems, catalog],
  )

  const saveList = async (next: string[]) => {
    setSaving(true)
    try {
      const saved = await api.marketBot.saveConfig({ ...config, disabled_items: next })
      onSaved(saved)
    }
    catch (e: unknown) {
      toast.danger(t('common.failed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setSaving(false)
    }
  }

  const add = (templateId: string) => {
    if (safeItems.includes(templateId)) return
    saveList([...safeItems, templateId])
    setSearch('')
  }

  const remove = (templateId: string) => {
    saveList(safeItems.filter((i) => i !== templateId))
  }

  return (
    <div className="flex flex-col gap-4">
      {/* Search + add row */}
      <div className="flex gap-2 items-end">
        <div className="flex flex-col gap-0.5 flex-1">
          <label className="text-xs text-muted">{t('market.bot.disabledItems.searchLabel')}</label>
          <SearchField
            aria-label={t('market.bot.disabledItems.searchAriaLabel')}
            value={search}
            onChange={setSearch}
            className="w-full"
          >
            <SearchField.Group>
              <SearchField.SearchIcon />
              <SearchField.Input placeholder={t('market.bot.disabledItems.searchPlaceholder')} />
              <SearchField.ClearButton />
            </SearchField.Group>
          </SearchField>
        </div>
        {saving && <Spinner size="sm" color="current" className="mb-2" />}
      </div>

      {/* Search results */}
      {results.length > 0 && (
        <div className="flex flex-col border border-border rounded overflow-hidden">
          {results.map((item) => (
            <div
              key={item.template_id}
              className="flex items-center gap-3 px-3 py-2 bg-surface hover:bg-surface/70 border-b border-border/40 last:border-0 transition-colors"
            >
              <div className="flex flex-col flex-1 min-w-0">
                <span className="text-sm text-foreground truncate">{item.display_name}</span>
                <span className="text-xs text-muted font-mono truncate">{item.template_id}</span>
              </div>
              <Button size="sm" variant="outline" onPress={() => add(item.template_id)}>
                <Icon name="plus" />
                {' '}
                {t('common.add')}
              </Button>
            </div>
          ))}
        </div>
      )}

      {search.trim() && results.length === 0 && (
        <p className="text-xs text-muted">{t('market.bot.disabledItems.noMatchingItems')}</p>
      )}

      {/* Disabled list */}
      <div className="flex flex-col gap-2">
        <span className="text-xs font-semibold text-muted uppercase tracking-wider">
          {safeItems.length > 0
            ? t('market.bot.disabledItems.disabledItemsCount', { count: safeItems.length })
            : t('market.bot.disabledItems.disabledItemsHeading')}
        </span>
        {disabledRows.length === 0
          ? (
              <p className="text-xs text-muted">{t('market.bot.disabledItems.noDisabledItems')}</p>
            )
          : (
              <DataTable<DisabledRow, RowKey>
                aria-label={t('market.bot.disabledItems.ariaLabel')}
                className="flex-1 min-h-0"
                columns={COLUMNS}
                rows={disabledRows}
                rowId={(r) => r.template_id}
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
                          {t('common.remove')}
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
