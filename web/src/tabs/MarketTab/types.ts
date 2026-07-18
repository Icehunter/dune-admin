import type { MarketItem } from '../../api/client'

export type MarketView = 'grid' | 'table'

export type ViewToggleProps = {
  view: MarketView
  onChange: (v: MarketView) => void
}

export type MarketSidebarProps = {
  categories: string[]
  selected: string
  onSelect: (cat: string) => void
}

export type Node = {
  label: string
  path: string // full path used for filtering — also the FileTree item key
  displayPath: string // path used as tree key (items/ stripped)
  children: Node[]
}

export type MarketGridProps = {
  items: MarketItem[]
  onSelect: (item: MarketItem) => void
}

export type MarketTableKey = 'display_name' | 'quality' | 'category' | 'tier' | 'rarity' | 'lowest_price' | 'total_stock' | 'listing_count'

export type MarketTableProps = {
  items: MarketItem[]
  onSelect: (item: MarketItem) => void
}

export type MarketFilters = {
  search: string
  category: string
  owner: '' | 'bot' | 'player'
}

export type MarketSearchProps = {
  filters: MarketFilters
  onChange: (f: MarketFilters) => void
  onReset: () => void
}

export type QualityArcProps = {
  quality: number
  size?: number
}
