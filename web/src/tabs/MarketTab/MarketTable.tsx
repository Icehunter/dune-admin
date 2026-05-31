import { DataTable, type Column } from '../../dune-ui'
import { useTranslation } from 'react-i18next'
import type { MarketItem } from '../../api/client'
import { qualityLabel } from '../../utils/icons'

type Key = 'display_name' | 'quality' | 'category' | 'tier' | 'rarity' | 'lowest_price' | 'total_stock' | 'bot_stock' | 'listing_count'

const RARITY_COLORS: Record<string, string> = {
  common: 'text-foreground',
  uncommon: 'text-success',
  rare: 'text-blue-400',
  epic: 'text-purple-400',
  legendary: 'text-amber-400',
  unique: 'text-orange-400',
  memento: 'text-rose-400',
}

type Props = {
  items: MarketItem[]
  onSelect: (item: MarketItem) => void
}

export default function MarketTable({ items, onSelect }: Props) {
  const { t } = useTranslation()

  const COLUMNS: Column<Key>[] = [
    { key: 'display_name', label: t('market.table.item'), minWidth: 200 },
    { key: 'quality', label: t('market.table.grade'), width: 100 },
    { key: 'category', label: t('market.table.category'), minWidth: 140 },
    { key: 'tier', label: t('market.table.tier'), width: 60 },
    { key: 'rarity', label: t('market.table.rarity'), width: 100 },
    { key: 'lowest_price', label: t('market.table.lowestPrice'), width: 120 },
    { key: 'total_stock', label: t('market.table.stock'), width: 80 },
    { key: 'bot_stock', label: t('market.table.botStock'), width: 90 },
    { key: 'listing_count', label: t('market.table.listings'), width: 80 },
  ]

  return (
    <DataTable<MarketItem, Key>
      aria-label={t('market.table.ariaLabel')}
      className="min-h-0 max-h-full"
      columns={COLUMNS}
      rows={items}
      rowId={(it) => `${it.template_id}:${it.quality}`}
      initialSort={{ column: 'display_name', direction: 'ascending' }}
      sortValue={(it, k) => {
        switch (k) {
          case 'display_name': return it.display_name
          case 'quality': return it.quality
          case 'category': return it.category
          case 'rarity': return it.rarity
          case 'tier': return it.tier
          case 'lowest_price': return it.lowest_price
          case 'total_stock': return it.total_stock
          case 'bot_stock': return it.bot_stock
          case 'listing_count': return it.listing_count
        }
      }}
      onRowAction={onSelect}
      emptyState={<div className="py-8 text-center text-muted">{t('market.table.noItemsFound')}</div>}
      renderCell={(it, key) => {
        switch (key) {
          case 'display_name':
            return <span className="font-medium">{it.display_name || it.template_id}</span>
          case 'quality':
            return it.quality > 0
              ? <span className="text-xs text-muted">{qualityLabel(it.quality)}</span>
              : <span className="text-xs text-muted/50">Standard</span>
          case 'category':
            return <span className="text-muted text-xs">{it.category || '—'}</span>
          case 'tier':
            return it.tier > 0 ? <span className="text-muted">{it.tier}</span> : <span className="text-muted">—</span>
          case 'rarity':
            return (
              <span className={`text-xs font-medium capitalize ${RARITY_COLORS[it.rarity?.toLowerCase()] ?? 'text-foreground'}`}>
                {it.rarity || '—'}
              </span>
            )
          case 'lowest_price':
            return <span className="font-mono text-accent">{it.lowest_price.toLocaleString()}</span>
          case 'total_stock':
            return <span className="text-muted">{it.total_stock.toLocaleString()}</span>
          case 'bot_stock':
            return <span className="text-muted">{it.bot_stock.toLocaleString()}</span>
          case 'listing_count':
            return <span className="text-muted">{it.listing_count}</span>
        }
      }}
    />
  )
}
