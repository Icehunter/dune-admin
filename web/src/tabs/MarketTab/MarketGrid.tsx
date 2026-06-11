import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { iconUrl, categoryColor, qualityLabel } from '../../utils/icons'
import type { MarketGridProps } from './types'

const RARITY_BORDER: Record<string, string> = {
  common: 'border-border',
  uncommon: 'border-rarity-uncommon/60',
  rare: 'border-rarity-rare/60',
  epic: 'border-rarity-epic/60',
  legendary: 'border-rarity-legendary/60',
  unique: 'border-rarity-unique/60',
  memento: 'border-rarity-memento/60',
}

const RARITY_TEXT: Record<string, string> = {
  common: 'text-foreground',
  uncommon: 'text-rarity-uncommon',
  rare: 'text-rarity-rare',
  epic: 'text-rarity-epic',
  legendary: 'text-rarity-legendary',
  unique: 'text-rarity-unique',
  memento: 'text-rarity-memento',
}

export const MarketGrid: React.FC<MarketGridProps> = ({ items, onSelect }: MarketGridProps) => {
  const { t } = useTranslation()

  if (items.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <EmptyState size="md">
          <EmptyState.Header>
            <EmptyState.Media variant="icon">
              <IconifyIcon icon="gravity-ui:tag" className="size-5" />
            </EmptyState.Media>
            <EmptyState.Title>{t('market.table.noItemsFound')}</EmptyState.Title>
          </EmptyState.Header>
        </EmptyState>
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto pr-1">
      <div className="grid grid-cols-[repeat(auto-fill,minmax(160px,1fr))] gap-3 pb-3">
        {items.map((item) => {
          const key = `${item.template_id}:${item.quality}`
          const rarity = item.rarity?.toLowerCase()
          const border = RARITY_BORDER[rarity] ?? 'border-border'
          const textColor = RARITY_TEXT[rarity] ?? 'text-foreground'
          const img = iconUrl(item.template_id, 'thumb')

          return (
            <button
              key={key}
              className={`flex flex-col rounded-[var(--radius)] border ${border} bg-surface hover:bg-surface/80 text-left transition-colors overflow-hidden`}
              onClick={() => onSelect(item)}
            >
              {/* Icon area */}
              <div
                className="w-full aspect-square flex items-center justify-center shrink-0"
                style={{ background: img ? undefined : categoryColor(item.category) }}
              >
                {img
                  ? (
                      <img
                        src={img}
                        alt={item.display_name}
                        className="w-full h-full object-contain p-2"
                        onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none' }}
                      />
                    )
                  : (
                      <span className="text-3xl text-white/20 font-bold uppercase select-none">
                        {item.display_name.charAt(0)}
                      </span>
                    )}
              </div>

              {/* Card body */}
              <div className="p-2 flex flex-col gap-0.5 min-w-0">
                <span className="text-xs font-medium leading-tight line-clamp-2 text-foreground">
                  {item.display_name}
                </span>
                <div className="flex items-center justify-between gap-1 mt-0.5">
                  {item.quality > 0 && (
                    <span className="text-[10px] text-muted truncate">{qualityLabel(item.quality)}</span>
                  )}
                  {item.rarity && (
                    <span className={`text-[10px] capitalize shrink-0 ${textColor}`}>{item.rarity}</span>
                  )}
                </div>
                <div className="flex items-center justify-between gap-1 mt-1">
                  <span className="text-xs font-mono text-accent font-semibold truncate">
                    {item.lowest_price.toLocaleString()}
                  </span>
                  <span className="text-[10px] text-muted shrink-0">
                    ×
                    {item.total_stock.toLocaleString()}
                  </span>
                </div>
              </div>
            </button>
          )
        })}
      </div>
    </div>
  )
}
