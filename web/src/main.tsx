import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { HashRouter } from 'react-router-dom'
import './index.css'
import './i18n'
import App from './App.tsx'
import { ClerkProvider } from '@clerk/react'
import { dark } from '@clerk/themes'

const publishableKey = import.meta.env.VITE_CLERK_PUBLISHABLE_KEY

// Match Clerk modals to the dune-admin dark amber theme.
// Element class overrides are needed for the backdrop because Clerk injects
// it via inline style — the appearance.elements className alone doesn't win.
const clerkAppearance = {
  baseTheme: dark,
  variables: {
    colorPrimary: '#c9820a',
    colorDanger: '#c9230a',
    borderRadius: '2px',
    fontFamily: 'system-ui, -apple-system, sans-serif',
  },
  elements: {
    formButtonPrimary:
      'bg-[#c9820a] hover:bg-[#d4900f] text-black font-bold shadow-none normal-case tracking-normal',
    footerActionLink: 'text-[#c9820a] hover:text-[#d4900f]',
  },
} as const

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <HashRouter>
      {publishableKey
        ? (
            <ClerkProvider publishableKey={publishableKey} afterSignOutUrl="/" appearance={clerkAppearance}>
              <App />
            </ClerkProvider>
          )
        : (
            <App />
          )}
    </HashRouter>
  </StrictMode>,
)
