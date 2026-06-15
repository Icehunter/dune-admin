import * as React from 'react'

export type ActiveServerContextValue = {
  servers: { id: number, name: string, active: boolean }[]
  /** Numeric id of the active server; 0 when none is selected. */
  activeID: number
  /** True until the first server-list fetch resolves (avoids an empty-state flash). */
  loading: boolean
  setActive: (id: number) => Promise<void>
  removeServer: (id: number) => Promise<void>
  refresh: () => Promise<void>
}

export const ActiveServerContext = React.createContext<ActiveServerContextValue>({
  servers: [],
  activeID: 0,
  loading: true,
  setActive: async () => {},
  removeServer: async () => {},
  refresh: async () => {},
})
