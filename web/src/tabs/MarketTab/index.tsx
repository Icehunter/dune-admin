import * as React from 'react'
import { Button, Spinner } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../api/client'
import type { MarketItem } from '../../api/client'
import { Icon, LoadingState, PageHeader } from '../../dune-ui'
import { MarketSidebar } from './MarketSidebar'
import { MarketSearch } from './MarketSearch'
import type { MarketFilters, MarketView } from './types'
import { MarketTable } from './MarketTable'
import { MarketGrid } from './MarketGrid'
import { ViewToggle } from './ViewToggle'
import { ItemDetail } from './ItemDetail'
import { BotControlPanel } from './bot/BotControlPanel'

const DEFAULT_FILTERS: MarketFilters = { search: '', category: '', owner: '' }

export const MarketTab: React.FC = () => {
  const { t } = useTranslation()
  const [items, setItems] = React.useState<MarketItem[]>([])
  const [categories, setCategories] = React.useState<string[]>([])
  const categoriesRef = React.useRef<string[]>([])
  const [loading, setLoading] = React.useState(false)
  const [filters, setFilters] = React.useState<MarketFilters>(DEFAULT_FILTERS)
  const [selected, setSelected] = React.useState<MarketItem | null>(null)
  const [view, setView] = React.useState<MarketView>('table')
  const [botOpen, setBotOpen] = React.useState(false)
  // Show Bot Control whenever the bot is configured (embedded or remote),
  // even if currently disabled/not running.
  const [botConfigured, setBotConfigured] = React.useState(false)

  React.useEffect(() => {
    api.marketBot
      .status()
      // configured field from newer backends; fall back to mode check for older ones.
      // Treat absent mode (pre-mode backend) as not-configured rather than configured.
      .then((s) => setBotConfigured(s.configured ?? (s.mode !== undefined && s.mode !== 'none')))
      .catch(() => setBotConfigured(false))
  }, [])

  const load = React.useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() =>
        Promise.all([
          api.market.items({
            search: filters.search || undefined,
            category: filters.category || undefined,
            owner: filters.owner || undefined,
          }),
          categoriesRef.current.length === 0 ? api.market.categories() : Promise.resolve(categoriesRef.current),
        ]),
      )
      .then(([res, cats]) => {
        setItems(res.items)
        if (categoriesRef.current.length === 0) {
          categoriesRef.current = cats
          setCategories(cats)
        }
      })
      .catch(() => {
        /* errors surface via empty state */
      })
      .finally(() => setLoading(false))
  }, [filters])

  React.useEffect(() => {
    load()
  }, [load])

  const handleFiltersChange = (f: MarketFilters) => {
    setFilters(f)
    if (selected && f.category !== filters.category) setSelected(null)
  }

  const handleCategorySelect = (cat: string) => {
    setFilters((f) => ({ ...f, category: cat }))
    setSelected(null)
  }

  return (
    <div className="flex flex-col h-full gap-3 min-h-0">
      <PageHeader title={t('market.title')} subtitle={t('market.subtitle')}>
        {botConfigured
          ? (
              <Button size="sm" variant="ghost" onPress={() => setBotOpen(true)}>
                <Icon name="bot" />
                {' '}
                {t('market.botControl')}
              </Button>
            )
          : (
              <span className="hidden text-xs text-muted sm:inline">
                {t('market.noBotConnected')}
              </span>
            )}
        <ViewToggle view={view} onChange={setView} />
        <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
          {loading
            ? (
                <Spinner size="sm" color="current" />
              )
            : (
                <>
                  <Icon name="refresh-cw" />
                  {' '}
                  {t('common.refresh')}
                </>
              )}
        </Button>
      </PageHeader>

      <MarketSearch
        filters={filters}
        onChange={handleFiltersChange}
        onReset={() => {
          setFilters(DEFAULT_FILTERS)
          setSelected(null)
        }}
      />

      <div className="flex flex-1 gap-3 min-h-0 overflow-hidden">
        <MarketSidebar categories={categories} selected={filters.category} onSelect={handleCategorySelect} />

        <div className="flex flex-1 min-w-0 min-h-0 overflow-hidden">
          {loading && items.length === 0
            ? (
                <LoadingState fill />
              )
            : view === 'grid'
              ? (
                  <MarketGrid items={items} onSelect={setSelected} />
                )
              : (
                  <MarketTable items={items} onSelect={setSelected} />
                )}
        </div>

      </div>

      <ItemDetail item={selected} onClose={() => setSelected(null)} />
      <BotControlPanel open={botOpen} onClose={() => setBotOpen(false)} />
    </div>
  )
}
