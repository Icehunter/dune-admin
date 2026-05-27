import { useEffect, useState } from 'react'
import { Button, Spinner } from '@heroui/react'
import { api } from '../../api/client'
import type { MarketItem, MarketListing } from '../../api/client'
import { Icon, Panel, SectionLabel } from '../../dune-ui'
import { iconUrl, qualityLabel } from '../../utils/icons'
import { getItemEntry } from '../../data/itemData'
import qualityData from '../../data/quality-data.json'

type ItemEntry = {
  is_gradeable?: boolean
  armor_value?: number
  mitigation?: Record<string, number>
}

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

type Props = {
  item: MarketItem | null
  onClose: () => void
}

export default function ItemDetail({ item, onClose }: Props) {
  const [listings, setListings] = useState<MarketListing[]>([])
  const [loading, setLoading] = useState(false)
  const [entry, setEntry] = useState<ItemEntry | null>(null)

  useEffect(() => {
    if (!item) return
    setListings([])
    setEntry(null)
    setLoading(true)
    Promise.all([
      api.market.listings(item.template_id),
      getItemEntry(item.template_id),
    ])
      .then(([ls, e]) => { setListings(ls); setEntry(e) })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [item?.template_id])

  if (!item) return null

  const img = iconUrl(item.template_id, 'thumb')
  const byQuality = listings.reduce<Record<number, MarketListing[]>>((acc, l) => {
    ;(acc[l.quality] ??= []).push(l)
    return acc
  }, {})
  const qualities = Object.keys(byQuality).map(Number).sort((a, b) => a - b)

  const isArmor = !!entry?.armor_value
  const isWeapon = item.category?.startsWith('items/weapons')
  const isGradeable = entry?.is_gradeable

  return (
    <div className="w-80 shrink-0 flex flex-col border-l border-border bg-surface overflow-y-auto">
      <div className="flex items-center justify-between px-4 py-3 border-b border-border shrink-0">
        <div className="flex items-center gap-2 min-w-0">
          {img && (
            <img
              src={img}
              alt=""
              className="w-8 h-8 object-contain shrink-0 rounded"
              onError={e => { (e.currentTarget as HTMLImageElement).style.display = 'none' }}
            />
          )}
          <span className="font-semibold text-sm text-accent truncate">
            {item.display_name || item.template_id}
          </span>
        </div>
        <Button size="sm" variant="ghost" isIconOnly aria-label="Close" onPress={onClose}>
          <Icon name="x" />
        </Button>
      </div>

      <div className="flex flex-col gap-3 p-3">
        <Panel>
          <SectionLabel>Item Info</SectionLabel>
          <Row label="Category" value={item.category || '—'} />
          <Row label="Tier" value={item.tier > 0 ? String(item.tier) : '—'} />
          <Row label="Rarity" value={item.rarity || '—'} />
          <Row label="Total Stock" value={item.total_stock.toLocaleString()} />
          <Row label="Bot Stock" value={item.bot_stock.toLocaleString()} />
          <Row label="Listings" value={String(item.listing_count)} />
          <Row label="Lowest Price" value={item.lowest_price.toLocaleString()} accent />
        </Panel>

        {isArmor && (
          <Panel>
            <SectionLabel>Armor Stats</SectionLabel>
            {isGradeable ? (
              <>
                <div className="text-xs text-muted mb-1">Armor Value by Quality</div>
                <table className="w-full text-xs mb-2">
                  <thead>
                    <tr className="text-muted">
                      {QUALITY_LABELS.map((ql, i) => (
                        <th key={i} className="text-center pb-1 font-normal">{ql.slice(0, 3)}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      {qualityData.armor.map((mult, i) => (
                        <td key={i} className="text-center font-mono text-foreground">
                          {Math.round(entry!.armor_value! * mult)}
                        </td>
                      ))}
                    </tr>
                  </tbody>
                </table>
              </>
            ) : (
              <Row label="Armor Value" value={String(entry!.armor_value)} />
            )}
            {entry?.mitigation && Object.keys(entry.mitigation).length > 0 && (
              <>
                <div className="text-xs text-muted mt-1 mb-1">Resistances</div>
                {Object.entries(entry.mitigation).map(([k, v]) => (
                  <Row
                    key={k}
                    label={MITIGATION_LABELS[k] ?? k}
                    value={`${Math.round(v * 100)}%`}
                  />
                ))}
              </>
            )}
          </Panel>
        )}

        {isWeapon && isGradeable && (
          <Panel>
            <SectionLabel>Weapon Quality Scaling</SectionLabel>
            <div className="text-xs text-muted mb-1">Damage multiplier by quality</div>
            <table className="w-full text-xs">
              <thead>
                <tr className="text-muted">
                  {QUALITY_LABELS.map((ql, i) => (
                    <th key={i} className="text-center pb-1 font-normal">{ql.slice(0, 3)}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                <tr>
                  {qualityData.weapon_damage.map((mult, i) => (
                    <td key={i} className="text-center font-mono text-foreground">
                      {mult.toFixed(2)}×
                    </td>
                  ))}
                </tr>
              </tbody>
            </table>
          </Panel>
        )}

        <Panel>
          <SectionLabel>Active Listings</SectionLabel>
          {loading ? (
            <div className="flex justify-center py-4"><Spinner size="sm" /></div>
          ) : listings.length === 0 ? (
            <p className="text-xs text-muted">No active listings.</p>
          ) : (
            <div className="flex flex-col gap-3">
              {qualities.map(q => (
                <div key={q}>
                  <div className="text-xs font-medium text-muted mb-1">{qualityLabel(q)}</div>
                  <table className="w-full text-xs">
                    <thead>
                      <tr className="text-muted">
                        <th className="text-left pb-1 font-normal">Seller</th>
                        <th className="text-right pb-1 font-normal">Stock</th>
                        <th className="text-right pb-1 font-normal">Price</th>
                      </tr>
                    </thead>
                    <tbody>
                      {byQuality[q].sort((a, b) => a.price - b.price).map(l => (
                        <tr key={l.order_id} className="border-t border-border/40">
                          <td className={`py-0.5 ${l.owner_type === 'bot' ? 'text-accent' : 'text-foreground'}`}>
                            {l.owner_name}
                          </td>
                          <td className="py-0.5 text-right text-muted">{l.stock.toLocaleString()}</td>
                          <td className="py-0.5 text-right font-mono">{l.price.toLocaleString()}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ))}
            </div>
          )}
        </Panel>
      </div>
    </div>
  )
}

function Row({ label, value, accent }: { label: string; value: string; accent?: boolean }) {
  return (
    <div className="flex items-center justify-between text-xs py-0.5">
      <span className="text-muted">{label}</span>
      <span className={accent ? 'font-mono text-accent font-semibold' : 'text-foreground'}>{value}</span>
    </div>
  )
}
