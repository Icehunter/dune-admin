import * as React from 'react'
import { useAtom } from 'jotai'
import { Button, Spinner } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../api/client'
import type { MarketItem, BotConfig } from '../../api/client'
import { usePermissions } from '../../hooks/usePermissions'
import { Icon, LoadingState, PageHeader } from '../../dune-ui'
import { MarketSidebar } from './MarketSidebar'
import { MarketSearch } from './MarketSearch'
import type { MarketFilters } from './types'
import { marketViewAtom } from './store'
import { MarketTable } from './MarketTable'
import { MarketGrid } from './MarketGrid'
import { ViewToggle } from './ViewToggle'
import { ItemDetail } from './ItemDetail'
import { BotControlPanel } from './bot/BotControlPanel'
import { fetchAllMarketItems } from './helpers'

const DEFAULT_FILTERS: MarketFilters = { search: '', category: '', owner: '' }

export const MarketTab: React.FC = () => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const [items, setItems] = React.useState<MarketItem[]>([])
  const [categories, setCategories] = React.useState<string[]>([])
  const categoriesRef = React.useRef<string[]>([])
  const [loading, setLoading] = React.useState(false)
  const [filters, setFilters] = React.useState<MarketFilters>(DEFAULT_FILTERS)
  const [selected, setSelected] = React.useState<MarketItem | null>(null)
  const [view, setView] = useAtom(marketViewAtom)
  const [botOpen, setBotOpen] = React.useState(false)
  // Show Bot Control whenever the bot is configured (embedded or remote),
  // even if currently disabled/not running.
  const [botConfigured, setBotConfigured] = React.useState(false)
  const [botConfig, setBotConfig] = React.useState<BotConfig | null>(null)
  const canManageBot = can('market-bot:manage')

  React.useEffect(() => {
    api.marketBot
      .status()
      // configured field from newer backends; fall back to mode check for older ones.
      // Treat absent mode (pre-mode backend) as not-configured rather than configured.
      .then((s) => setBotConfigured(s.configured ?? (s.mode !== undefined && s.mode !== 'none')))
      .catch(() => setBotConfigured(false))
  }, [])

  // Fetch the bot config so per-row disable actions (#288) can read/update
  // disabled_items without opening the bot drawer.
  React.useEffect(() => {
    if (!canManageBot) return
    api.marketBot.config().then(setBotConfig).catch(() => {})
  }, [canManageBot])

  const load = (): void => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() =>
        Promise.all([
          fetchAllMarketItems(api.market.items, {
            search: filters.search || undefined,
            category: filters.category || undefined,
            owner: filters.owner || undefined,
          }),
          categoriesRef.current.length === 0 ? api.market.categories() : Promise.resolve(categoriesRef.current),
        ]),
      )
      .then(([fetchedItems, cats]) => {
        setItems(fetchedItems)
        if (categoriesRef.current.length === 0) {
          categoriesRef.current = cats
          setCategories(cats)
        }
      })
      .catch(() => {
        /* errors surface via empty state */
      })
      .finally(() => setLoading(false))
  }

  React.useEffect(() => {
    load()
  }, [filters]) // eslint-disable-line react-hooks/exhaustive-deps

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
        {can('market-bot:read') && (
          botConfigured
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
              )
        )}
        <ViewToggle view={view} onChange={setView} />
        <Button size="sm" variant="ghost" onPress={load} isDisabled={loading}>
          {loading
            ? (
                <Spinner size="sm" color="current" />
              )
            : (
                <React.Fragment>
                  <Icon name="refresh-cw" />
                  {' '}
                  {t('common.refresh')}
                </React.Fragment>
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
                  <MarketGrid
                    items={items}
                    onSelect={setSelected}
                    canManageBot={canManageBot}
                    botConfig={botConfig}
                    onItemDisabled={setBotConfig}
                  />
                )
              : (
                  <MarketTable
                    items={items}
                    onSelect={setSelected}
                    canManageBot={canManageBot}
                    botConfig={botConfig}
                    onItemDisabled={setBotConfig}
                  />
                )}
        </div>

      </div>

      <ItemDetail item={selected} onClose={() => setSelected(null)} />
      {can('market-bot:read') && <BotControlPanel open={botOpen} onClose={() => setBotOpen(false)} />}
    </div>
  )
}
