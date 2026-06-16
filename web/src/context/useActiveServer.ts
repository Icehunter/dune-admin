import * as React from 'react'
import { ActiveServerContext } from './ActiveServerContext'
import type { ActiveServerContextValue } from './ActiveServerContext'

export function useActiveServer(): ActiveServerContextValue {
  return React.useContext(ActiveServerContext)
}
