import { CATEGORY_GROUPS } from '../constants'

export type TypeRowProps = { typeKey: string, label: string, count: number, category: string }

export type CategorySectionProps = { group: (typeof CATEGORY_GROUPS)[number] }
