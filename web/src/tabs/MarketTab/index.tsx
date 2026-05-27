import { useState, useEffect, useCallback } from 'react'
import { Button, Spinner } from '@heroui/react'
import { api } from '../../api/client'
import type { MarketItem } from '../../api/client'
import { Icon, PageHeader } from '../../dune-ui'
import MarketSidebar from './MarketSidebar'
import MarketSearch, { type MarketFilters } from './MarketSearch'
import MarketTable from './MarketTable'
import MarketGrid from './MarketGrid'
import ViewToggle, { type MarketView } from './ViewToggle'
import ItemDetail from './ItemDetail'
import BotControlPanel from './bot/BotControlPanel'

const DEFAULT_FILTERS: MarketFilters = { search: '', category: '', owner: '' }

type Props = {
  isSignedIn?: boolean
}

export default function MarketTab({ isSignedIn = false }: Props) {
  const [items, setItems] = useState<MarketItem[]>([])
  const [categories, setCategories] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [filters, setFilters] = useState<MarketFilters>(DEFAULT_FILTERS)
  const [selected, setSelected] = useState<MarketItem | null>(null)
  const [view, setView] = useState<MarketView>('table')
  const [botOpen, setBotOpen] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [res, cats] = await Promise.all([
        api.market.items({
          search: filters.search || undefined,
          category: filters.category || undefined,
          owner: filters.owner || undefined,
        }),
        categories.length === 0 ? api.market.categories() : Promise.resolve(categories),
      ])
      setItems(res.items)
      if (categories.length === 0) setCategories(cats)
    } catch {
      // errors surface via empty state
    } finally {
      setLoading(false)
    }
  }, [filters])

  useEffect(() => { load() }, [load])

  const handleFiltersChange = (f: MarketFilters) => {
    setFilters(f)
    if (selected && f.category !== filters.category) setSelected(null)
  }

  const handleCategorySelect = (cat: string) => {
    setFilters(f => ({ ...f, category: cat }))
    setSelected(null)
  }

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <PageHeader title="Market Board" subtitle="Browse active exchange listings from bot and player sellers.">
        {isSignedIn && (
          <Button size="sm" variant="ghost" onPress={() => setBotOpen(true)}>
            <Icon name="bot" /> Bot Control
          </Button>
        )}
        <ViewToggle view={view} onChange={setView} />
        <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
          {loading ? <Spinner size="sm" color="current" /> : <><Icon name="refresh-cw" /> Refresh</>}
        </Button>
      </PageHeader>

      <MarketSearch
        filters={filters}
        onChange={handleFiltersChange}
        onReset={() => { setFilters(DEFAULT_FILTERS); setSelected(null) }}
      />

      <div className="flex flex-1 gap-3 min-h-0 overflow-hidden">
        <MarketSidebar
          categories={categories}
          selected={filters.category}
          onSelect={handleCategorySelect}
        />

        <div className="flex flex-1 min-w-0 min-h-0 overflow-hidden">
          {loading ? (
            <div className="flex flex-1 justify-center py-12"><Spinner size="lg" /></div>
          ) : view === 'grid' ? (
            <MarketGrid items={items} onSelect={setSelected} />
          ) : (
            <MarketTable items={items} onSelect={setSelected} />
          )}
        </div>

        <ItemDetail item={selected} onClose={() => setSelected(null)} />
      </div>

      {isSignedIn && (
        <BotControlPanel open={botOpen} onClose={() => setBotOpen(false)} />
      )}
    </div>
  )
}
