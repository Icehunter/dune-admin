import { SideNav } from '../../dune-ui'
import { SIDEBAR_ITEMS, type Sidebar as SidebarKey } from './types'

interface Props {
  active: SidebarKey
  onSelect: (key: SidebarKey) => void
}

export function Sidebar({ active, onSelect }: Props) {
  return (
    <SideNav
      items={SIDEBAR_ITEMS}
      active={active}
      onSelect={onSelect}
      title="Players"
    />
  )
}
