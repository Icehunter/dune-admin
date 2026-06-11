export type Container = {
  id: number
  name: string
  class: string
  map: string
  item_count: number
  item_templates: string[]
  item_names: string[]
  owner_name: string
}

export interface AddItemsModalProps {
  container: Container
  open: boolean
  onClose: () => void
  onSuccess: () => void
  onRefresh: () => void
}

export type AddResult = { given: string[], skipped: { template: string, reason: string }[] } | null
