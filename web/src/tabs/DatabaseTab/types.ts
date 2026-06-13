import type { Section } from './constants'

export type TableData = { headers: string[], rows: string[][] }

export interface DatabaseTabProps {
  section?: Section
}
