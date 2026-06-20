import * as React from 'react'
import { useAuth } from '@clerk/react'
import { AppCore } from './AppCore'

export const AppWithAuth: React.FC = (): React.ReactElement => {
  const { isSignedIn } = useAuth()
  return <AppCore isSignedIn={!!isSignedIn} />
}
