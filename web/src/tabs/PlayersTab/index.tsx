import { useState, useEffect, useMemo, useCallback } from 'react'
import { Button, SearchField, Spinner, toast } from '@heroui/react'
import { api } from '../../api/client'
import type { Player } from '../../api/client'
import { PageHeader } from '../../dune-ui'
import { useAutoRefresh } from '../../hooks/useAutoRefresh'
import { PlayerCard } from './components/PlayerCard'
import { PlayerDetailPanel } from './components/PlayerDetailPanel'
import { StatusDot } from './components/StatusDot'
import { InventoryView } from './views/InventoryView'
import { VehiclesView } from './views/VehiclesView'
import { GiveItemsView } from './views/GiveItemsView'
import { ActionsView } from './views/ActionsView'

type DetailTab = 'overview' | 'inventory' | 'vehicles' | 'give' | 'actions'

const DETAIL_TABS: { key: DetailTab, label: string }[] = [
  { key: 'overview', label: 'Overview' },
  { key: 'inventory', label: 'Inventory' },
  { key: 'vehicles', label: 'Vehicles' },
  { key: 'give', label: 'Give' },
  { key: 'actions', label: 'Actions' },
]

const POLL_MS = 30_000

export default function PlayersTab({ isActive = false }: { isActive?: boolean }) {
  const [players, setPlayers] = useState<Player[]>([])
  const [loading, setLoading] = useState(false)
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<Player | null>(null)
  const [activeTab, setActiveTab] = useState<DetailTab>('overview')

  const loadPlayers = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.players.list())
      .then((list) => {
        setPlayers(list)
        setSelected((prev) => prev ?? list[0] ?? null)
      })
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    loadPlayers()
  }, [loadPlayers])

  const { countdown, refresh } = useAutoRefresh(loadPlayers, POLL_MS, isActive)

  const filtered = useMemo(() => {
    const q = search.toLowerCase()
    return q
      ? players.filter((p) =>
          p.name.toLowerCase().includes(q)
          || p.class.toLowerCase().includes(q)
          || p.map.toLowerCase().includes(q),
        )
      : players
  }, [players, search])

  const onlineCount = useMemo(
    () => players.filter((p) => p.online_status === 'Online').length,
    [players],
  )

  return (
    <div className="flex flex-col h-full min-h-0">
      <PageHeader title="Players" onRefresh={refresh} loading={loading} countdown={isActive ? countdown : undefined}>
        <div className="flex items-center gap-2 text-sm text-muted">
          <StatusDot status={onlineCount > 0 ? 'Online' : 'Offline'} />
          {onlineCount}
          {' online'}
        </div>
      </PageHeader>

      <div className="flex flex-1 min-h-0">
        {/* Left sidebar */}
        <div className="w-80 shrink-0 flex flex-col border-r border-border">
          <div className="p-2 border-b border-border">
            <SearchField
              aria-label="Search players"
              className="w-full"
              value={search}
              onChange={setSearch}
            >
              <SearchField.Group>
                <SearchField.SearchIcon />
                <SearchField.Input placeholder="Filter players..." />
                <SearchField.ClearButton />
              </SearchField.Group>
            </SearchField>
          </div>

          <div className="flex-1 overflow-y-auto p-2 flex flex-col gap-1">
            {loading && players.length === 0
              ? <div className="flex justify-center py-4"><Spinner size="sm" /></div>
              : filtered.length === 0
                ? <p className="text-muted text-sm text-center py-4">No players found</p>
                : filtered.map((p) => (
                    <PlayerCard
                      key={p.id}
                      player={p}
                      selected={selected?.id === p.id}
                      onSelect={setSelected}
                    />
                  ))}
          </div>
        </div>

        {/* Right detail panel */}
        <div className="flex-1 min-w-0 flex flex-col min-h-0">
          {selected
            ? (
                <>
                  {/* Fixed header: name + status + tab nav */}
                  <div className="shrink-0 flex items-center gap-2 px-4 py-3 border-b border-border">
                    <span className="font-semibold text-accent">{selected.name}</span>
                    <StatusDot status={selected.online_status} />
                    <span className="text-muted text-xs">{selected.online_status}</span>
                    <div className="ml-auto flex gap-1">
                      {DETAIL_TABS.map((tab) => (
                        <Button
                          key={tab.key}
                          size="sm"
                          variant={activeTab === tab.key ? 'secondary' : 'ghost'}
                          onPress={() => setActiveTab(tab.key)}
                        >
                          {tab.label}
                        </Button>
                      ))}
                    </div>
                  </div>

                  {/* Tab content — each tab owns its own scroll/height context */}
                  <div className="flex-1 min-h-0 overflow-hidden">
                    {activeTab === 'overview' && (
                      <div className="h-full overflow-y-auto p-4">
                        <PlayerDetailPanel player={selected} />
                      </div>
                    )}
                    {activeTab === 'inventory' && (
                      <div className="h-full flex flex-col p-4">
                        <InventoryView player={selected} />
                      </div>
                    )}
                    {activeTab === 'vehicles' && (
                      <div className="h-full flex flex-col p-4">
                        <VehiclesView player={selected} />
                      </div>
                    )}
                    {activeTab === 'give' && (
                      <div className="h-full flex flex-col pt-4 pl-4 pb-4">
                        <GiveItemsView player={selected} />
                      </div>
                    )}
                    {activeTab === 'actions' && (
                      <div className="h-full flex flex-col p-4">
                        <ActionsView player={selected} />
                      </div>
                    )}
                  </div>
                </>
              )
            : (
                <div className="flex flex-1 items-center justify-center">
                  <p className="text-muted text-sm">Select a player</p>
                </div>
              )}
        </div>
      </div>
    </div>
  )
}
