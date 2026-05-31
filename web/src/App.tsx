import { memo, useState, useEffect, useRef, type ReactNode } from 'react'
import { Show, SignInButton, UserButton, useAuth } from '@clerk/react'
import { Button, Chip, Modal, Spinner, Toast, toast } from '@heroui/react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useStatus } from './hooks/useStatus'
import SettingsConfigForm from './components/SettingsConfigForm'
import { LanguageSelector } from './components/LanguageSelector'
import BattlegroupTab from './tabs/BattlegroupTab'
import LiveMapTab from './tabs/LiveMapTab'
import PlayersTab from './tabs/PlayersTab'
import DatabaseTab from './tabs/DatabaseTab'
import LogsTab from './tabs/LogsTab'
import BlueprintsTab from './tabs/BlueprintsTab'
import BasesTab from './tabs/BasesTab'
import StorageTab from './tabs/StorageTab'
import ServerSettingsTab from './tabs/ServerSettingsTab'
import MarketTab from './tabs/MarketTab'
import WelcomePackageTab from './tabs/WelcomePackageTab'
import { Icon, SideNav } from './dune-ui'
import { api } from './api/client'
import type { UpdateCheckResult } from './api/client'

const TAB_IDS = [
  'battlegroup',
  'players',
  'database',
  'logs',
  'blueprints',
  'bases',
  'storage',
  'livemap',
  'server',
  'market',
  'welcome',
] as const
type TabId = (typeof TAB_IDS)[number]
const DEFAULT_TAB: TabId = 'battlegroup'

function currentTabFromPath(pathname: string): TabId {
  const seg = pathname.replace(/^\//, '').split('/')[0]
  return (TAB_IDS as readonly string[]).includes(seg) ? (seg as TabId) : DEFAULT_TAB
}

type DbSection = 'tables' | 'describe' | 'sample' | 'search' | 'sql'

// Sub-items shown in the Operations nav when the Database tab is active.
const DB_SECTIONS: { key: string, label: string, depth: number }[] = [
  { key: 'db:tables', label: 'Tables', depth: 1 },
  { key: 'db:describe', label: 'Describe', depth: 1 },
  { key: 'db:sample', label: 'Sample', depth: 1 },
  { key: 'db:search', label: 'Search Columns', depth: 1 },
  { key: 'db:sql', label: 'Run SQL', depth: 1 },
]

// Memoized at module level so identity is stable — prevents all inactive tabs from
// re-rendering whenever AppCore re-renders (e.g. router location change, useStatus poll).
const MBattlegroupTab = memo(BattlegroupTab)
const MLiveMapTab = memo(LiveMapTab)
const MPlayersTab = memo(PlayersTab)
const MDatabaseTab = memo(DatabaseTab)
const MLogsTab = memo(LogsTab)
const MBlueprintsTab = memo(BlueprintsTab)
const MBasesTab = memo(BasesTab)
const MStorageTab = memo(StorageTab)
const MServerSettingsTab = memo(ServerSettingsTab)
const MMarketTab = memo(MarketTab)
const MWelcomePackageTab = memo(WelcomePackageTab)

const hasClerk = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY

function AppWithAuth() {
  const { isSignedIn } = useAuth()
  return <AppCore isSignedIn={!!isSignedIn} />
}

export default function App() {
  return hasClerk ? <AppWithAuth /> : <AppCore isSignedIn={true} />
}

function parseVer(v: string): [number, number, number] {
  // Strip leading "v" and any pre-release suffix (-dev, -rc1, etc.) before parsing.
  const [a, b, c] = v.replace(/^v/, '').replace(/-.*$/, '').split('.').map(Number)
  return [a || 0, b || 0, c || 0]
}

function isNewer(latest: string, current: string): boolean {
  const [la, lb, lc] = parseVer(latest)
  const [ca, cb, cc] = parseVer(current)
  if (la !== ca) return la > ca
  if (lb !== cb) return lb > cb
  return lc > cc
}

function AppCore({ isSignedIn }: { isSignedIn: boolean }) {
  const status = useStatus()
  const location = useLocation()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const [reconnecting, setReconnecting] = useState(false)

  // Re-establish backend connections (DB + control plane) without a service
  // restart — used by the header Reconnect button when the DB shows disconnected
  // (e.g. dune-admin came up before the database was ready).
  const handleReconnect = async () => {
    setReconnecting(true)
    try {
      const s = await api.reconnect()
      if (s.db_connected) toast.success(t('app.reconnected'))
      else toast.danger(t('app.reconnectFailed', { error: 'database still unreachable' }))
    }
    catch (e) {
      toast.danger(t('app.reconnectFailed', { error: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setReconnecting(false)
    }
  }

  // Left-sidebar navigation, grouped to mirror the product's structure
  // (operator tooling today; a Player Portal group lands here later).
  const NAV_GROUPS: { title: string, items: { key: TabId, label: string }[] }[] = [
    {
      title: t('nav.groups.operations'),
      items: [
        { key: 'battlegroup' as TabId, label: t('nav.battlegroup') },
        { key: 'logs' as TabId, label: t('nav.logs') },
        { key: 'database' as TabId, label: t('nav.database') },
        { key: 'server' as TabId, label: t('nav.server') },
      ],
    },
    {
      title: t('nav.groups.playerWorld'),
      items: [
        { key: 'players' as TabId, label: t('nav.players') },
        { key: 'livemap' as TabId, label: t('nav.liveMap') },
        { key: 'storage' as TabId, label: t('nav.storage') },
        { key: 'bases' as TabId, label: t('nav.bases') },
        { key: 'blueprints' as TabId, label: t('nav.blueprints') },
      ],
    },
    {
      title: t('nav.groups.economy'),
      items: [
        { key: 'market' as TabId, label: t('nav.market') },
        { key: 'welcome' as TabId, label: t('nav.welcome') },
      ],
    },
  ]

  const [dbSection, setDbSection] = useState<DbSection>('tables')
  const [showBackendConfig, setShowBackendConfig] = useState(false)
  const [latestVersion, setLatestVersion] = useState<string | null>(null)
  const [updateInfo, setUpdateInfo] = useState<UpdateCheckResult | null>(null)
  const [updateChecking, setUpdateChecking] = useState(false)
  const [updateApplying, setUpdateApplying] = useState(false)
  const [formSaving, setFormSaving] = useState(false)
  const formSaveRef = useRef<(() => Promise<void>) | null>(null)

  useEffect(() => {
    const seg = location.pathname.replace(/^\//, '').split('/')[0]
    if (!seg || !(TAB_IDS as readonly string[]).includes(seg)) {
      navigate(`/${DEFAULT_TAB}`, { replace: true })
    }
  }, [location.pathname, navigate])

  const currentTab = currentTabFromPath(location.pathname)

  useEffect(() => {
    fetch('https://api.github.com/repos/Icehunter/dune-admin/releases/latest')
      .then((r) => r.json())
      .then((d) => setLatestVersion(d.tag_name || null))
      .catch(() => {})
  }, [])

  const checkUpdate = async () => {
    setUpdateChecking(true)
    try {
      setUpdateInfo(await api.update.check())
    }
    catch {
      // silently ignore — user can retry
    }
    finally {
      setUpdateChecking(false)
    }
  }

  const applyUpdate = async (force = false) => {
    setUpdateApplying(true)
    try {
      const result = await api.update.apply(force)
      if (result.updated) {
        toast.success(force ? t('app.reinstalled', { version: result.version ?? 'latest' }) : t('app.updated', { version: result.version ?? 'latest' }))
        setUpdateInfo(null)
      }
      else {
        toast.info(result.message)
      }
    }
    catch (e) {
      toast.danger(t('app.updateFailed', { message: e instanceof Error ? e.message : String(e) }))
    }
    finally {
      setUpdateApplying(false)
    }
  }

  return (
    <div className="h-screen flex flex-col overflow-hidden bg-background">
      <Toast.Provider />

      {/* Header */}
      <header
        className="flex items-center justify-between px-6 py-3 border-b border-[#4e3411] bg-surface shrink-0"
        style={{ background: 'linear-gradient(180deg, #241a0e 0%, #1a1610 100%)' }}
      >
        <div className="flex items-center gap-3">
          <span className="text-xl font-bold uppercase tracking-[0.2em] text-accent">{t('app.title')}</span>
          {status?.control && status.control !== 'none' && <span className="text-xs text-muted">{status.control}</span>}
          {status?.ssh_host && <span className="text-xs text-muted">{status.ssh_host}</span>}
          {status?.db_host && status.control !== 'kubectl' && (
            <span className="text-xs text-muted">{status.db_host}</span>
          )}
          {status?.version && (
            <button
              className="text-xs text-muted hover:text-foreground cursor-pointer bg-transparent border-0 p-0"
              onClick={() => setShowBackendConfig(true)}
              title={t('app.openSettings')}
            >
              v
              {status.version}
            </button>
          )}
          {latestVersion && status?.version && isNewer(latestVersion, status.version) && (
            <a
              href="https://github.com/Icehunter/dune-admin/releases/latest"
              target="_blank"
              rel="noreferrer"
              className="no-underline"
            >
              <Chip size="sm" color="warning" variant="soft">
                ↑
                {' '}
                {latestVersion}
              </Chip>
            </a>
          )}
        </div>

        <div className="flex items-center gap-3">
          {status?.executor === 'ssh' && <ConnectionBadge label="SSH" connected={status.ssh_connected} />}
          <ConnectionBadge label="DB" connected={status?.db_connected ?? false} />
          {status && !status.db_connected && (
            <Button
              size="sm"
              variant="outline"
              isDisabled={reconnecting}
              onPress={handleReconnect}
            >
              {reconnecting ? t('app.reconnecting') : t('app.reconnect')}
            </Button>
          )}
          {status?.pod_ns && (
            <span className="text-xs text-muted">
              ns:
              {status.pod_ns}
            </span>
          )}

          <LanguageSelector />
          <Button
            size="sm"
            variant="ghost"
            isIconOnly
            aria-label={t('app.configureBackend')}
            onPress={() => setShowBackendConfig((v) => !v)}
            className={showBackendConfig ? 'text-accent' : ''}
          >
            <Icon name="settings" />
          </Button>

          {hasClerk && (
            <>
              <Show when="signed-out">
                <SignInButton>
                  <Button size="sm" variant="outline">
                    {t('app.signIn')}
                  </Button>
                </SignInButton>
              </Show>
              <Show when="signed-in">
                <UserButton />
              </Show>
            </>
          )}
        </div>
      </header>

      {/* Settings modal — structure mirrors BotControlPanel */}
      <Modal>
        <Modal.Backdrop isOpen={showBackendConfig} onOpenChange={(v) => !v && setShowBackendConfig(false)}>
          <Modal.Container size="cover" scroll="outside">
            <Modal.Dialog className="h-[92vh] flex flex-col">
              <Modal.CloseTrigger />
              <Modal.Header>
                <div className="flex items-baseline gap-6 flex-wrap">
                  <Modal.Heading className="text-accent">{t('app.settings')}</Modal.Heading>
                  {status && (
                    <div className="flex items-center gap-4 text-xs text-muted">
                      {status.version && (
                        <span className="font-mono">
                          v
                          {status.version}
                        </span>
                      )}
                      {status.control && status.control !== 'none' && <span>{status.control}</span>}
                      {status.commit && status.commit !== 'unknown' && (
                        <span className="font-mono opacity-60">{status.commit}</span>
                      )}
                    </div>
                  )}
                </div>
              </Modal.Header>

              {/* Body scrolls; form fills it with its own internal tab scroll */}
              <Modal.Body className="flex flex-col overflow-y-auto flex-1 min-h-0 pr-1">
                {showBackendConfig && (
                  <SettingsConfigForm saveRef={formSaveRef} onSavingChange={setFormSaving} />
                )}
              </Modal.Body>

              <Modal.Footer className="flex items-center gap-2">
                {/* Left: update controls — fixed positions so buttons don't shift */}
                <Button
                  size="sm"
                  variant="ghost"
                  onPress={checkUpdate}
                  isDisabled={updateChecking || updateApplying}
                >
                  {updateChecking
                    ? (
                        <>
                          <Spinner size="sm" color="current" />
                          {' '}
                          {t('common.checking')}
                        </>
                      )
                    : t('app.checkUpdates')}
                </Button>
                {updateInfo && !updateInfo.needs_update && (
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={() => applyUpdate(true)}
                    isDisabled={updateApplying}
                  >
                    {updateApplying ? <Spinner size="sm" color="current" /> : t('app.reinstall')}
                  </Button>
                )}
                {updateInfo?.needs_update && (
                  <Button size="sm" onPress={() => applyUpdate()} isDisabled={updateApplying}>
                    {updateApplying
                      ? <Spinner size="sm" color="current" />
                      : (
                          <span className="font-mono text-xs">
                            v
                            {updateInfo.current}
                            {' → '}
                            v
                            {updateInfo.latest.replace(/^v/, '')}
                          </span>
                        )}
                  </Button>
                )}

                {/* Spacer */}
                <span className="flex-1" />

                {/* Right: save + close */}
                <span className="text-xs text-muted">{t('app.changesNote')}</span>
                <Button
                  size="sm"
                  onPress={() => formSaveRef.current?.()}
                  isDisabled={formSaving}
                >
                  {formSaving
                    ? (
                        <>
                          <Spinner size="sm" color="current" />
                          {' '}
                          {t('common.saving')}
                        </>
                      )
                    : (
                        <>
                          <Icon name="save" />
                          {' '}
                          {t('app.saveApply')}
                        </>
                      )}
                </Button>
                <Button
                  size="sm"
                  variant="tertiary"
                  onPress={() => setShowBackendConfig(false)}
                >
                  {t('common.close')}
                </Button>
              </Modal.Footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>

      {/* Body: grouped left sidebar + content. All tabs stay mounted (inactive
          hidden) so per-tab state and isActive auto-refresh behavior persist. */}
      <div className="flex-1 flex gap-3 p-3 overflow-hidden min-h-0">
        <nav className="w-60 shrink-0 flex flex-col gap-2 overflow-y-auto">
          {/* Operations: rendered separately so Database can expand DB sub-items inline */}
          <SideNav
            width="w-full"
            title={NAV_GROUPS[0].title}
            items={[
              ...NAV_GROUPS[0].items.slice(0, 3),
              ...(currentTab === 'database' ? DB_SECTIONS : []),
              ...NAV_GROUPS[0].items.slice(3),
            ] as { key: string, label: string, depth?: number }[]}
            active={currentTab === 'database' ? `db:${dbSection}` : currentTab}
            onSelect={(k: string) => {
              if (k.startsWith('db:')) {
                setDbSection(k.slice(3) as DbSection)
                if (currentTab !== 'database') navigate('/database')
              }
              else {
                navigate(`/${k}`)
              }
            }}
          />
          {/* Player World + Economy */}
          {NAV_GROUPS.slice(1).map((group) => (
            <SideNav
              key={group.title}
              width="w-full"
              title={group.title}
              items={group.items}
              active={currentTab}
              onSelect={(k) => navigate(`/${k}`)}
            />
          ))}
        </nav>
        <main className="flex-1 overflow-hidden min-h-0">
          <TabPane active={currentTab === 'battlegroup'}>
            <MBattlegroupTab isActive={currentTab === 'battlegroup'} />
          </TabPane>
          <TabPane active={currentTab === 'players'}>
            <MPlayersTab isActive={currentTab === 'players'} />
          </TabPane>
          <TabPane active={currentTab === 'database'}>
            <MDatabaseTab section={dbSection} />
          </TabPane>
          <TabPane active={currentTab === 'logs'}>
            <MLogsTab control={status?.control} />
          </TabPane>
          <TabPane active={currentTab === 'blueprints'}>
            <MBlueprintsTab isSignedIn={isSignedIn} />
          </TabPane>
          <TabPane active={currentTab === 'bases'}>
            <MBasesTab isSignedIn={isSignedIn} />
          </TabPane>
          <TabPane active={currentTab === 'storage'}>
            <MStorageTab />
          </TabPane>
          <TabPane active={currentTab === 'livemap'}>
            <MLiveMapTab isActive={currentTab === 'livemap'} />
          </TabPane>
          <TabPane active={currentTab === 'server'}>
            <MServerSettingsTab />
          </TabPane>
          <TabPane active={currentTab === 'market'}>
            <MMarketTab />
          </TabPane>
          <TabPane active={currentTab === 'welcome'}>
            <MWelcomePackageTab />
          </TabPane>
        </main>
      </div>
    </div>
  )
}

// TabPane keeps every tab mounted and toggles visibility, preserving in-tab
// state and the isActive auto-refresh contract when switching via the sidebar.
function TabPane({ active, children }: { active: boolean, children: ReactNode }) {
  return (
    <div className={`h-full min-h-0 ${active ? 'flex flex-col' : 'hidden'}`}>
      {children}
    </div>
  )
}

function ConnectionBadge({ label, connected }: { label: string, connected: boolean }) {
  return (
    <div className="flex items-center gap-1.5 text-xs">
      <div className={`w-2 h-2 rounded-full ${connected ? 'bg-success' : 'bg-muted/40'}`} />
      <span className={connected ? 'text-foreground' : 'text-muted'}>{label}</span>
    </div>
  )
}
