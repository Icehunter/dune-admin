import type { Section } from './constants'

export type TableData = { headers: string[], rows: string[][] }

export interface DatabaseTabProps {
  showSubnav?: boolean
  section?: Section
  onSectionChange?: (s: Section) => void
}
