import * as React from 'react'
import { Chip, Spinner } from '@heroui/react'
import { useAtomValue } from 'jotai'
import { unwrap } from 'jotai/utils'
import { DataTable, Panel, SectionLabel } from '../dune-ui'
import { iconUrl, categoryColor, qualityLabel } from '../utils/icons'
import { qualityDataAtom } from '../data/store'
import type { ItemEntry } from '../data/store'
import type { MarketListing } from '../api/client'
import { Row } from './ItemDetailCard.Row'
import { MitigationBar } from './ItemDetailCard.MitigationBar'

// ── Constants ──────────────────────────────────────────────────────────────────

const QUALITY_LABELS = ['Standard', 'Refined', 'Superior', 'Masterwork', 'Pristine', 'Flawless']

const MITIGATION_LABELS: Record<string, string> = {
  melee: 'Melee',
  physical: 'Physical',
  energy: 'Energy',
  explosive: 'Explosive',
  heat: 'Heat',
  cold: 'Cold',
  poison: 'Poison',
  radiation: 'Radiation',
  sandstorm1: 'Sandstorm I',
  sandstorm2: 'Sandstorm II',
  sandstorm3: 'Sandstorm III',
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

const qualityDataSync = unwrap(qualityDataAtom, () => null)

// ── Types ──────────────────────────────────────────────────────────────────────

export type MarketDetail = {
  lowestPrice: number
  totalStock: number
  botStock: number
  listingCount: number
  listings: MarketListing[]
  listingsLoading: boolean
}

export type ItemDetailCardProps = {
  templateId: string
  /** Display name override (e.g. from the templates list). Falls back to entry.name then templateId. */
  name?: string | undefined
  entry: ItemEntry | null
  /**
   * Category override for the displayed "Category" row (e.g. MarketItem.category,
   * which reclassifies schematics into schematics/<segment>). Without this the raw
   * catalog path from entry.category is shown, which disagreed with the market
   * table's category for schematics (#295) — pass the same computed value the
   * caller already used to categorize the item so the two views always agree.
   */
  category?: string | undefined
  market?: MarketDetail | undefined
}

// ── Component ──────────────────────────────────────────────────────────────────

export const ItemDetailCard: React.FC<ItemDetailCardProps> = ({ templateId, name, entry, category, market }) => {
  const qualityData = useAtomValue(qualityDataSync)

  const img = iconUrl(templateId, 'detail')
  const rarity = entry?.rarity?.toLowerCase()
  const borderClass = RARITY_BORDER[rarity ?? ''] ?? 'border-border'

  const isArmor = !!entry?.armor_value
  const isWeapon = entry?.category?.startsWith('items/weapons')
  const isGradeable = entry?.is_gradeable
  const displayName = name || entry?.name || templateId
  // Prefer the caller-computed category (e.g. MarketItem.category) for display so
  // it always matches whatever category the caller listed/grouped this item under.
  const displayCategory = category ?? entry?.category

  const listings = market?.listings ?? []
  const byQuality = listings.reduce<Record<number, MarketListing[]>>((acc, l) => {
    ;(acc[l.quality] ??= []).push(l)
    return acc
  }, {})
  const qualities = Object.keys(byQuality).map(Number).sort((a, b) => a - b)

  const renderRarityChips = (): React.ReactNode => (
    <React.Fragment>
      {rarity
        ? (
            <Chip size="sm" variant="soft" className="capitalize" style={{ color: `var(--rarity-${rarity})` }}>
              {rarity}
            </Chip>
          )
        : null}
      {entry?.tier && entry.tier > 0
        ? (
            <Chip size="sm" variant="soft">
              {`T${entry.tier}`}
            </Chip>
          )
        : null}
      {entry?.is_schematic
        ? (
            <Chip size="sm" variant="soft">
              Schematic
            </Chip>
          )
        : null}
    </React.Fragment>
  )

  const renderItemInfoPanel = (): React.ReactNode => {
    if (!displayCategory && !entry?.slot && !entry?.faction && !market) return null
    return (
      <Panel>
        <SectionLabel>Item Info</SectionLabel>
        {displayCategory
          ? <Row label="Category" value={displayCategory} wrap />
          : null}
        {entry?.slot
          ? <Row label="Slot" value={entry.slot} />
          : null}
        {entry?.faction
          ? <Row label="Faction" value={entry.faction} />
          : null}
        {market
          ? (
              <React.Fragment>
                <Row label="Total Stock" value={market.totalStock.toLocaleString()} />
                <Row label="Bot Stock" value={market.botStock.toLocaleString()} />
                <Row label="Listings" value={String(market.listingCount)} />
                <Row label="Lowest Price" value={market.lowestPrice.toLocaleString()} accent />
              </React.Fragment>
            )
          : null}
      </Panel>
    )
  }

  const renderArmorPanel = (): React.ReactNode => {
    if (!isArmor) return null
    return (
      <Panel>
        <SectionLabel>Armor Stats</SectionLabel>
        {isGradeable
          ? (
              <React.Fragment>
                <div className="text-xs text-muted mb-2">Armor Value by Quality</div>
                {(qualityData?.armor ?? []).map((mult, i) => (
                  <Row
                    key={i}
                    label={QUALITY_LABELS[i] ?? `Q${i}`}
                    value={String(Math.round(entry!.armor_value! * mult))}
                  />
                ))}
              </React.Fragment>
            )
          : (
              <Row label="Armor Value" value={String(entry!.armor_value)} />
            )}
        {entry?.mitigation && Object.keys(entry.mitigation).length > 0 && (
          <React.Fragment>
            <div className="text-xs text-muted mt-2 mb-1">Resistances</div>
            {Object.entries(entry.mitigation).map(([k, v]) => (
              <MitigationBar key={k} label={MITIGATION_LABELS[k] ?? k} value={v} />
            ))}
          </React.Fragment>
        )}
      </Panel>
    )
  }

  const renderWeaponPanel = (): React.ReactNode => {
    if (!isWeapon || !isGradeable) return null
    return (
      <Panel>
        <SectionLabel>Weapon Quality Scaling</SectionLabel>
        <div className="text-xs text-muted mb-2">Damage multiplier by quality</div>
        {(qualityData?.weapon_damage ?? []).map((mult, i) => (
          <Row
            key={i}
            label={QUALITY_LABELS[i] ?? `Q${i}`}
            value={`${mult.toFixed(2)}×`}
          />
        ))}
      </Panel>
    )
  }

  const renderMarketPanel = (): React.ReactNode => {
    if (market === undefined) return null
    return (
      <Panel>
        <SectionLabel>Active Listings</SectionLabel>
        {market.listingsLoading
          ? (
              <div className="flex justify-center py-4"><Spinner size="sm" /></div>
            )
          : listings.length === 0
            ? (
                <p className="text-xs text-muted">No active listings.</p>
              )
            : (
                <div className="flex flex-col gap-3">
                  {qualities.map((q) => (
                    <div key={q}>
                      <div className="text-xs font-medium text-muted mb-1">{qualityLabel(q)}</div>
                      <DataTable<MarketListing, 'seller' | 'stock' | 'price'>
                        aria-label={`Active Listings — ${qualityLabel(q)}`}
                        columns={[
                          { key: 'seller', label: 'Seller', isRowHeader: true },
                          { key: 'stock', label: 'Stock', align: 'end', width: 80 },
                          { key: 'price', label: 'Price', align: 'end', width: 100 },
                        ]}
                        rows={byQuality[q]}
                        rowId={(l) => String(l.order_id)}
                        initialSort={{ column: 'price', direction: 'ascending' }}
                        sortValue={(l, k) => (k === 'seller' ? l.owner_name : l[k])}
                        renderCell={(l, k) => {
                          switch (k) {
                            case 'seller':
                              return (
                                <span className={`truncate block ${l.owner_type === 'bot' ? 'text-accent' : 'text-foreground'}`}>
                                  {l.owner_name}
                                </span>
                              )
                            case 'stock':
                              return <span className="text-muted tabular-nums">{l.stock.toLocaleString()}</span>
                            case 'price':
                              return <span className="font-mono">{l.price.toLocaleString()}</span>
                          }
                        }}
                      />
                    </div>
                  ))}
                </div>
              )}
      </Panel>
    )
  }

  return (
    <div className="flex flex-col gap-3">

      {/* Header: icon + name + template id + chips */}
      <div className="flex gap-3 items-start">
        <div
          className={`w-16 h-16 shrink-0 rounded-lg border-2 ${borderClass} flex items-center justify-center overflow-hidden`}
          style={{ background: categoryColor(entry?.category ?? '', entry?.rarity?.toLowerCase(), templateId) }}
        >
          {img
            ? (
                <img
                  src={img}
                  alt={displayName}
                  className="w-full h-full object-contain p-1.5"
                  onError={(e) => {
                    (e.currentTarget as HTMLImageElement).style.display = 'none'
                  }}
                />
              )
            : (
                <span className="text-2xl text-white/20 font-bold uppercase select-none">
                  {displayName.charAt(0)}
                </span>
              )}
        </div>

        <div className="flex-1 min-w-0 flex flex-col gap-1">
          <div className="font-semibold text-sm text-foreground leading-tight">{displayName}</div>
          <div className="font-mono text-[10px] text-muted leading-none">{templateId}</div>
          <div className="flex flex-wrap gap-1 mt-0.5">
            {renderRarityChips()}
          </div>
        </div>
      </div>

      {/* Description */}
      {entry?.description && (
        <p className="text-xs text-muted leading-relaxed">{entry.description}</p>
      )}

      {/* Item Info Panel — only rendered when it has at least one row */}
      {renderItemInfoPanel()}

      {/* Armor Stats */}
      {renderArmorPanel()}

      {/* Weapon Quality Scaling */}
      {renderWeaponPanel()}

      {/* Active Listings (market only) */}
      {renderMarketPanel()}
    </div>
  )
}
