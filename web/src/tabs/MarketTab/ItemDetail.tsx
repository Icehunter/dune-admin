import * as React from 'react'
import { Drawer } from '@heroui/react'
import { api } from '../../api/client'
import type { MarketListing } from '../../api/client'
import { getItemEntry } from '../../data/itemData'
import { ItemDetailCard } from '../../components/ItemDetailCard'
import type { ItemDetailProps } from './interfaces'

export const ItemDetail: React.FC<ItemDetailProps> = ({ item, onClose }) => {
  const [listings, setListings] = React.useState<MarketListing[]>([])
  const [listingsLoading, setListingsLoading] = React.useState(false)
  const [entry, setEntry] = React.useState<Awaited<ReturnType<typeof getItemEntry>>>(null)

  React.useEffect(() => {
    if (!item) return
    Promise.resolve()
      .then(() => {
        setListings([])
        setEntry(null)
        setListingsLoading(true)
      })
      .then(() => Promise.all([
        api.market.listings(item.template_id),
        getItemEntry(item.template_id),
      ]))
      .then(([ls, e]) => {
        setListings(ls)
        setEntry(e)
      })
      .catch(() => {})
      .finally(() => setListingsLoading(false))
  }, [item])

  return (
    <Drawer.Backdrop variant="opaque" isOpen={!!item} onOpenChange={(v) => !v && onClose()}>
      <Drawer.Content placement="right">
        <Drawer.Dialog className="w-[480px] max-w-[95vw] flex flex-col">
          <Drawer.Header>
            <div className="flex items-center gap-2 px-4 py-3 border-b border-border w-full">
              <Drawer.Heading className="font-semibold text-sm text-accent truncate flex-1">
                {item?.display_name || item?.template_id || ''}
              </Drawer.Heading>
              <Drawer.CloseTrigger />
            </div>
          </Drawer.Header>
          <Drawer.Body className="flex flex-col gap-3 p-3 overflow-y-auto">
            {item && (
              <ItemDetailCard
                templateId={item.template_id}
                name={item.display_name}
                entry={entry}
                market={{
                  lowestPrice: item.lowest_price,
                  totalStock: item.total_stock,
                  botStock: item.bot_stock,
                  listingCount: item.listing_count,
                  listings,
                  listingsLoading,
                }}
              />
            )}
          </Drawer.Body>
        </Drawer.Dialog>
      </Drawer.Content>
    </Drawer.Backdrop>
  )
}
