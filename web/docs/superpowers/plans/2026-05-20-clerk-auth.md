# Clerk Auth Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Clerk authentication to Dune Admin — gating the Blueprints tab behind sign-in and forwarding the Clerk JWT on all API requests.

**Architecture:** Install `@clerk/react`, wrap the app root in `<ClerkProvider>`, use `<Show>` to conditionally render the Blueprints tab/panel, and update `req()` in `api/client.ts` to attach the Clerk session token as an `Authorization: Bearer` header. No React hooks needed inside the client module — `window.Clerk` is used directly since `req()` is a plain async function.

**Tech Stack:** React 19, Vite, TypeScript, `@clerk/react@latest`, HeroUI v3, Tailwind CSS v4

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `.env.local` | Create | Hold `VITE_CLERK_PUBLISHABLE_KEY` (not committed — covered by `*.local` in `.gitignore`) |
| `src/main.tsx` | Modify | Wrap `<App />` in `<ClerkProvider>` |
| `src/api/client.ts` | Modify | Add `window.Clerk` type declaration; attach JWT in `req()` |
| `src/App.tsx` | Modify | Add `<UserButton>` / `<SignInButton>` to header; gate Blueprints tab + panel |

---

## Task 1: Install @clerk/react

**Files:**

- Modify: `package.json`, `package-lock.json` (via npm)

- [ ] **Step 1: Install the SDK**

Run from `web/`:

```bash
npm install @clerk/react@latest
```

- [ ] **Step 2: Verify the dependency was added**

Run:

```bash
grep '"@clerk/react"' package.json
```

Expected output (version may differ):

```
"@clerk/react": "^5.x.x",
```

---

## Task 2: Create .env.local

**Files:**

- Create: `web/.env.local`

- [ ] **Step 1: Get your Publishable Key**

Open the Clerk Dashboard → API Keys → choose **React** → copy the Publishable Key (starts with `pk_test_` or `pk_live_`).

- [ ] **Step 2: Create the file**

Create `web/.env.local` with this content (replace the value):

```
VITE_CLERK_PUBLISHABLE_KEY=pk_test_YOUR_KEY_HERE
```

- [ ] **Step 3: Confirm .gitignore covers it**

Run:

```bash
git check-ignore -v .env.local
```

Expected output (the `*.local` rule should match):

```
../.gitignore:X:*.local .env.local
```

---

## Task 3: Wrap app in ClerkProvider

**Files:**

- Modify: `src/main.tsx`

- [ ] **Step 1: Replace the contents of `src/main.tsx`**

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { ClerkProvider } from '@clerk/react'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ClerkProvider afterSignOutUrl="/">
      <App />
    </ClerkProvider>
  </StrictMode>,
)
```

- [ ] **Step 2: Verify dev server starts without errors**

Run:

```bash
npm run dev
```

Expected: Vite starts, browser opens, app renders. No console errors about Clerk missing or misconfigured. If you see `Missing publishable key`, confirm `.env.local` has the correct `VITE_CLERK_PUBLISHABLE_KEY` value.

---

## Task 4: Forward JWT in api/client.ts

**Files:**

- Modify: `src/api/client.ts` (lines 1–19)

- [ ] **Step 1: Add the `window.Clerk` type declaration at the top of the file**

Insert this block immediately before the `function getApiBase()` line (the very top of the file, before any existing code):

```ts
declare global {
  interface Window {
    Clerk?: { session?: { getToken(): Promise<string | null> } }
  }
}
```

- [ ] **Step 2: Replace the `req` function**

Replace the existing `req` function (lines 8–19) with:

```ts
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

- [ ] **Step 3: Verify TypeScript compiles**

Run:

```bash
npm run build 2>&1 | head -20
```

Expected: Build succeeds with no type errors. If you see `Property 'Clerk' does not exist on type 'Window'`, confirm the `declare global` block was added before `function getApiBase()`.

---

## Task 5: Add auth controls to the header in App.tsx

**Files:**

- Modify: `src/App.tsx`

- [ ] **Step 1: Add Clerk imports**

Add `Show`, `SignInButton`, and `UserButton` to the imports at the top of `App.tsx`. The existing first line is:

```tsx
import { useState } from 'react'
```

Add after it:

```tsx
import { Show, SignInButton, UserButton } from '@clerk/react'
```

- [ ] **Step 2: Add auth controls after the gear button in the header**

Find the gear `<button>` block in the header's right-side flex container. It ends with:

```tsx
          >
            ⚙
          </button>
```

Immediately after that closing `</button>` tag (still inside the `<div className="flex items-center gap-4 text-xs">` container), add:

```tsx
          <Show when="signed-out">
            <SignInButton />
          </Show>
          <Show when="signed-in">
            <UserButton />
          </Show>
```

- [ ] **Step 3: Verify the header renders in the browser**

With `npm run dev` running, open the app. The header should show a "Sign in" button on the right when not signed in, or your avatar when signed in. No layout shifts or console errors.

---

## Task 6: Gate the Blueprints tab and panel

**Files:**

- Modify: `src/App.tsx`

- [ ] **Step 1: Wrap the Blueprints `<Tabs.Tab>` in `<Show when="signed-in">`**

Find:

```tsx
              <Tabs.Tab id="blueprints">Blueprints<Tabs.Indicator /></Tabs.Tab>
```

Replace with:

```tsx
              <Show when="signed-in">
                <Tabs.Tab id="blueprints">Blueprints<Tabs.Indicator /></Tabs.Tab>
              </Show>
```

- [ ] **Step 2: Wrap the Blueprints `<Tabs.Panel>` in `<Show when="signed-in">`**

Find:

```tsx
          <Tabs.Panel id="blueprints" className="flex-1 overflow-hidden flex flex-col p-4">
            <BlueprintsTab />
          </Tabs.Panel>
```

Replace with:

```tsx
          <Show when="signed-in">
            <Tabs.Panel id="blueprints" className="flex-1 overflow-hidden flex flex-col p-4">
              <BlueprintsTab />
            </Tabs.Panel>
          </Show>
```

- [ ] **Step 3: Verify TypeScript compiles**

Run:

```bash
npm run build 2>&1 | head -20
```

Expected: No errors.

---

## Task 7: Manual End-to-End Verification

- [ ] **Step 1: Start the dev server**

```bash
npm run dev
```

- [ ] **Step 2: Verify signed-out state**

Open the app in your browser while not signed in. Confirm:

- Header shows a "Sign in" button on the right
- Tab list shows: Battlegroup, Players, Database, Logs, Storage — but **not** Blueprints

- [ ] **Step 3: Sign in**

Click the Sign In button, complete the Clerk sign-in flow. Confirm:

- Header shows your user avatar (`<UserButton>`)
- Blueprints tab appears in the tab list
- Clicking Blueprints tab renders the Blueprints content

- [ ] **Step 4: Verify JWT on API requests**

While signed in, open browser DevTools → Network tab. Trigger any API call (e.g., switch to the Battlegroup tab). Find a request to `/api/v1/...` and inspect its headers. Confirm:

```
Authorization: Bearer eyJ...
```

- [ ] **Step 5: Sign out and verify Blueprints disappears**

Click your avatar → Sign Out. Confirm:

- Blueprints tab is gone from the nav
- Sign In button is back in the header
- API requests to `/api/v1/...` no longer carry an `Authorization` header
