import { useEffect, useState } from 'react'
import { Button, Spinner } from '@heroui/react'
import { api } from '../../api/client'
import type { MarketItem, MarketListing } from '../../api/client'
import { Icon } from '../../dune-ui'
import { iconUrl, qualityLabel } from '../../utils/icons'

type Props = {
  item: MarketItem | null
  onClose: () => void
}

export default function ItemDetail({ item, onClose }: Props) {
  const [listings, setListings] = useState<MarketListing[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!item) return
    setListings([])
    setLoading(true)
    api.market.listings(item.template_id)
      .then(setListings)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [item?.template_id])

  if (!item) return null

  const img = iconUrl(item.template_id)
  const byQuality = listings.reduce<Record<number, MarketListing[]>>((acc, l) => {
    ;(acc[l.quality] ??= []).push(l)
    return acc
  }, {})
  const qualities = Object.keys(byQuality).map(Number).sort((a, b) => a - b)

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

      <div className="px-4 py-3 flex flex-col gap-1 border-b border-border shrink-0">
        <Row label="Category" value={item.category || '—'} />
        <Row label="Tier" value={item.tier > 0 ? String(item.tier) : '—'} />
        <Row label="Rarity" value={item.rarity || '—'} />
        <Row label="Total Stock" value={item.total_stock.toLocaleString()} />
        <Row label="Bot Stock" value={item.bot_stock.toLocaleString()} />
        <Row label="Listings" value={String(item.listing_count)} />
        <Row label="Lowest Price" value={item.lowest_price.toLocaleString()} accent />
      </div>

      <div className="px-4 py-3 flex flex-col gap-3 min-h-0">
        <span className="text-xs font-semibold text-muted uppercase tracking-wider">Active Listings</span>

        {loading ? (
          <div className="flex justify-center py-6"><Spinner size="sm" /></div>
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
      </div>
    </div>
  )
}

function Row({ label, value, accent }: { label: string; value: string; accent?: boolean }) {
  return (
    <div className="flex items-center justify-between text-xs">
      <span className="text-muted">{label}</span>
      <span className={accent ? 'font-mono text-accent font-semibold' : 'text-foreground'}>{value}</span>
    </div>
  )
}
