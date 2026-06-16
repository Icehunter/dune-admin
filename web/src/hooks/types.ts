import type { Status } from '../api/client'

// ConnState distinguishes the initial load from a hard "never reached the
// backend" failure, so the UI can show a setup screen on real connection
// failure without flickering during the first poll.
export type ConnState = 'loading' | 'connected' | 'error'

export interface StatusResult {
  status: Status | null
  state: ConnState
  /** Force an immediate status re-fetch (after server switch/delete/setup). */
  refresh: () => Promise<void>
}
