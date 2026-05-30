import { useState, useEffect, useMemo, useCallback } from 'react'
import { toast } from '@heroui/react'
import { api } from '../../api/client'
import type {
  Player, CurrencyRow, FactionRep, SpecTrack, OnlineRow,
} from '../../api/client'

import { Sidebar } from './Sidebar'
import type { Sidebar as SidebarKey } from './types'
import { PlayersListView } from './views/PlayersListView'
import { CurrencyView } from './views/CurrencyView'
import { FactionsView } from './views/FactionsView'
import { SpecsView } from './views/SpecsView'
import { OnlineView } from './views/OnlineView'
import { InventoryModal } from './modals/InventoryModal'
import { GiveItemsModal } from './modals/GiveItemsModal'
import { PlayerActionsModal } from './modals/PlayerActionsModal'

export default function PlayersTab() {
  const [active, setActive] = useState<SidebarKey>('players')

  // Main player list
  const [players, setPlayers] = useState<Player[]>([])
  const [loading, setLoading] = useState(false)

  // Side data
  const [currencyData, setCurrencyData] = useState<CurrencyRow[]>([])
  const [factionData, setFactionData] = useState<FactionRep[]>([])
  const [specData, setSpecData] = useState<SpecTrack[]>([])
  const [onlineData, setOnlineData] = useState<OnlineRow[]>([])
  const [sideLoading, setSideLoading] = useState(false)

  // Selection + modals
  const [selectedPlayer, setSelectedPlayer] = useState<Player | null>(null)
  const [showInventory, setShowInventory] = useState(false)
  const [showGiveItems, setShowGiveItems] = useState(false)
  const [showActions, setShowActions] = useState(false)

  const loadPlayers = useCallback(() => {
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => api.players.list())
      .then(setPlayers)
      .catch((e: unknown) => toast.danger(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    loadPlayers()
  }, [loadPlayers])

  const loadSideData = async (section: SidebarKey) => {
    setActive(section)
    if (section === 'players') return
    setSideLoading(true)
    try {
      if (section === 'online') setOnlineData(await api.players.online())
      else if (section === 'currency') setCurrencyData(await api.players.currency())
      else if (section === 'factions') setFactionData(await api.players.factions())
      else if (section === 'specs') setSpecData(await api.players.specs())
    }
    catch (e: unknown) {
      toast.danger(e instanceof Error ? e.message : String(e))
    }
    finally {
      setSideLoading(false)
    }
  }

  const controllerToName = useMemo(() => {
    const m = new Map<number, string>()
    for (const p of players) m.set(p.controller_id, p.name)
    return m
  }, [players])

  const handleAction = (player: Player, action: 'inventory' | 'give' | 'actions') => {
    setSelectedPlayer(player)
    if (action === 'inventory') setShowInventory(true)
    else if (action === 'give') setShowGiveItems(true)
    else setShowActions(true)
  }

  return (
    <div className="flex gap-4 h-full min-h-0">
      <Sidebar active={active} onSelect={loadSideData} />

      {/* Main view region: flex column, min-h-0 so the Table inside can own
          its own scrolling via Table.ScrollContainer. NO overflow-hidden
          here — that would clip the Input's focus ring at the edge.       */}
      <div className="flex-1 flex flex-col gap-3 min-h-0">
        {active === 'players' && <PlayersListView players={players} loading={loading} onRefresh={loadPlayers} onAction={handleAction} />}
        {active === 'currency' && <CurrencyView data={currencyData} loading={sideLoading} controllerToName={controllerToName} />}
        {active === 'factions' && <FactionsView data={factionData} loading={sideLoading} controllerToName={controllerToName} />}
        {active === 'specs' && <SpecsView data={specData} loading={sideLoading} controllerToName={controllerToName} />}
        {active === 'online' && <OnlineView data={onlineData} loading={sideLoading} />}
      </div>

      {selectedPlayer && (
        <InventoryModal player={selectedPlayer} open={showInventory} onClose={() => setShowInventory(false)} />
      )}
      {selectedPlayer && (
        <GiveItemsModal player={selectedPlayer} open={showGiveItems} onClose={() => setShowGiveItems(false)} />
      )}
      {selectedPlayer && (
        <PlayerActionsModal player={selectedPlayer} open={showActions} onClose={() => setShowActions(false)} />
      )}
    </div>
  )
}
