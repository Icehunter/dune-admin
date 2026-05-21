import { useState } from 'react'
import { Show, SignInButton, UserButton, useAuth } from '@clerk/react'
import { Toast } from '@heroui/react'
import { Tabs } from '@heroui/react'
import { useStatus } from './hooks/useStatus'
import { getAdminToken, setAdminToken } from './api/client'
import BattlegroupTab from './tabs/BattlegroupTab'
import PlayersTab from './tabs/PlayersTab'
import DatabaseTab from './tabs/DatabaseTab'
import LogsTab from './tabs/LogsTab'
import BlueprintsTab from './tabs/BlueprintsTab'
import StorageTab from './tabs/StorageTab'

const hasClerk = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY

function AppWithAuth() {
  const { isSignedIn } = useAuth()
  return <AppCore isSignedIn={!!isSignedIn} />
}

export default function App() {
  return hasClerk ? <AppWithAuth /> : <AppCore isSignedIn={true} />
}

function AppCore({ isSignedIn }: { isSignedIn: boolean }) {
  const status = useStatus()
  const [showBackendConfig, setShowBackendConfig] = useState(false)
  const [backendUrl, setBackendUrl] = useState(() => localStorage.getItem('dune_admin_backend') || '')
  const [tokenInput, setTokenInput] = useState(() => getAdminToken())

  const saveBackendSettings = () => {
    const trimmedUrl = backendUrl.trim()
    if (trimmedUrl) localStorage.setItem('dune_admin_backend', trimmedUrl)
    else localStorage.removeItem('dune_admin_backend')
    setAdminToken(tokenInput)
    window.location.reload()
  }

  const resetBackendSettings = () => {
    localStorage.removeItem('dune_admin_backend')
    setAdminToken('')
    window.location.reload()
  }

  return (
    <div className="h-screen flex flex-col overflow-hidden" style={{ background: 'var(--color-background)' }}>
      <Toast.Provider />

      <div
        className="flex items-center justify-between px-6 py-3 border-b shrink-0"
        style={{ borderColor: '#2a2418', background: 'var(--color-surface)' }}
      >
        <div className="flex items-center gap-3">
          <span
            className="text-xl font-bold tracking-widest uppercase"
            style={{ color: 'var(--color-primary)', letterSpacing: '0.2em' }}
          >
            DUNE ADMIN
          </span>
          {status?.ssh_host && (
            <span className="text-xs" style={{ color: 'var(--color-text-dim)' }}>
              {status.ssh_host}
            </span>
          )}
        </div>
        <div className="flex items-center gap-4 text-xs">
          <ConnectionBadge label="SSH" connected={status?.ssh_connected ?? false} />
          <ConnectionBadge label="DB" connected={status?.db_connected ?? false} />
          {status?.pod_ns && (
            <span style={{ color: 'var(--color-text-dim)' }}>ns: {status.pod_ns}</span>
          )}
          <button
            onClick={() => setShowBackendConfig(v => !v)}
            title="Configure backend URL and admin token"
            style={{
              background: 'transparent',
              border: '1px solid #2a2418',
              borderRadius: '4px',
              color: showBackendConfig ? 'var(--color-primary)' : 'var(--color-text-dim)',
              cursor: 'pointer',
              fontSize: '14px',
              padding: '2px 6px',
              lineHeight: 1,
            }}
          >
            ⚙
          </button>
          {hasClerk && (
            <>
              <Show when="signed-out"><SignInButton /></Show>
              <Show when="signed-in"><UserButton /></Show>
            </>
          )}
        </div>
      </div>

      {showBackendConfig && (
        <div
          style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, zIndex: 100, background: 'rgba(0,0,0,0.6)', display: 'flex', alignItems: 'flex-start', justifyContent: 'flex-end', paddingTop: '52px', paddingRight: '16px' }}
          onClick={e => { if (e.target === e.currentTarget) setShowBackendConfig(false) }}
        >
          <div style={{ background: '#0d0b07', border: '1px solid #2a2418', borderRadius: '8px', padding: '16px', width: '380px', display: 'flex', flexDirection: 'column', gap: '12px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <span style={{ color: 'var(--color-primary)', fontWeight: 600, fontSize: '13px' }}>
                Backend Settings
              </span>
              <button onClick={() => setShowBackendConfig(false)} style={{ background: 'transparent', border: 'none', color: 'var(--color-text-dim)', cursor: 'pointer', fontSize: '16px', lineHeight: 1, padding: '0 2px' }}>
                ✕
              </button>
            </div>

            <div style={{ fontSize: '11px', color: 'var(--color-text-dim)' }}>
              Current URL:{' '}
              <span style={{ color: 'var(--color-text)', fontFamily: 'monospace' }}>
                {localStorage.getItem('dune_admin_backend') || 'http://localhost:8080'}
              </span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
              <label style={{ fontSize: '11px', color: 'var(--color-text-dim)' }}>
                Backend URL
              </label>
              <input
                value={backendUrl}
                onChange={e => setBackendUrl(e.target.value)}
                placeholder="http://host:port"
                style={{ background: 'var(--color-surface)', border: '1px solid #2a2418', borderRadius: '4px', color: 'var(--color-text)', fontFamily: 'monospace', fontSize: '12px', outline: 'none', padding: '6px 10px' }}
                onKeyDown={e => { if (e.key === 'Enter') saveBackendSettings() }}
              />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
              <label style={{ fontSize: '11px', color: 'var(--color-text-dim)' }}>
                Admin Token
              </label>
              <input
                type="password"
                value={tokenInput}
                onChange={e => setTokenInput(e.target.value)}
                placeholder="ADMIN_TOKEN from backend .env"
                style={{ background: 'var(--color-surface)', border: '1px solid #2a2418', borderRadius: '4px', color: 'var(--color-text)', fontFamily: 'monospace', fontSize: '12px', outline: 'none', padding: '6px 10px' }}
                onKeyDown={e => { if (e.key === 'Enter') saveBackendSettings() }}
              />
              <span style={{ fontSize: '11px', color: 'var(--color-text-dim)' }}>
                Stored locally in this browser and sent to the backend as X-Admin-Token.
              </span>
            </div>

            <div style={{ display: 'flex', gap: '8px' }}>
              <button onClick={saveBackendSettings} style={{ background: 'var(--color-primary)', border: 'none', borderRadius: '4px', color: '#fff', cursor: 'pointer', fontSize: '12px', fontWeight: 600, padding: '6px 12px' }}>
                Save &amp; Reload
              </button>
              <button onClick={resetBackendSettings} style={{ background: 'transparent', border: '1px solid #2a2418', borderRadius: '4px', color: 'var(--color-text-dim)', cursor: 'pointer', fontSize: '12px', padding: '6px 12px' }}>
                Reset
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="flex-1 flex flex-col overflow-hidden min-h-0">
        <Tabs className="flex-1 flex flex-col overflow-hidden min-h-0">
          <Tabs.ListContainer className="px-4 pt-2 shrink-0">
            <Tabs.List aria-label="Admin sections" className="gap-1">
              <Tabs.Tab id="battlegroup">Battlegroup<Tabs.Indicator /></Tabs.Tab>
              <Tabs.Tab id="players">Players<Tabs.Indicator /></Tabs.Tab>
              <Tabs.Tab id="database">Database<Tabs.Indicator /></Tabs.Tab>
              <Tabs.Tab id="logs">Logs<Tabs.Indicator /></Tabs.Tab>
              {isSignedIn && <Tabs.Tab id="blueprints">Blueprints<Tabs.Indicator /></Tabs.Tab>}
              <Tabs.Tab id="storage">Storage<Tabs.Indicator /></Tabs.Tab>
            </Tabs.List>
          </Tabs.ListContainer>
          <Tabs.Panel id="battlegroup" className="flex-1 overflow-hidden flex flex-col"><BattlegroupTab /></Tabs.Panel>
          <Tabs.Panel id="players" className="flex-1 overflow-auto p-4"><PlayersTab /></Tabs.Panel>
          <Tabs.Panel id="database" className="flex-1 overflow-auto p-4"><DatabaseTab /></Tabs.Panel>
          <Tabs.Panel id="logs" className="flex-1 overflow-hidden flex flex-col"><LogsTab /></Tabs.Panel>
          {isSignedIn && <Tabs.Panel id="blueprints" className="flex-1 overflow-hidden flex flex-col p-4"><BlueprintsTab /></Tabs.Panel>}
          <Tabs.Panel id="storage" className="flex-1 overflow-hidden flex flex-col p-4"><StorageTab /></Tabs.Panel>
        </Tabs>
      </div>
    </div>
  )
}

function ConnectionBadge({ label, connected }: { label: string; connected: boolean }) {
  return (
    <div className="flex items-center gap-1.5">
      <div className="w-2 h-2 rounded-full" style={{ background: connected ? 'var(--color-success)' : '#555' }} />
      <span style={{ color: connected ? 'var(--color-text)' : 'var(--color-text-dim)' }}>{label}</span>
    </div>
  )
}
