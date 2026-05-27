/**
 * Dune Admin component library — opinionated wrappers around HeroUI v3 that
 * carry the project's amber/dark aesthetic. Import from here, not from
 * @heroui/react directly, when there's an equivalent dune-ui wrapper.
 *
 * Side effect: importing this module registers the lucide icon collection
 * with iconify so `<Icon name="..." />` works offline.
 */
import './icons'

export { Icon } from './Icon'
export { PageHeader } from './PageHeader'
export { InfoCard } from './InfoCard'
export { SectionDivider } from './SectionDivider'
export { SectionLabel } from './SectionLabel'
export { Panel } from './Panel'
export { StatusChip } from './StatusChip'
export type { StatusKind } from './StatusChip'
export { DataTable } from './DataTable'
export type { Column } from './DataTable'
export { Dropzone } from './Dropzone'
export { SideNav } from './SideNav'
export { ConfirmDialog } from './ConfirmDialog'
