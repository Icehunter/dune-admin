import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useAtomValue } from 'jotai'
import { EmptyState } from '@heroui-pro/react'
import { Icon as IconifyIcon } from '@iconify/react'
import { iconUrl, categoryColor, qualityLabel, BG_PURPLE } from '../../utils/icons'
import { itemDataSyncAtom } from '../../data/store'
import type { MarketGridProps } from './types'
import { QualityArc } from './QualityArc'
import { DisableItemAction } from './DisableItemAction'

// Max volume for bar scaling (most wearable/weapon items top out around 30V)
const VOL_MAX = 30

// Schematic background — deep purple grid with light-ray bloom (sampled Image #23, darkened ~25%)
const SCHEMATIC_BG: React.CSSProperties = {
  background: [
    'linear-gradient(rgba(175,125,255,0.20) 1px, transparent 1px)',
    'linear-gradient(90deg, rgba(175,125,255,0.20) 1px, transparent 1px)',
    'radial-gradient(ellipse at 65% 20%, rgba(200,155,255,0.22) 0%, transparent 55%)',
    '#1e0838',
  ].join(', '),
  backgroundSize: '20px 20px, 20px 20px, 100% 100%, 100% 100%',
}

const RARITY_BORDER: Record<string, string> = {
  common: 'border-border',
  uncommon: 'border-rarity-uncommon/60',
  rare: 'border-rarity-rare/60',
  epic: 'border-rarity-epic/60',
  legendary: 'border-rarity-legendary/60',
  unique: 'border-rarity-unique/60',
  memento: 'border-rarity-memento/60',
}

export const MarketGrid: React.FC<MarketGridProps> = (
  { items, onSelect, canManageBot, botConfig, onItemDisabled }: MarketGridProps,
) => {
  const { t } = useTranslation()
  const itemData = useAtomValue(itemDataSyncAtom)

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
          const img = iconUrl(item.template_id, 'thumb')
          const entry = itemData.items[item.template_id] ?? null
          // unique rarity = schematic in game data; also check is_schematic for safety
          const isSchematic = rarity === 'unique' || (entry?.is_schematic ?? false)
          // named unique set wearables also use the purple background (grid overlay for schematics only)
          const isNamedSet = !isSchematic && rarity === 'rare' && /Unique/.test(item.template_id)
          const volume = entry?.volume ?? 0
          const volPct = volume > 0 ? Math.min(1, volume / VOL_MAX) * 100 : 0

          return (
            <div key={key} className="relative group">
              <button
                className={`flex flex-col w-full rounded-[var(--radius)] border-2 ${border} bg-surface text-left transition-all overflow-hidden hover:shadow-md`}
                onClick={() => onSelect(item)}
              >
                {/* Icon area */}
                <div
                  className="relative w-full aspect-square flex items-center justify-center shrink-0"
                  style={isSchematic
                    ? SCHEMATIC_BG
                    : { background: isNamedSet ? BG_PURPLE : categoryColor(item.category, rarity) }}
                >
                  {/* Item image */}
                  {img
                    ? (
                        <img
                          src={img}
                          alt={item.display_name}
                          className="w-full h-full object-contain p-2 transition-transform duration-200 group-hover:scale-105"
                          onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none' }}
                        />
                      )
                    : (
                        <span className="text-3xl text-white/20 font-bold uppercase select-none">
                          {item.display_name.charAt(0)}
                        </span>
                      )}

                  {/* Quality arc — top-right corner, only for gradeable items */}
                  {entry?.is_gradeable && (
                    <div className="absolute top-1 right-1 pointer-events-none">
                      <QualityArc quality={item.quality} size={20} />
                    </div>
                  )}

                  {/* Left weight/volume bar — teal, scales with volume */}
                  {volume > 0 && (
                    <div className="absolute left-1.5 bottom-1.5 w-1 rounded-full overflow-hidden" style={{ height: '40%' }}>
                      <div className="absolute inset-0 bg-white/10" />
                      <div
                        className="absolute bottom-0 left-0 right-0 rounded-full"
                        style={{ height: `${volPct}%`, background: '#4dd0c4' }}
                      />
                    </div>
                  )}

                  {/* Bottom gradient blending into card body */}
                  <div className="absolute inset-x-0 bottom-0 h-1/4 bg-gradient-to-t from-surface to-transparent pointer-events-none" />
                </div>

                {/* Card body */}
                <div className="px-2.5 pb-2.5 pt-1 flex flex-col gap-1 min-w-0">
                  <span className="text-xs font-medium leading-snug line-clamp-2 text-foreground">
                    {item.display_name}
                  </span>
                  <div className="flex items-center justify-between gap-1">
                    {item.quality > 0
                      ? <span className="text-[10px] text-muted truncate">{qualityLabel(item.quality)}</span>
                      : <span />}
                    {rarity && (
                      <span
                        className="text-[10px] capitalize font-medium shrink-0"
                        style={{ color: `var(--rarity-${rarity})` }}
                      >
                        {rarity}
                      </span>
                    )}
                  </div>
                  <div className="flex items-center justify-between gap-1 pt-1.5 border-t border-border/40 mt-0.5">
                    <span className="text-sm font-mono text-accent font-semibold truncate">
                      {item.lowest_price.toLocaleString()}
                    </span>
                    <span className="text-[10px] text-muted shrink-0">
                      ×
                      {item.total_stock.toLocaleString()}
                    </span>
                  </div>
                </div>
              </button>

              {/* Disable action — sibling overlay, not nested in the card button (#288) */}
              <div className="absolute top-1 left-1 z-10 opacity-0 group-hover:opacity-100 focus-within:opacity-100 transition-opacity">
                <DisableItemAction
                  item={item}
                  botConfig={botConfig}
                  canManage={canManageBot}
                  onDisabled={onItemDisabled}
                  variant="icon"
                />
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
