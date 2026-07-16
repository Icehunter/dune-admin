import type { MarketItem, MarketItemsParams, MarketItemsResponse } from '../../api/client'

const MAX_PAGE_SIZE = 500

export const fetchAllMarketItems = async (
  fetchPage: (params: MarketItemsParams) => Promise<MarketItemsResponse>,
  params: Omit<MarketItemsParams, 'page' | 'limit'>,
): Promise<MarketItem[]> => {
  const items: MarketItem[] = []
  let page = 0
  for (;;) {
    const res = await fetchPage({ ...params, page, limit: MAX_PAGE_SIZE })
    if (res.items.length === 0) break
    items.push(...res.items)
    if (items.length >= res.total) break
    page += 1
  }
  return items
}
