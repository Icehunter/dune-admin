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
    colorPrimary: '#1574e5',
    colorDanger: '#ff5a5f',
    borderRadius: '10px',
    fontFamily: '"Manrope Variable", system-ui, -apple-system, sans-serif',
  },
  elements: {
    formButtonPrimary:
      'bg-[#1574e5] hover:bg-[#2a82ea] text-white font-bold shadow-none normal-case tracking-normal',
    footerActionLink: 'text-[#5aa2f0] hover:text-[#7db4f4]',
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
