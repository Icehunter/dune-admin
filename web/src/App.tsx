import * as React from 'react'
import { Spinner, Toast } from '@heroui/react'
import { AuthContext } from './auth/context'
import { LoginPage } from './auth/LoginPage'
import { AppWithAuth } from './components/app/AppWithAuth'
import { AppCore } from './components/app/AppCore'

const hasClerk = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY

export const App: React.FC = (): React.ReactElement => {
  const auth = React.useContext(AuthContext)

  // Backend auth gate (self-host login). Independent of Clerk, which only
  // exists on the hosted CDN deploy. When auth is disabled (default) this
  // renders exactly the pre-auth app.
  if (auth.loading) {
    return (
      <div className="h-screen flex items-center justify-center bg-background">
        <Spinner />
      </div>
    )
  }
  if (auth.enabled && !auth.session) {
    return (
      <React.Fragment>
        <Toast.Provider />
        <LoginPage />
      </React.Fragment>
    )
  }

  return hasClerk ? <AppWithAuth /> : <AppCore isSignedIn={true} />
}
