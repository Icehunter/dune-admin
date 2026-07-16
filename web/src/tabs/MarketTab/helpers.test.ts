import { describe, it, expect, vi } from 'vitest'
import { fetchAllMarketItems } from './helpers'
import type { MarketItem, MarketItemsParams, MarketItemsResponse } from '../../api/client'

const item = (template_id: string): MarketItem => ({
  template_id,
  display_name: template_id,
  category: 'weapons',
  tier: 1,
  rarity: 'common',
  quality: 0,
  lowest_price: 10,
  total_stock: 1,
  bot_stock: 0,
  listing_count: 1,
  icon: null,
})

describe('fetchAllMarketItems', () => {
  it('returns every item without looping when everything fits on one page', async () => {
    const items = [item('a'), item('b')]
    const fetchPage = vi.fn(
      async (): Promise<MarketItemsResponse> => ({ items, total: 2, page: 0, limit: 500 }),
    )

    const result = await fetchAllMarketItems(fetchPage, {})

    expect(result).toEqual(items)
    expect(fetchPage).toHaveBeenCalledTimes(1)
  })

  it('loops until every item is collected when the board exceeds one page — the #287 bug', async () => {
    const pages: Record<number, MarketItem[]> = {
      0: Array.from({ length: 500 }, (_, i) => item(`t${i}`)),
      1: Array.from({ length: 500 }, (_, i) => item(`t${i + 500}`)),
      2: [item('t1000')],
    }
    const fetchPage = vi.fn(
      async (params: MarketItemsParams): Promise<MarketItemsResponse> => ({
        items: pages[params.page ?? 0] ?? [],
        total: 1001,
        page: params.page ?? 0,
        limit: 500,
      }),
    )

    const result = await fetchAllMarketItems(fetchPage, {})

    expect(result).toHaveLength(1001)
    expect(fetchPage).toHaveBeenCalledTimes(3)
    expect(fetchPage).toHaveBeenNthCalledWith(1, expect.objectContaining({ page: 0, limit: 500 }))
    expect(fetchPage).toHaveBeenNthCalledWith(2, expect.objectContaining({ page: 1, limit: 500 }))
    expect(fetchPage).toHaveBeenNthCalledWith(3, expect.objectContaining({ page: 2, limit: 500 }))
  })

  it('passes search/category/owner filters through on every page request', async () => {
    const fetchPage = vi.fn(
      async (): Promise<MarketItemsResponse> => ({ items: [item('a')], total: 1, page: 0, limit: 500 }),
    )

    await fetchAllMarketItems(fetchPage, { search: 'spice', category: 'weapons', owner: 'player' })

    expect(fetchPage).toHaveBeenCalledWith(
      expect.objectContaining({ search: 'spice', category: 'weapons', owner: 'player' }),
    )
  })

  it('stops on an empty page even if total implies more, to avoid looping forever on a bad count', async () => {
    const fetchPage = vi.fn(
      async (params: MarketItemsParams): Promise<MarketItemsResponse> => ({
        items: (params.page ?? 0) === 0 ? [item('a')] : [],
        total: 999,
        page: params.page ?? 0,
        limit: 500,
      }),
    )

    const result = await fetchAllMarketItems(fetchPage, {})

    expect(result).toEqual([item('a')])
    expect(fetchPage).toHaveBeenCalledTimes(2)
  })

  it('returns an empty array when the board has no items', async () => {
    const fetchPage = vi.fn(
      async (): Promise<MarketItemsResponse> => ({ items: [], total: 0, page: 0, limit: 500 }),
    )

    const result = await fetchAllMarketItems(fetchPage, {})

    expect(result).toEqual([])
    expect(fetchPage).toHaveBeenCalledTimes(1)
  })
})
