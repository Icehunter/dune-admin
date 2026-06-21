import * as React from 'react'
import { ApiError, api } from '../api/client'
import type { Status } from '../api/client'
import type { StatusResult } from './interfaces'
import type { ConnState } from './types'

export type { ConnState, StatusResult }

export const useStatus = (): StatusResult => {
  const [status, setStatus] = React.useState<Status | null>(null)
  const [state, setState] = React.useState<ConnState>('loading')
  const everConnected = React.useRef(false)

  const poll = async (): Promise<void> => {
    try {
      const s = await api.status()
      everConnected.current = true
      setStatus(s)
      setState('connected')
    }
    catch (e) {
      // A 401/403 means the backend IS reachable but auth/permissions block
      // the status read — render the app shell (tabs gate themselves), never
      // the "can't reach backend" screen, which would trap the user.
      if (e instanceof ApiError && (e.status === 401 || e.status === 403)) {
        everConnected.current = true
        setState('connected')
        return
      }
      // Only surface the hard "can't reach backend" screen if we've NEVER
      // connected. A transient blip after a successful connect keeps the last
      // status — the header's DB/SSH badges already reflect dependency health.
      if (!everConnected.current) {
        setStatus(null)
        setState('error')
      }
    }
  }

  React.useEffect(() => {
    // Defer the first poll a microtask so the synchronous setState-in-effect
    // lint rule is satisfied (poll() updates state).
    void Promise.resolve().then(poll)
    const id = setInterval(() => void poll(), 5000)
    return () => clearInterval(id)
  }, [])

  return { status, state, refresh: poll }
}
