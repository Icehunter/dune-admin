import { useState, useEffect } from 'react'
import { api } from '../api/client'
import type { Status } from '../api/client'

export function useStatus() {
  const [status, setStatus] = useState<Status | null>(null)

  useEffect(() => {
    const poll = async () => {
      try {
        const s = await api.status()
        setStatus(s)
      }
      catch {
        setStatus(null)
      }
    }
    poll()
    const id = setInterval(poll, 5000)
    return () => clearInterval(id)
  }, [])

  return status
}
