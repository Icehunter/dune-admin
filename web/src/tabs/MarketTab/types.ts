import type { MarketItem, BotConfig } from '../../api/client'

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
  canManageBot: boolean
  botConfig: BotConfig | null
  onItemDisabled: (cfg: BotConfig) => void
}

export type MarketTableKey = 'display_name' | 'quality' | 'category' | 'tier' | 'rarity' | 'lowest_price' | 'total_stock' | 'listing_count' | 'actions'

export type MarketTableProps = {
  items: MarketItem[]
  onSelect: (item: MarketItem) => void
  canManageBot: boolean
  botConfig: BotConfig | null
  onItemDisabled: (cfg: BotConfig) => void
}

export type DisableItemActionVariant = 'button' | 'icon'

export type DisableItemActionProps = {
  item: MarketItem
  botConfig: BotConfig | null
  canManage: boolean
  onDisabled: (cfg: BotConfig) => void
  variant: DisableItemActionVariant
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
