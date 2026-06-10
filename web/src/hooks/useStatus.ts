import { useState, useEffect } from 'react'
import { api } from '../api/client'
import type { Status } from '../api/client'

// ConnState distinguishes the initial load from a hard "never reached the
// backend" failure, so the UI can show a setup screen on real connection
// failure without flickering during the first poll.
export type ConnState = 'loading' | 'connected' | 'error'

export interface StatusResult {
  status: Status | null
  state: ConnState
}

export function useStatus(): StatusResult {
  const [status, setStatus] = useState<Status | null>(null)
  const [state, setState] = useState<ConnState>('loading')

  useEffect(() => {
    let everConnected = false
    const poll = async () => {
      try {
        const s = await api.status()
        everConnected = true
        setStatus(s)
        setState('connected')
      }
      catch {
        // Only surface the hard "can't reach backend" screen if we've NEVER
        // connected. A transient blip after a successful connect keeps the last
        // status — the header's DB/SSH badges already reflect dependency health.
        if (!everConnected) {
          setStatus(null)
          setState('error')
        }
      }
    }
    poll()
    const id = setInterval(poll, 5000)
    return () => clearInterval(id)
  }, [])

  return { status, state }
}
