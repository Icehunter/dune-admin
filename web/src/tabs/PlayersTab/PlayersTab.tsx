import * as React from 'react'
import { Button, SearchField, Spinner, toast } from '@heroui/react'
import { Segment } from '@heroui-pro/react'
import { useTranslation } from 'react-i18next'
import { api } from '../../api/client'
import type { Player } from '../../api/client'
import { Icon, SideNav } from '../../dune-ui'
import { useAutoRefresh } from '../../hooks/useAutoRefresh'
import { usePermissions } from '../../hooks/usePermissions'
import { DiscordBadge } from './components/DiscordBadge'
import { PlayerDetailPanel } from './components/PlayerDetailPanel'
import { PlayerListControls } from './components/PlayerListControls'
import { ServerDashboard } from './components/ServerDashboard'
import { StatusDot } from './components/StatusDot'
import { InventoryView } from './views/InventoryView'
import { VehiclesView } from './views/VehiclesView'
import { GiveItemsView } from './views/GiveItemsView'
import { ActionsView } from './views/ActionsView'
import { comparePlayers, factionLabel } from './playerListHelpers'
import type { DetailTab, PlayerSortKey, PlayerStatusFilter, SortDir } from './types'

const POLL_MS = 30_000

export const PlayersTab: React.FC = () => {
  const { t } = useTranslation()
  const { can } = usePermissions()
  const canPlayersWrite = can('players:write')
  const canExportData = can('data:export')

  const ALL_DETAIL_TABS: { key: DetailTab, label: string }[] = [
    { key: 'overview', label: t('players.tabs.overview') },
    { key: 'inventory', label: t('players.tabs.inventory') },
    { key: 'vehicles', label: t('players.tabs.vehicles') },
    { key: 'give', label: t('players.tabs.give') },
    { key: 'actions', label: t('players.tabs.actions') },
  ]
  const DETAIL_TABS = ALL_DETAIL_TABS.filter((tab) => {
    if (tab.key === 'give') return canPlayersWrite
    if (tab.key === 'actions') return canPlayersWrite || canExportData
    return true
  })

  const [players, setPlayers] = React.useState<Player[]>([])
  const [loading, setLoading] = React.useState(false)
  const [search, setSearch] = React.useState('')
  const [statusFilter, setStatusFilter] = React.useState<PlayerStatusFilter>('all')
  const [factionFilter, setFactionFilter] = React.useState<Set<number>>(new Set())
  const [sortKey, setSortKey] = React.useState<PlayerSortKey>('name')
  const [sortDir, setSortDir] = React.useState<SortDir>('asc')
  const [selected, setSelected] = React.useState<Player | null>(null)
  const [activeTab, setActiveTab] = React.useState<DetailTab>('overview')

  const loadPlayers = (): void => {
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
  }

  React.useEffect(() => {
    loadPlayers()
  }, [])

  const { countdown, refresh } = useAutoRefresh(loadPlayers, POLL_MS)

  const _pq = search.toLowerCase()
  const _searched = _pq
    ? players.filter((p) =>
        p.name.toLowerCase().includes(_pq)
        || p.class.toLowerCase().includes(_pq)
        || p.map.toLowerCase().includes(_pq),
      )
    : players
  const _byStatus = statusFilter === 'online'
    ? _searched.filter((p) => p.online_status === 'Online')
    : statusFilter === 'offline'
      ? _searched.filter((p) => p.online_status !== 'Online')
      : _searched
  const _byFaction = factionFilter.size === 0
    ? _byStatus
    : _byStatus.filter((p) => factionFilter.has(p.faction_id))

  // SORT is a single axis + direction, orthogonal to the FILTER facets above
  // (#281) — no more hard online-first grouping.
  const unalignedLabel = t('players.detail.unaligned')
  const filtered = [..._byFaction].sort(
    (a, b) => comparePlayers(a, b, sortKey, sortDir, unalignedLabel),
  )

  const factionOptions = Array.from(new Set(players.map((p) => p.faction_id)))
    .sort((a, b) => a - b)
    .map((id) => ({ id, label: factionLabel(id, unalignedLabel) }))

  const navItems = filtered.map((p) => {
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
  })

  return (
    <div className="flex h-full min-h-0 gap-3">
      <SideNav
        items={navItems}
        active={selected ? String(selected.id) : null}
        onSelect={(id) => {
          const p = players.find((x) => String(x.id) === id)
          if (p) setSelected(p)
        }}
        title={`${t('players.title')} (${players.length})`}
        titleAction={(
          <Button size="sm" variant="ghost" onPress={refresh} isDisabled={loading}>
            {loading
              ? <Spinner size="sm" color="current" />
              : (
                  <React.Fragment>
                    <span className="w-7 text-right tabular-nums text-muted/60 text-xs">
                      {countdown}
                      s
                    </span>
                    <Icon name="refresh-cw" />
                  </React.Fragment>
                )}
          </Button>
        )}
        width="w-80"
        listHeader={(
          <PlayerListControls
            sortKey={sortKey}
            onSortKeyChange={setSortKey}
            sortDir={sortDir}
            onToggleSortDir={() => setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))}
            statusFilter={statusFilter}
            onStatusFilterChange={setStatusFilter}
            factionFilter={factionFilter}
            onFactionFilterChange={setFactionFilter}
            factionOptions={factionOptions}
          />
        )}
        emptyContent={t('players.filter.empty')}
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
        <button
          type="button"
          onClick={() => setSelected(null)}
          className={[
            'w-full flex items-center gap-3 px-3 py-2 h-14 rounded-[var(--radius)] text-sm text-left cursor-pointer transition-colors',
            !selected
              ? 'text-[var(--color-focus)] font-semibold'
              : 'text-foreground hover:bg-[color-mix(in_srgb,var(--accent)_12%,transparent)]',
          ].join(' ')}
          style={!selected
            ? {
                background: 'linear-gradient(90deg, color-mix(in srgb, var(--accent) 32%, transparent), color-mix(in srgb, var(--accent) 14%, transparent))',
                boxShadow: 'inset 0 0 18px color-mix(in srgb, var(--accent) 15%, transparent)',
                border: '1px solid color-mix(in srgb, var(--accent) 55%, transparent)',
              }
            : undefined}
        >
          <Icon name="layout-dashboard" className="size-4 shrink-0" />
          <span className="truncate">{t('players.dashboard.navLabel')}</span>
        </button>
      </SideNav>

      {/* Right detail panel */}
      <div className="flex-1 min-w-0 flex flex-col min-h-0">
        {selected
          ? (
              <React.Fragment>
                {/* Fixed header: name + account id + status + tab nav */}
                <div className="shrink-0 flex items-center gap-2 pr-3 py-2">
                  <span className="font-semibold text-accent">{selected.name}</span>
                  <span className="text-muted text-xs font-mono">{`#${selected.account_id}`}</span>
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
                  {activeTab === 'give' && canPlayersWrite && (
                    <div className="h-full flex flex-col pt-4 pb-4">
                      <GiveItemsView player={selected} />
                    </div>
                  )}
                  {activeTab === 'actions' && (canPlayersWrite || canExportData) && (
                    <div className="h-full flex flex-col pr-3">
                      <ActionsView player={selected} />
                    </div>
                  )}
                </div>
              </React.Fragment>
            )
          : <ServerDashboard />}
      </div>
    </div>
  )
}
