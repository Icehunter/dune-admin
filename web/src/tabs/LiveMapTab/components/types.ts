import * as React from 'react'
import { CATEGORY_GROUPS } from '../constants'

export type TypeRowProps = {
  typeKey: string
  label: string
  count: number
  category: string
  filter: Record<string, boolean>
  onToggle: (key: string, currentVisual: boolean) => void
}

export type CategorySectionProps = {
  group: (typeof CATEGORY_GROUPS)[number]
  typesByCategory: Record<string, Map<string, { label: string, count: number }>>
  expanded: Record<string, boolean>
  setExpanded: React.Dispatch<React.SetStateAction<Record<string, boolean>>>
  search: string
  filter: Record<string, boolean>
  onToggle: (key: string, currentVisual: boolean) => void
}
