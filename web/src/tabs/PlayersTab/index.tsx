import { useState, useEffect, useMemo, useCallback } from 'react'
import { SearchField, Spinner, toast } from '@heroui/react'
import { api } from '../../api/client'
import type { Player } from '../../api/client'
import { PageHeader } from '../../dune-ui'
import { PlayerCard } from './components/PlayerCard'
import { PlayerDetailPanel } from './components/PlayerDetailPanel'
import { StatusDot } from './components/StatusDot'
import { InventoryModal } from './modals/InventoryModal'
import { GiveItemsModal } from './modals/GiveItemsModal'
import { PlayerActionsModal } from './modals/PlayerActionsModal'

const MAX_VISIBLE = 5

export default function PlayersTab() {
  const [players, setPlayers] = useState<Player[]>([])
  const [loading, setLoading] = useState(false)
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<Player | null>(null)

  const [showInventory, setShowInventory] = useState(false)
  const [showGive, setShowGive] = useState(false)
  const [showActions, setShowActions] = useState(false)
  const [modalPlayer, setModalPlayer] = useState<Player | null>(null)

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

  const filtered = useMemo(() => {
    const q = search.toLowerCase()
    const list = q
      ? players.filter((p) =>
          p.name.toLowerCase().includes(q)
          || p.class.toLowerCase().includes(q)
          || p.map.toLowerCase().includes(q),
        )
      : players
    return list.slice(0, MAX_VISIBLE)
  }, [players, search])

  const onlineCount = useMemo(
    () => players.filter((p) => p.online_status === 'Online').length,
    [players],
  )

  const openModal = (player: Player, action: 'inventory' | 'give' | 'actions') => {
    setModalPlayer(player)
    if (action === 'inventory') setShowInventory(true)
    else if (action === 'give') setShowGive(true)
    else setShowActions(true)
  }

  return (
    <div className="flex flex-col gap-4 h-full min-h-0 px-1">
      <PageHeader title="Players" onRefresh={loadPlayers} loading={loading}>
        <div className="flex items-center gap-2 text-sm text-muted">
          <StatusDot status={onlineCount > 0 ? 'Online' : 'Offline'} />
          {onlineCount}
          {' online'}
        </div>
      </PageHeader>

      <div className="flex flex-col gap-2 shrink-0 min-h-0">
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

        {loading && players.length === 0
          ? <div className="flex justify-center py-4"><Spinner size="sm" /></div>
          : filtered.length === 0
            ? <p className="text-muted text-sm text-center py-2">No players found</p>
            : (
                <div className={filtered.length > 3 ? 'flex flex-col gap-2 overflow-y-auto max-h-48' : 'flex flex-col gap-2'}>
                  {filtered.map((p) => (
                    <PlayerCard
                      key={p.id}
                      player={p}
                      selected={selected?.id === p.id}
                      onSelect={setSelected}
                      onAction={openModal}
                    />
                  ))}
                </div>
              )}

        {players.length > MAX_VISIBLE && !search && (
          <p className="text-muted text-xs text-center">
            {'Showing '}
            {MAX_VISIBLE}
            {' of '}
            {players.length}
            {' — use search to filter'}
          </p>
        )}
      </div>

      {selected
        ? (
            <div className="flex flex-col gap-1 overflow-y-auto flex-1 min-h-0">
              <div className="flex items-center gap-2 mb-1">
                <span className="font-semibold text-accent">{selected.name}</span>
                <StatusDot status={selected.online_status} />
                <span className="text-muted text-xs">{selected.online_status}</span>
              </div>
              <PlayerDetailPanel player={selected} />
            </div>
          )
        : (
            <p className="text-muted text-sm text-center py-8">Select a player above</p>
          )}

      {modalPlayer && (
        <>
          <InventoryModal player={modalPlayer} open={showInventory} onClose={() => setShowInventory(false)} />
          <GiveItemsModal player={modalPlayer} open={showGive} onClose={() => setShowGive(false)} />
          <PlayerActionsModal player={modalPlayer} open={showActions} onClose={() => setShowActions(false)} />
        </>
      )}
    </div>
  )
}
