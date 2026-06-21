import type { MarketListing } from '../api/client'
import type { ItemEntry } from '../data/store'

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
  market?: MarketDetail
}
