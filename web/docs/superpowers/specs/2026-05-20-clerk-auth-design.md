# Clerk Auth Integration — Design Spec

**Date:** 2026-05-20
**Status:** Approved

## Overview

Add Clerk authentication to the Dune Admin React (Vite) app. Auth gates the Blueprints tab and adds user identity controls to the header. All other tabs remain publicly accessible. API requests forward the Clerk JWT when available, for future backend enforcement.

## Scope

- Install `@clerk/react@latest`
- Wrap app in `<ClerkProvider>`
- Add `<UserButton>` / `<SignInButton>` to header (alongside existing controls)
- Hide Blueprints tab and panel from unauthenticated users
- Forward Clerk JWT as `Authorization: Bearer` on all API requests

**Out of scope:** Cloudflare DB integration, Go backend JWT verification, per-user blueprint ownership.

## Files Changed

| File | Change |
|---|---|
| `.env.local` | Add `VITE_CLERK_PUBLISHABLE_KEY` |
| `src/main.tsx` | Wrap `<App />` in `<ClerkProvider afterSignOutUrl="/">` |
| `src/App.tsx` | Add auth controls to header; gate Blueprints tab/panel with `<Show>` |
| `src/api/client.ts` | Forward JWT via `window.Clerk?.session?.getToken()` in `req()` |

No new files. No tab components touched other than how they are conditionally rendered.

## Design Details

### Environment

`.env.local` (not committed):

```
VITE_CLERK_PUBLISHABLE_KEY=pk_...
```

### `main.tsx`

```tsx
import { ClerkProvider } from '@clerk/react'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ClerkProvider afterSignOutUrl="/">
      <App />
    </ClerkProvider>
  </StrictMode>
)
```

### `App.tsx` — Header

The header structure is unchanged. Auth controls are appended to the right side, after the existing gear button:

```tsx
import { Show, SignInButton, UserButton } from '@clerk/react'

// In the header's right-side flex container:
<Show when="signed-out"><SignInButton /></Show>
<Show when="signed-in"><UserButton /></Show>
```

### `App.tsx` — Blueprints Tab

The Blueprints `<Tabs.Tab>` and `<Tabs.Panel>` are wrapped in `<Show when="signed-in">`. All other tabs are untouched.

```tsx
<Show when="signed-in">
  <Tabs.Tab id="blueprints">Blueprints<Tabs.Indicator /></Tabs.Tab>
</Show>

// ...

<Show when="signed-in">
  <Tabs.Panel id="blueprints" className="flex-1 overflow-hidden flex flex-col p-4">
    <BlueprintsTab />
  </Tabs.Panel>
</Show>
```

### `api/client.ts` — JWT Forwarding

Add a `window.Clerk` type declaration and update `req()` to attach the token when available:

```ts
declare global {
  interface Window {
    Clerk?: { session?: { getToken(): Promise<string | null> } }
  }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const token = await window.Clerk?.session?.getToken()
  const headers: Record<string, string> = {}
  if (body) headers['Content-Type'] = 'application/json'
  if (token) headers['Authorization'] = `Bearer ${token}`

  const res = await fetch(`${BASE}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }
  return res.json()
}
```

## Auth Behavior Summary

| State | Header | Tabs visible | Blueprints |
|---|---|---|---|
| Signed out | Shows `<SignInButton>` | All except Blueprints | Hidden |
| Signed in | Shows `<UserButton>` | All tabs | Visible |

## Future Work

- Go backend verifies Clerk JWT (`Authorization: Bearer`) on protected routes
- Cloudflare D1/KV for blueprint cloud save/delete, scoped per Clerk user ID
- Blueprints tab gains save-to-cloud and delete-from-cloud actions
