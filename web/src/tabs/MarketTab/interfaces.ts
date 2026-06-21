import type { MarketItem } from '../../api/client'

export interface ItemDetailProps {
  item: MarketItem | null
  onClose: () => void
}

export interface RowProps {
  label: string
  value: string
  accent?: boolean
  wrap?: boolean
}
