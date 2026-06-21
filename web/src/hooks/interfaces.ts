import type { Status } from '../api/client'
import type { ConnState } from './types'

export interface StatusResult {
  status: Status | null
  state: ConnState
  /** Force an immediate status re-fetch (after server switch/delete/setup). */
  refresh: () => Promise<void>
}
