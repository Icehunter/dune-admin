import * as React from 'react'
import { api } from '../api/client'
import type { Status } from '../api/client'
import type { ConnState, StatusResult } from './types'

export type { ConnState, StatusResult }

export const useStatus = (): StatusResult => {
  const [status, setStatus] = React.useState<Status | null>(null)
  const [state, setState] = React.useState<ConnState>('loading')

  React.useEffect(() => {
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
