import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { HashRouter } from 'react-router-dom'
import './index.css'
import './i18n'
import { App } from './App.tsx'
import { ClerkProvider } from '@clerk/react'
import { dark } from '@clerk/themes'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const publishableKey = import.meta.env.VITE_CLERK_PUBLISHABLE_KEY

// Shared React Query client. The backend pushes no real-time updates, so
// refetch-on-focus is off; tabs that need polling set their own refetchInterval.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 5_000,
      refetchOnWindowFocus: false,
    },
  },
})

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
      <QueryClientProvider client={queryClient}>
        {publishableKey
          ? (
              <ClerkProvider publishableKey={publishableKey} afterSignOutUrl="/" appearance={clerkAppearance}>
                <App />
              </ClerkProvider>
            )
          : (
              <App />
            )}
      </QueryClientProvider>
    </HashRouter>
  </StrictMode>,
)
