import type { Status } from '../../../api/client'
import type { BGInfo, ServerRow } from '../types'

export type HealthProps = { bg?: BGInfo, servers: ServerRow[], status: Status | null }
