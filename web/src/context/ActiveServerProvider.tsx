import * as React from 'react'
import { api, getActiveServerID, setActiveServerID } from '../api/client'
import type { ServerInfo } from '../api/client'
import { ActiveServerContext } from './ActiveServerContext'
import type { ActiveServerContextValue } from './ActiveServerContext'
import { AuthContext } from '../auth/context'

export const ActiveServerProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const auth = React.useContext(AuthContext)
  const [servers, setServers] = React.useState<ServerInfo[]>([])
  // activeID is numeric (0 = none); the X-Dune-Server header carries its decimal
  // string form, which the client persists in localStorage.
  const [activeID, setActiveID] = React.useState<number>(() => Number(getActiveServerID()) || 0)
  const [loading, setLoading] = React.useState(true)

  const refresh = React.useCallback(async () => {
    const list = await api.servers.list().catch(() => [] as ServerInfo[])
    setServers(list)
    setLoading(false)
    // Self-heal a stale persisted active server. localStorage is per-origin, so
    // the Vite dev server (:5173) and the embedded SPA (:8080) keep separate
    // copies; a deleted/recreated server leaves an id that no longer exists in
    // the registry. Without this, every request keeps sending a dead
    // X-Dune-Server header and the backend rejects it with 404.
    const current = Number(getActiveServerID()) || 0
    if (current && !list.some((s) => s.id === current)) {
      const fallback = list.find((s) => s.active)?.id ?? 0
      setActiveServerID(fallback ? String(fallback) : '')
      setActiveID(fallback)
    }
  }, [])

  // Fetch (and re-fetch) the server list once auth has resolved and whenever the
  // session changes. The provider wraps the auth gate, so its first render runs
  // before login — fetching then would return an unauthenticated/empty list and
  // leave the dashboard stale until a manual refresh. Keying on auth.loading +
  // auth.session guarantees an authenticated fetch right after login (and a
  // refetch on logout).
  const hasSession = !!auth.session
  React.useEffect(() => {
    if (auth.loading) return
    void Promise.resolve().then(refresh)
  }, [auth.loading, hasSession, refresh])

  const setActive = React.useCallback(async (id: number) => {
    // Changing the process-wide active server requires server:control; guests
    // (read-only) still get client-side scoping via the X-Dune-Server header, so
    // a rejected backend call must not block the view switch.
    await api.servers.setActive(id).catch(() => {})
    setActiveServerID(String(id))
    setActiveID(id)
    setServers((prev) => prev.map((s) => ({ ...s, active: s.id === id })))
  }, [])

  const removeServer = React.useCallback(async (id: number) => {
    await api.servers.remove(id)
    // Refetch the authoritative list and reconcile the active id. Deleting the
    // active server (backend reassigns active) or the last server (registry
    // empties → setup) both resolve here, so callers don't special-case them.
    await refresh()
  }, [refresh])

  const value = React.useMemo<ActiveServerContextValue>(
    () => ({ servers, activeID, loading, setActive, removeServer, refresh }),
    [servers, activeID, loading, setActive, removeServer, refresh],
  )

  return <ActiveServerContext.Provider value={value}>{children}</ActiveServerContext.Provider>
}
