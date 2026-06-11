import * as React from 'react'
import { Button, SearchField, Spinner, toast } from '@heroui/react'
import { Segment } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../api/client'
import type { Player } from '../../api/client'
import { Icon, SideNav } from '../../dune-ui'
import { useAutoRefresh } from '../../hooks/useAutoRefresh'
import { DiscordBadge } from './components/DiscordBadge'
import { PlayerDetailPanel } from './components/PlayerDetailPanel'
import { ServerDashboard } from './components/ServerDashboard'
import { StatusDot } from './components/StatusDot'
import { InventoryView } from './views/InventoryView'
import { VehiclesView } from './views/VehiclesView'
import { GiveItemsView } from './views/GiveItemsView'
import { ActionsView } from './views/ActionsView'
import type { DetailTab, PlayersTabProps } from './types'

const POLL_MS = 30_000
// Sentinel SideNav key for the server-wide dashboard landing (#130).
const OVERVIEW_KEY = '__overview__'

export const PlayersTab: React.FC<PlayersTabProps> = ({ isActive = false }) => {
  const { t } = useTranslation()

  const DETAIL_TABS: { key: DetailTab, label: string }[] = [
    { key: 'overview', label: t('players.tabs.overview') },
    { key: 'inventory', label: t('players.tabs.inventory') },
    { key: 'vehicles', label: t('players.tabs.vehicles') },
    { key: 'give', label: t('players.tabs.give') },
    { key: 'actions', label: t('players.tabs.actions') },
  ]

  const [players, setPlayers] = React.useState<Player[]>([])
  const [loading, setLoading] = React.useState(false)
  const [search, setSearch] = React.useState('')
  const [selected, setSelected] = React.useState<Player | null>(null)
  const [activeTab, setActiveTab] = React.useState<DetailTab>('overview')

  const loadPlayers = React.useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.players.list())
      .then((list) => {
        setPlayers(list)
        // Land on the server dashboard (selected === null) rather than the first
        // player (#130); keep the current selection (refreshed) if one is set.
        setSelected((prev) => (prev ? list.find((p) => p.id === prev.id) ?? prev : null))
      })
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [])

  React.useEffect(() => {
    loadPlayers()
  }, [loadPlayers])

  const { countdown, refresh } = useAutoRefresh(loadPlayers, POLL_MS, isActive)

  const filtered = React.useMemo(() => {
    const q = search.toLowerCase()
    return q
      ? players.filter((p) =>
          p.name.toLowerCase().includes(q)
          || p.class.toLowerCase().includes(q)
          || p.map.toLowerCase().includes(q),
        )
      : players
  }, [players, search])

  const navItems = React.useMemo(() => [
    {
      key: OVERVIEW_KEY,
      icon: <Icon name="layout-dashboard" />,
      label: t('players.dashboard.navLabel'),
    },
    ...filtered.map((p) => {
      const statusDotColor = p.online_status === 'Online'
        ? 'bg-success'
        : p.online_status === 'LoggingOut'
          ? 'bg-warning'
          : 'bg-muted'
      return {
        key: String(p.id),
        icon: (active: boolean) => (
          <div className="relative w-8 h-8 shrink-0">
            <div className="w-full h-full rounded-4xl overflow-hidden bg-surface-secondary flex items-center justify-center [transform:translateZ(0)]">
              {p.discord_avatar
                ? <img src={p.discord_avatar} alt={p.name} className="w-full h-full object-cover" />
                : <Icon name="user" className="size-3.5 text-muted" />}
            </div>
            <span
              className={`absolute bottom-0 right-0 z-[1] size-3 rounded-full border-2 ${statusDotColor}`}
              style={{ borderColor: active ? 'var(--accent)' : 'var(--surface)' }}
            />
          </div>
        ),
        label: p.name,
        sublabel: p.map,
        hint: (active: boolean) => (
          <DiscordBadge discordUserId={p.discord_user_id} color={active ? 'white' : '#5865F2'} />
        ),
      }
    }),
  ], [filtered, t])

  return (
    <div className="flex h-full min-h-0 gap-3">
      <SideNav
        items={navItems}
        active={selected ? String(selected.id) : OVERVIEW_KEY}
        onSelect={(id) => {
          if (id === OVERVIEW_KEY) {
            setSelected(null)
            return
          }
          const p = players.find((x) => String(x.id) === id)
          if (p) setSelected(p)
        }}
        title={`${t('players.title')} (${players.length})`}
        titleAction={(
          <Button size="sm" variant="ghost" onPress={refresh} isDisabled={loading}>
            {loading
              ? <Spinner size="sm" color="current" />
              : (
                  <>
                    {isActive && (
                      <span className="w-7 text-right tabular-nums text-muted/60 text-xs">
                        {countdown}
                        s
                      </span>
                    )}
                    <Icon name="refresh-cw" />
                  </>
                )}
          </Button>
        )}
        width="w-80"
      >
        <SearchField
          aria-label={t('players.searchLabel')}
          className="w-full"
          value={search}
          onChange={setSearch}
        >
          <SearchField.Group>
            <SearchField.SearchIcon />
            <SearchField.Input placeholder={t('players.searchPlaceholder')} />
            <SearchField.ClearButton />
          </SearchField.Group>
        </SearchField>
      </SideNav>

      {/* Right detail panel */}
      <div className="flex-1 min-w-0 flex flex-col min-h-0">
        {selected
          ? (
              <>
                {/* Fixed header: name + status + tab nav */}
                <div className="shrink-0 flex items-center gap-2 pr-3 py-2">
                  <span className="font-semibold text-accent">{selected.name}</span>
                  <StatusDot status={selected.online_status} />
                  <span className="text-muted text-xs">{selected.online_status}</span>
                  <Segment
                    size="sm"
                    className="ml-auto"
                    selectedKey={activeTab}
                    onSelectionChange={(key) => setActiveTab(key as DetailTab)}
                  >
                    {DETAIL_TABS.map((tab) => (
                      <Segment.Item key={tab.key} id={tab.key}>
                        <Segment.Separator />
                        {tab.label}
                      </Segment.Item>
                    ))}
                  </Segment>
                </div>

                {/* Tab content — each tab owns its own scroll/height context */}
                <div className="flex-1 min-h-0 overflow-hidden">
                  {activeTab === 'overview' && (
                    <div className="h-full overflow-y-auto pr-3">
                      <PlayerDetailPanel player={selected} />
                    </div>
                  )}
                  {activeTab === 'inventory' && (
                    <div className="h-full flex flex-col pr-3">
                      <InventoryView player={selected} />
                    </div>
                  )}
                  {activeTab === 'vehicles' && (
                    <div className="h-full flex flex-col pr-3">
                      <VehiclesView player={selected} />
                    </div>
                  )}
                  {activeTab === 'give' && (
                    <div className="h-full flex flex-col pt-4 pb-4">
                      <GiveItemsView player={selected} />
                    </div>
                  )}
                  {activeTab === 'actions' && (
                    <div className="h-full flex flex-col pr-3">
                      <ActionsView player={selected} />
                    </div>
                  )}
                </div>
              </>
            )
          : <ServerDashboard />}
      </div>
    </div>
  )
}
