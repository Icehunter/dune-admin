export type ItemKey = 'id' | 'template' | 'stack_size' | 'quality' | 'durability' | 'actions'

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
