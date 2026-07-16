import * as React from 'react'
import { DataTable, type Column } from '../../dune-ui'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import type { MarketItem } from '../../api/client'
import { ItemIcon } from '../../components/ItemIcon'
import { qualityLabel } from '../../utils/icons'
import type { MarketTableKey, MarketTableProps } from './types'

const RARITY_COLORS: Record<string, string> = {
  common: 'text-foreground',
  uncommon: 'text-rarity-uncommon',
  rare: 'text-rarity-rare',
  epic: 'text-rarity-epic',
  legendary: 'text-rarity-legendary',
  unique: 'text-rarity-unique',
  memento: 'text-rarity-memento',
}

export const MarketTable: React.FC<MarketTableProps> = ({ items, onSelect }) => {
  const { t } = useTranslation()

  const COLUMNS: Column<MarketTableKey>[] = [
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
    <DataTable<MarketItem, MarketTableKey>
      aria-label={t('market.table.ariaLabel')}
      className="min-h-0 max-h-full"
      rowHeight={56}
      pageSize={100}
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
      emptyState={(
        <EmptyState size="sm">
          <EmptyState.Header>
            <EmptyState.Media variant="icon">
              <IconifyIcon icon="gravity-ui:tag" className="size-5" />
            </EmptyState.Media>
            <EmptyState.Title>{t('market.table.noItemsFound')}</EmptyState.Title>
          </EmptyState.Header>
        </EmptyState>
      )}
      renderCell={(it, key) => {
        switch (key) {
          case 'display_name':
            return (
              <span className="inline-flex items-center gap-2">
                <ItemIcon
                  templateId={it.template_id}
                  category={it.category}
                  rarity={it.rarity}
                  name={it.display_name || undefined}
                />
                <span className="inline-flex flex-col min-w-0">
                  <span className="text-xs font-medium truncate text-foreground">{it.display_name || it.template_id}</span>
                  <span className="font-mono text-[10px] text-muted truncate">{it.template_id}</span>
                </span>
              </span>
            )
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
